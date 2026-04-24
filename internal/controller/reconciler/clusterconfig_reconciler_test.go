package reconciler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/reconciler"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/registry"
)

func buildClusterConfigReconciler(cc *k8saiV1.ClusterConfig, secret *corev1.Secret) (*reconciler.ClusterConfigReconciler, *registry.ClusterClientRegistry) {
	reg := registry.NewClusterClientRegistry()
	scheme := testScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	if cc != nil {
		builder = builder.WithObjects(cc).WithStatusSubresource(cc)
	}
	if secret != nil {
		builder = builder.WithObjects(secret)
	}
	fakeClient := builder.Build()
	r := &reconciler.ClusterConfigReconciler{
		Client:   fakeClient,
		Registry: reg,
	}
	return r, reg
}

func reconcileClusterConfig(t *testing.T, r *reconciler.ClusterConfigReconciler, name, namespace string) (ctrl.Result, error) {
	t.Helper()
	return r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
	})
}

// TestClusterConfig_NotFound verifies that reconciling a deleted ClusterConfig
// removes the entry from the registry.
func TestClusterConfig_NotFound(t *testing.T) {
	reg := registry.NewClusterClientRegistry()
	// Pre-populate registry with a stale entry
	reg.Set("deleted-cluster", fake.NewClientBuilder().Build())

	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &reconciler.ClusterConfigReconciler{
		Client:   fakeClient,
		Registry: reg,
	}

	result, err := reconcileClusterConfig(t, r, "deleted-cluster", "default")
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	_, ok := reg.Get("deleted-cluster")
	assert.False(t, ok, "deleted ClusterConfig should be removed from registry")
}

// TestClusterConfig_SecretNotFound verifies error status when the referenced secret doesn't exist.
func TestClusterConfig_SecretNotFound(t *testing.T) {
	cc := &k8saiV1.ClusterConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "prod", Namespace: "default"},
		Spec: k8saiV1.ClusterConfigSpec{
			KubeConfigRef: k8saiV1.SecretKeyRef{Name: "missing-secret", Key: "kubeconfig"},
		},
	}

	r, reg := buildClusterConfigReconciler(cc, nil)

	_, err := reconcileClusterConfig(t, r, "prod", "default")
	// setStatus may return error from status update, but the reconciler should have attempted it
	// The important thing is the cluster was NOT registered
	_ = err
	_, ok := reg.Get("prod")
	assert.False(t, ok, "cluster should not be registered when secret is missing")
}

// TestClusterConfig_KeyNotFoundInSecret verifies error when the secret exists but key is missing.
func TestClusterConfig_KeyNotFoundInSecret(t *testing.T) {
	cc := &k8saiV1.ClusterConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "prod", Namespace: "default"},
		Spec: k8saiV1.ClusterConfigSpec{
			KubeConfigRef: k8saiV1.SecretKeyRef{Name: "my-secret", Key: "missing-key"},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"},
		Data:       map[string][]byte{"other-key": []byte("data")},
	}

	r, reg := buildClusterConfigReconciler(cc, secret)

	_, err := reconcileClusterConfig(t, r, "prod", "default")
	_ = err
	_, ok := reg.Get("prod")
	assert.False(t, ok, "cluster should not be registered when key is missing in secret")

	// Verify status was set to Error
	var updated k8saiV1.ClusterConfig
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "prod", Namespace: "default"}, &updated))
	assert.Equal(t, "Error", updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "missing-key")
}

// TestClusterConfig_InvalidKubeconfig verifies error when kubeconfig data is malformed.
func TestClusterConfig_InvalidKubeconfig(t *testing.T) {
	cc := &k8saiV1.ClusterConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "prod", Namespace: "default"},
		Spec: k8saiV1.ClusterConfigSpec{
			KubeConfigRef: k8saiV1.SecretKeyRef{Name: "my-secret", Key: "kubeconfig"},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"},
		Data:       map[string][]byte{"kubeconfig": []byte("this is not valid yaml kubeconfig")},
	}

	r, reg := buildClusterConfigReconciler(cc, secret)

	_, err := reconcileClusterConfig(t, r, "prod", "default")
	_ = err
	_, ok := reg.Get("prod")
	assert.False(t, ok, "cluster should not be registered with invalid kubeconfig")

	var updated k8saiV1.ClusterConfig
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "prod", Namespace: "default"}, &updated))
	assert.Equal(t, "Error", updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "invalid kubeconfig")
}

