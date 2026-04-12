package reconciler

import (
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type DiagnosticSkillReconciler struct {
	client.Client
	Store store.Store
}

func (r *DiagnosticSkillReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var skill k8saiV1.DiagnosticSkill
	if err := r.Get(ctx, req.NamespacedName, &skill); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	toolsJSON, _ := json.Marshal(skill.Spec.Tools)
	requiresJSON, _ := json.Marshal(skill.Spec.RequiresData)

	priority := 100
	if skill.Spec.Priority != nil {
		priority = *skill.Spec.Priority
	}

	s := &store.Skill{
		Name:             skill.Name,
		Dimension:        skill.Spec.Dimension,
		Prompt:           skill.Spec.Prompt,
		ToolsJSON:        string(toolsJSON),
		RequiresDataJSON: string(requiresJSON),
		Source:           "cr",
		Enabled:          skill.Spec.Enabled,
		Priority:         priority,
	}
	if err := r.Store.UpsertSkill(ctx, s); err != nil {
		return ctrl.Result{}, err
	}
	logger.Info("synced skill", "name", skill.Name)
	return ctrl.Result{}, nil
}

func (r *DiagnosticSkillReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8saiV1.DiagnosticSkill{}).
		Complete(r)
}
