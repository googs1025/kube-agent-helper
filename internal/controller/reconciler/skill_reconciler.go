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
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// DiagnosticSkillReconciler 把 DiagnosticSkill CR 同步到 Store。
//
// 双源 Skill 模型：
//   - source="builtin"  启动时 main.go 从 /skills/*.md 加载（可被同名 CR 覆盖）
//   - source="cr"       这里写入，用户通过 kubectl apply 管理
//
// CR 删除时只清理 source="cr" 的记录，避免误删 builtin。
// Translator 调用 SkillRegistry.ListEnabled 时会按 priority 合并两类来源。
type DiagnosticSkillReconciler struct {
	client.Client
	Store store.Store
}

func (r *DiagnosticSkillReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var skill k8saiV1.DiagnosticSkill
	if err := r.Get(ctx, req.NamespacedName, &skill); err != nil {
		if errors.IsNotFound(err) {
			// CR deleted — remove from store only if it was CR-sourced.
			existing, getErr := r.Store.GetSkill(ctx, req.Name)
			if getErr == nil && existing.Source == "cr" {
				if delErr := r.Store.DeleteSkill(ctx, req.Name); delErr != nil {
					logger.Error(delErr, "failed to delete skill from store", "name", req.Name)
				} else {
					logger.Info("deleted cr skill from store", "name", req.Name)
				}
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	toolsJSON, err := json.Marshal(skill.Spec.Tools)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("marshal tools: %w", err)
	}
	requiresJSON, err := json.Marshal(skill.Spec.RequiresData)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("marshal requiresData: %w", err)
	}

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