// TestClusterConfig_ValidKubeconfig verifies successful registration with a valid kubeconfig.
func TestClusterConfig_ValidKubeconfig(t *testing.T) {
	cc := &k8saiV1.ClusterConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "prod", Namespace: "default"},
		Spec: k8saiV1.ClusterConfigSpec{
			KubeConfigRef: k8saiV1.SecretKeyRef{Name: "my-secret", Key: "kubeconfig"},
			PrometheusURL: "http://prometheus.prod:9090",
			Description:   "Production cluster",
		},
	}

	// Build a minimal valid kubeconfig
	kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: fake-token
`)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"},
		Data:       map[string][]byte{"kubeconfig": kubeconfig},
	}

	r, reg := buildClusterConfigReconciler(cc, secret)

	result, err := reconcileClusterConfig(t, r, "prod", "default")
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	client, ok := reg.Get("prod")
	assert.True(t, ok, "cluster should be registered after successful reconciliation")
	assert.NotNil(t, client)

	// Verify status was set to Connected
	var updated k8saiV1.ClusterConfig
	require.NoError(t, r.Get(context.Background(), types.NamespacedName{Name: "prod", Namespace: "default"}, &updated))
	assert.Equal(t, "Connected", updated.Status.Phase)
	assert.Empty(t, updated.Status.Message)
}

// TestClusterConfig_ReRegistration verifies that re-reconciling updates the client in the registry.
func TestClusterConfig_ReRegistration(t *testing.T) {
	cc := &k8saiV1.ClusterConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "staging", Namespace: "default"},
		Spec: k8saiV1.ClusterConfigSpec{
			KubeConfigRef: k8saiV1.SecretKeyRef{Name: "staging-secret", Key: "kubeconfig"},
		},
	}

	kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
  name: staging
contexts:
- context:
    cluster: staging
    user: staging-user
  name: staging-context
current-context: staging-context
users:
- name: staging-user
  user:
    token: fake-token
`)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "staging-secret", Namespace: "default"},
		Data:       map[string][]byte{"kubeconfig": kubeconfig},
	}

	r, reg := buildClusterConfigReconciler(cc, secret)

	// First reconciliation
	_, err := reconcileClusterConfig(t, r, "staging", "default")
	require.NoError(t, err)
	firstClient, ok := reg.Get("staging")
	require.True(t, ok)

	// Second reconciliation — should succeed and update the client
	_, err = reconcileClusterConfig(t, r, "staging", "default")
	require.NoError(t, err)
	secondClient, ok := reg.Get("staging")
	require.True(t, ok)

	// Both should be non-nil (the client gets replaced each time)
	assert.NotNil(t, firstClient)
	assert.NotNil(t, secondClient)
}

// TestClusterConfig_DeleteAfterRegistration verifies the full lifecycle: register then delete.
func TestClusterConfig_DeleteAfterRegistration(t *testing.T) {
	cc := &k8saiV1.ClusterConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "temp", Namespace: "default"},
		Spec: k8saiV1.ClusterConfigSpec{
			KubeConfigRef: k8saiV1.SecretKeyRef{Name: "temp-secret", Key: "kubeconfig"},
		},
	}

	kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
  name: temp
contexts:
- context:
    cluster: temp
    user: temp-user
  name: temp-context
current-context: temp-context
users:
- name: temp-user
  user:
    token: fake-token
`)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "temp-secret", Namespace: "default"},
		Data:       map[string][]byte{"kubeconfig": kubeconfig},
	}

	r, reg := buildClusterConfigReconciler(cc, secret)

	// Register
	_, err := reconcileClusterConfig(t, r, "temp", "default")
	require.NoError(t, err)
	_, ok := reg.Get("temp")
	require.True(t, ok, "should be registered")

	// Simulate deletion by removing the CR from the fake client
	require.NoError(t, r.Delete(context.Background(), cc))

	// Reconcile again — should detect NotFound and remove from registry
	_, err = reconcileClusterConfig(t, r, "temp", "default")
	require.NoError(t, err)
	_, ok = reg.Get("temp")
	assert.False(t, ok, "should be unregistered after deletion")
}
