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
)

// testModelConfig returns a ModelConfig with a valid APIKeyRef pointing to a
// Secret named "claude-secret" in namespace "default".
func testModelConfig() *k8saiV1.ModelConfig {
	return &k8saiV1.ModelConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "claude-default",
			Namespace: "default",
		},
		Spec: k8saiV1.ModelConfigSpec{
			Provider: "anthropic",
			Model:    "claude-sonnet-4-6",
			APIKeyRef: k8saiV1.SecretKeyRef{
				Name: "claude-secret",
				Key:  "api-key",
			},
		},
	}
}

// testAPISecret returns the Secret referenced by testModelConfig.
func testAPISecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "claude-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"api-key": []byte("sk-test-key"),
		},
	}
}

// TestModelConfigReconciler_Reconcile verifies that when a ModelConfig CR and
// its referenced Secret both exist, the reconciler processes the object without
// returning an error and does not request a requeue.
func TestModelConfigReconciler_Reconcile(t *testing.T) {
	mc := testModelConfig()
	secret := testAPISecret()

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(mc, secret).
		Build()

	r := &reconciler.ModelConfigReconciler{
		Client: fakeClient,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      mc.Name,
			Namespace: mc.Namespace,
		},
	})

	require.NoError(t, err, "reconciling an existing ModelConfig should not return an error")
	assert.Zero(t, result.RequeueAfter, "no requeue expected for a fully valid ModelConfig")
}

// TestModelConfigReconciler_NotFound verifies that reconciling a request for a
// CR that no longer exists (already deleted) is handled gracefully — the
// reconciler returns no error and no requeue.
func TestModelConfigReconciler_NotFound(t *testing.T) {
	// Build a client with no objects at all so the Get returns NotFound.
	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		Build()

	r := &reconciler.ModelConfigReconciler{
		Client: fakeClient,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent-model-config",
			Namespace: "default",
		},
	})

	require.NoError(t, err, "a NotFound error must be swallowed, not propagated")
	assert.Zero(t, result.RequeueAfter, "deleted CR should not trigger a requeue")
}
