package reconciler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/reconciler"
)

// Ensure networkingv1 is in the test scheme (testScheme is defined in run_reconciler_test.go)
var _ = networkingv1.NetworkPolicy{}

func fixReconcileOnce(t *testing.T, r *reconciler.DiagnosticFixReconciler, name, ns string) ctrl.Result {
	t.Helper()
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: ns},
	})
	require.NoError(t, err)
	return result
}

func TestFixReconciler_DryRunGoesToDryRunComplete(t *testing.T) {
	fix := &k8saiV1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "fix-1", Namespace: "default", UID: "uid-1"},
		Spec: k8saiV1.DiagnosticFixSpec{
			DiagnosticRunRef: "run-1",
			FindingTitle:     "test",
			Target:           k8saiV1.FixTarget{Kind: "Deployment", Namespace: "default", Name: "nginx"},
			Strategy:         "dry-run",
			ApprovalRequired: true,
			Patch:            k8saiV1.FixPatch{Type: "strategic-merge", Content: `{"spec":{"replicas":2}}`},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(fix).
		WithStatusSubresource(fix).
		Build()

	r := &reconciler.DiagnosticFixReconciler{
		Client: fakeClient,
		Store:  newMemStore(),
	}

	fixReconcileOnce(t, r, "fix-1", "default")

	var updated k8saiV1.DiagnosticFix
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "fix-1", Namespace: "default"}, &updated))
	assert.Equal(t, "DryRunComplete", updated.Status.Phase)
}

func TestFixReconciler_CreateStrategy_CreatesResource(t *testing.T) {
	// A Fix with strategy=create and a NetworkPolicy manifest as patch.content
	npJSON := `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":"deny-all","namespace":"default"},"spec":{"podSelector":{}}}`
	fix := &k8saiV1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "fix-create-1", Namespace: "default", UID: "uid-2"},
		Spec: k8saiV1.DiagnosticFixSpec{
			DiagnosticRunRef: "run-1",
			FindingTitle:     "No NetworkPolicy",
			Target:           k8saiV1.FixTarget{Kind: "NetworkPolicy", Namespace: "default", Name: "deny-all"},
			Strategy:         "create",
			ApprovalRequired: true,
			Patch:            k8saiV1.FixPatch{Type: "strategic-merge", Content: npJSON},
		},
	}
	// Start with Phase=Approved (as if user already approved)
	fix.Status.Phase = "Approved"

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(fix).
		WithStatusSubresource(fix).
		Build()

	ms := newMemStore()
	r := &reconciler.DiagnosticFixReconciler{
		Client: fakeClient,
		Store:  ms,
	}

	fixReconcileOnce(t, r, "fix-create-1", "default")

	// Fix should be Succeeded
	var updated k8saiV1.DiagnosticFix
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "fix-create-1", Namespace: "default"}, &updated))
	assert.Equal(t, "Succeeded", updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "created")

	// The NetworkPolicy should exist in the fake client
	var np networkingv1.NetworkPolicy
	err := fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "deny-all", Namespace: "default"}, &np)
	assert.NoError(t, err, "NetworkPolicy should have been created")
	assert.Equal(t, "deny-all", np.Name)
}

func TestFixReconciler_CreateStrategy_InvalidJSON(t *testing.T) {
	fix := &k8saiV1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "fix-bad", Namespace: "default", UID: "uid-3"},
		Spec: k8saiV1.DiagnosticFixSpec{
			DiagnosticRunRef: "run-1",
			FindingTitle:     "test",
			Target:           k8saiV1.FixTarget{Kind: "ConfigMap", Namespace: "default", Name: "x"},
			Strategy:         "create",
			ApprovalRequired: true,
			Patch:            k8saiV1.FixPatch{Type: "strategic-merge", Content: `not-valid-json`},
		},
	}
	fix.Status.Phase = "Approved"

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(fix).
		WithStatusSubresource(fix).
		Build()

	r := &reconciler.DiagnosticFixReconciler{
		Client: fakeClient,
		Store:  newMemStore(),
	}

	fixReconcileOnce(t, r, "fix-bad", "default")

	var updated k8saiV1.DiagnosticFix
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "fix-bad", Namespace: "default"}, &updated))
	assert.Equal(t, "Failed", updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "parse resource JSON")
}
