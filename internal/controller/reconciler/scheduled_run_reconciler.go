package reconciler

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/robfig/cron/v3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
)

const (
	scheduledByLabel    = "kube-agent-helper.io/scheduled-by"
	defaultHistoryLimit = int32(10)
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

type ScheduledRunReconciler struct {
	client.Client
}

func (r *ScheduledRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var run k8saiV1.DiagnosticRun
	if err := r.Get(ctx, req.NamespacedName, &run); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if run.Spec.Schedule == "" {
		return ctrl.Result{}, nil
	}

	sched, err := cronParser.Parse(run.Spec.Schedule)
	if err != nil {
		logger.Error(err, "invalid cron expression", "schedule", run.Spec.Schedule)
		return ctrl.Result{}, nil
	}

	now := time.Now().UTC()

	if run.Status.NextRunAt == nil {
		next := sched.Next(now)
		run.Status.NextRunAt = &metav1.Time{Time: next}
		if err := r.Status().Update(ctx, &run); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("scheduled run initialized", "nextRunAt", next)
		return ctrl.Result{RequeueAfter: time.Until(next)}, nil
	}

	nextRunAt := run.Status.NextRunAt.Time

	if now.Before(nextRunAt) {
		return ctrl.Result{RequeueAfter: time.Until(nextRunAt)}, nil
	}

	childName := childRunName(run.Name, nextRunAt)

	var existing k8saiV1.DiagnosticRun
	if err := r.Get(ctx, client.ObjectKey{Namespace: run.Namespace, Name: childName}, &existing); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		child := r.buildChildRun(&run, childName, nextRunAt)
		if err := r.Create(ctx, child); err != nil {
			return ctrl.Result{}, fmt.Errorf("create child run: %w", err)
		}
		logger.Info("created child run", "name", childName)
	}

	next := sched.Next(nextRunAt)
	run.Status.LastRunAt = &metav1.Time{Time: nextRunAt}
	run.Status.NextRunAt = &metav1.Time{Time: next}
	run.Status.ActiveRuns = appendUnique(run.Status.ActiveRuns, childName)

	if err := r.enforceHistoryLimit(ctx, &run); err != nil {
		logger.Error(err, "enforce history limit")
	}

	if err := r.Status().Update(ctx, &run); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Until(next)}, nil
}

func (r *ScheduledRunReconciler) buildChildRun(parent *k8saiV1.DiagnosticRun, name string, triggeredAt time.Time) *k8saiV1.DiagnosticRun {
	truePtr := true
	return &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: parent.Namespace,
			Labels: map[string]string{
				scheduledByLabel: parent.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "kube-agent-helper.io/v1alpha1",
					Kind:               "DiagnosticRun",
					Name:               parent.Name,
					UID:                parent.UID,
					Controller:         &truePtr,
					BlockOwnerDeletion: &truePtr,
				},
			},
			Annotations: map[string]string{
				"kube-agent-helper.io/triggered-at": triggeredAt.Format(time.RFC3339),
			},
		},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:         parent.Spec.Target,
			Skills:         parent.Spec.Skills,
			ModelConfigRef: parent.Spec.ModelConfigRef,
			TimeoutSeconds: parent.Spec.TimeoutSeconds,
			OutputLanguage: parent.Spec.OutputLanguage,
		},
	}
}

func (r *ScheduledRunReconciler) enforceHistoryLimit(ctx context.Context, parent *k8saiV1.DiagnosticRun) error {
	limit := defaultHistoryLimit
	if parent.Spec.HistoryLimit != nil {
		limit = *parent.Spec.HistoryLimit
	}
	if int32(len(parent.Status.ActiveRuns)) <= limit {
		return nil
	}

	var children k8saiV1.DiagnosticRunList
	if err := r.List(ctx, &children,
		client.InNamespace(parent.Namespace),
		client.MatchingLabels{scheduledByLabel: parent.Name},
	); err != nil {
		return err
	}
	sort.Slice(children.Items, func(i, j int) bool {
		return children.Items[i].CreationTimestamp.Before(&children.Items[j].CreationTimestamp)
	})

	toDelete := len(children.Items) - int(limit)
	for i := 0; i < toDelete; i++ {
		child := children.Items[i]
		if err := r.Delete(ctx, &child,
			client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("delete old child run %s: %w", child.Name, err)
		}
		parent.Status.ActiveRuns = removeFromSlice(parent.Status.ActiveRuns, child.Name)
	}
	return nil
}

func (r *ScheduledRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8saiV1.DiagnosticRun{}).
		Owns(&k8saiV1.DiagnosticRun{}).
		Named("scheduled-run").
		Complete(r)
}

func childRunName(parentName string, t time.Time) string {
	name := fmt.Sprintf("%s-%d", parentName, t.Unix())
	if len(name) > 253 {
		name = name[len(name)-253:]
	}
	return name
}

func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

func removeFromSlice(slice []string, s string) []string {
	result := make([]string, 0, len(slice))
	for _, v := range slice {
		if v != s {
			result = append(result, v)
		}
	}
	return result
}
