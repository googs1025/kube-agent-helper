package reconciler

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
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

		run.Status.Phase = string(store.PhaseRunning)
		run.Status.ReportID = string(run.UID)
		if err := r.Status().Update(ctx, &run); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Store.UpdateRunStatus(ctx, string(run.UID), store.PhaseRunning, ""); err != nil {
			logger.Error(err, "store.UpdateRunStatus failed")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Info("run started", "name", run.Name)
	}

	return ctrl.Result{}, nil
}

func (r *DiagnosticRunReconciler) failRun(ctx context.Context, run *k8saiV1.DiagnosticRun, msg string) (ctrl.Result, error) {
	run.Status.Phase = string(store.PhaseFailed)
	run.Status.Message = msg
	if err := r.Status().Update(ctx, run); err != nil {
		return ctrl.Result{}, fmt.Errorf("failRun status update: %w", err)
	}
	_ = r.Store.UpdateRunStatus(ctx, string(run.UID), store.PhaseFailed, msg)
	return ctrl.Result{}, nil
}

func (r *DiagnosticRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8saiV1.DiagnosticRun{}).
		Complete(r)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
