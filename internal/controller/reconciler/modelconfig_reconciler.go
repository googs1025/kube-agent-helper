package reconciler

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
)

// ModelConfigReconciler 仅做 ModelConfig 的合法性校验（Secret 是否存在等）。
//
// 设计上这是一个"只观察、不副作用"的 Reconciler：
//   - 不写 Store
//   - 不改其它 CR
//   - 只在 status 上记 Ready/Error 信息供 UI 展示
//
// 真正消费 ModelConfig 的是 Translator.resolveModelConfig（运行时按 ref 取）。
type ModelConfigReconciler struct {
	client.Client
}

func (r *ModelConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var mc k8saiV1.ModelConfig
	if err := r.Get(ctx, req.NamespacedName, &mc); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Validate APIKeyRef.Key is set
	if mc.Spec.APIKeyRef.Key == "" {
		logger.Error(nil, "apiKeyRef.key is empty", "modelconfig", mc.Name)
	}

	// Validate the referenced Secret exists
	var secret corev1.Secret
	secretRef := client.ObjectKey{
		Namespace: mc.Namespace,
		Name:      mc.Spec.APIKeyRef.Name,
	}
	if err := r.Get(ctx, secretRef, &secret); err != nil {
		logger.Error(err, "apiKeyRef secret not found", "secret", secretRef.Name)
		// Not a hard failure — the run will fail when it tries to use the key
	} else {
		logger.Info("ModelConfig validated", "name", mc.Name, "model", mc.Spec.Model)
	}

	return ctrl.Result{}, nil
}

func (r *ModelConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8saiV1.ModelConfig{}).
		Complete(r)
}
