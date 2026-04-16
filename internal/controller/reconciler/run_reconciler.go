package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type DiagnosticRunReconciler struct {
	client.Client
	Store      store.Store
	Translator *translator.Translator
}

func (r *DiagnosticRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var run k8saiV1.DiagnosticRun
	if err := r.Get(ctx, req.NamespacedName, &run); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Already terminal — nothing to do.
	if run.Status.Phase == string(store.PhaseSucceeded) || run.Status.Phase == string(store.PhaseFailed) {
		return ctrl.Result{}, nil
	}

	// Phase: Pending → Running
	if run.Status.Phase == "" || run.Status.Phase == string(store.PhasePending) {
		logger.Info("translating run", "name", run.Name)

		// Persist to store
		storeRun := &store.DiagnosticRun{
			ID:         string(run.UID),
			TargetJSON: mustJSON(run.Spec.Target),
			SkillsJSON: mustJSON(run.Spec.Skills),
			Status:     store.PhasePending,
		}
		if err := r.Store.CreateRun(ctx, storeRun); err != nil {
			return ctrl.Result{}, fmt.Errorf("store.CreateRun: %w", err)
		}

		// Translate to Job resources
		objects, err := r.Translator.Compile(ctx, &run)
		if err != nil {
			return r.failRun(ctx, &run, fmt.Sprintf("translate failed: %s", err))
		}

		// Apply all generated objects
		for _, obj := range objects {
			obj.SetNamespace(run.Namespace)
			if err := r.Create(ctx, obj); err != nil && !errors.IsAlreadyExists(err) {
				return r.failRun(ctx, &run, fmt.Sprintf("create %T: %s", obj, err))
			}
		}

		now := metav1.Now()
		run.Status.Phase = string(store.PhaseRunning)
		run.Status.ReportID = string(run.UID)
		run.Status.StartedAt = &now
		if err := r.Status().Update(ctx, &run); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Store.UpdateRunStatus(ctx, string(run.UID), store.PhaseRunning, ""); err != nil {
			logger.Error(err, "store.UpdateRunStatus failed")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Info("run started", "name", run.Name)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Phase: Running → check Job status
	if run.Status.Phase == string(store.PhaseRunning) {
		jobName := fmt.Sprintf("agent-%s", run.Name)
		var job batchv1.Job
		if err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: run.Namespace}, &job); err != nil {
			if errors.IsNotFound(err) {
				// Job not created yet or was cleaned up
				logger.Info("job not found, requeueing", "job", jobName)
				return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
			}
			return ctrl.Result{}, err
		}

		if job.Status.Succeeded > 0 {
			return r.completeRun(ctx, &run, store.PhaseSucceeded, "agent job completed successfully")
		}
		if job.Status.Failed > 0 {
			msg := "agent job failed"
			for _, c := range job.Status.Conditions {
				if c.Type == batchv1.JobFailed && c.Status == "True" {
					msg = fmt.Sprintf("agent job failed: %s", c.Message)
					break
				}
			}
			return r.completeRun(ctx, &run, store.PhaseFailed, msg)
		}

		// Still running — check Pod health for early error signals
		msg := r.podWaitingReason(ctx, job.Name, run.Namespace)
		if msg != "" && msg != run.Status.Message {
			run.Status.Message = msg
			if err := r.Status().Update(ctx, &run); err != nil {
				logger.Error(err, "failed to update run message")
			}
			_ = r.Store.UpdateRunStatus(ctx, string(run.UID), store.PhaseRunning, msg)
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// completeRun transitions a run to a terminal phase and writes findings back to CR status.
func (r *DiagnosticRunReconciler) completeRun(ctx context.Context, run *k8saiV1.DiagnosticRun, phase store.Phase, msg string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch findings from store
	runID := string(run.UID)
	findings, err := r.Store.ListFindings(ctx, runID)
	if err != nil {
		logger.Error(err, "failed to list findings")
		findings = nil
	}

	// Build severity counts and summaries
	counts := map[string]int{}
	var summaries []k8saiV1.FindingSummary
	for _, f := range findings {
		counts[f.Severity]++
		summaries = append(summaries, k8saiV1.FindingSummary{
			Dimension:         f.Dimension,
			Severity:          f.Severity,
			Title:             f.Title,
			ResourceKind:      f.ResourceKind,
			ResourceNamespace: f.ResourceNamespace,
			ResourceName:      f.ResourceName,
			Suggestion:        f.Suggestion,
		})
	}

	now := metav1.Now()
	run.Status.Phase = string(phase)
	run.Status.CompletedAt = &now
	run.Status.Message = msg
	run.Status.FindingCounts = counts
	run.Status.Findings = summaries

	if err := r.Status().Update(ctx, run); err != nil {
		return ctrl.Result{}, fmt.Errorf("completeRun status update: %w", err)
	}
	_ = r.Store.UpdateRunStatus(ctx, runID, phase, msg)

	logger.Info("run completed", "name", run.Name, "phase", phase, "findings", len(findings))
	return ctrl.Result{}, nil
}

func (r *DiagnosticRunReconciler) failRun(ctx context.Context, run *k8saiV1.DiagnosticRun, msg string) (ctrl.Result, error) {
	return r.completeRun(ctx, run, store.PhaseFailed, msg)
}

// podWaitingReason lists pods for the given job and returns a human-readable
// message if any container is in a waiting state (e.g. ImagePullBackOff).
func (r *DiagnosticRunReconciler) podWaitingReason(ctx context.Context, jobName, namespace string) string {
	var podList corev1.PodList
	if err := r.List(ctx, &podList,
		client.InNamespace(namespace),
		client.MatchingLabels{"job-name": jobName},
	); err != nil {
		return ""
	}
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
				reason := cs.State.Waiting.Reason
				detail := cs.State.Waiting.Message
				if detail != "" {
					return fmt.Sprintf("pod %s: %s — %s", pod.Name, reason, detail)
				}
				return fmt.Sprintf("pod %s: %s", pod.Name, reason)
			}
		}
		for _, cs := range pod.Status.InitContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
				reason := cs.State.Waiting.Reason
				detail := cs.State.Waiting.Message
				if detail != "" {
					return fmt.Sprintf("pod %s (init): %s — %s", pod.Name, reason, detail)
				}
				return fmt.Sprintf("pod %s (init): %s", pod.Name, reason)
			}
		}
	}
	return ""
}

func (r *DiagnosticRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8saiV1.DiagnosticRun{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
