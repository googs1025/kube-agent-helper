package reconciler

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/registry"
)

type ClusterConfigReconciler struct {
	client.Client
	Registry *registry.ClusterClientRegistry
}

func (r *ClusterConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cc k8saiV1.ClusterConfig
	if err := r.Get(ctx, req.NamespacedName, &cc); err != nil {
		if errors.IsNotFound(err) {
			r.Registry.Delete(req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Name: cc.Spec.KubeConfigRef.Name, Namespace: cc.Namespace,
	}, &secret); err != nil {
		return ctrl.Result{}, r.setStatus(ctx, &cc, "Error", "kubeconfig secret not found: "+err.Error())
	}

	kubeconfigData, ok := secret.Data[cc.Spec.KubeConfigRef.Key]
	if !ok {
		return ctrl.Result{}, r.setStatus(ctx, &cc, "Error", "key "+cc.Spec.KubeConfigRef.Key+" not found in secret")
	}

	restCfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
	if err != nil {
		return ctrl.Result{}, r.setStatus(ctx, &cc, "Error", "invalid kubeconfig: "+err.Error())
	}

	clusterClient, err := client.New(restCfg, client.Options{Scheme: r.Scheme()})
	if err != nil {
		return ctrl.Result{}, r.setStatus(ctx, &cc, "Error", "failed to build client: "+err.Error())
	}

	r.Registry.Set(cc.Name, clusterClient)
	logger.Info("registered cluster client", "cluster", cc.Name)

	return ctrl.Result{}, r.setStatus(ctx, &cc, "Connected", "")
}

func (r *ClusterConfigReconciler) setStatus(ctx context.Context, cc *k8saiV1.ClusterConfig, phase, msg string) error {
	cc.Status.Phase = phase
	cc.Status.Message = msg
	return r.Status().Update(ctx, cc)
}

func (r *ClusterConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8saiV1.ClusterConfig{}).
		Complete(r)
}
