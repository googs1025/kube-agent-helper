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

// ModelConfigReconciler validates ModelConfig resources and their Secret refs.
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
