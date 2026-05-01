package reconciler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/reconciler"
	"github.com/kube-agent-helper/kube-agent-helper/internal/notification"
)

// ── applyPatch happy path via Reconcile ──────────────────────────────────────

func TestFixReconciler_ApplyPatch_StrategicMerge_OK(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "nginx", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "nginx"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "nginx"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "n", Image: "nginx:1.0"}},
				},
			},
		},
	}

	fix := &k8saiV1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "fix-patch", Namespace: "default", UID: types.UID("uid-patch")},
		Spec: k8saiV1.DiagnosticFixSpec{
			DiagnosticRunRef: "run-1",
			FindingTitle:     "scale up",
			Target:           k8saiV1.FixTarget{Kind: "Deployment", Namespace: "default", Name: "nginx"},
			Strategy:         "patch",
			ApprovalRequired: true,
			Patch: k8saiV1.FixPatch{
				Type:    "strategic-merge",
				Content: `{"spec":{"replicas":3}}`,
			},
			Rollback: k8saiV1.RollbackConfig{SnapshotBefore: true},
		},
		Status: k8saiV1.DiagnosticFixStatus{Phase: "Approved"},
	}

	cli := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(dep, fix).
		WithStatusSubresource(fix).
		Build()

	r := &reconciler.DiagnosticFixReconciler{Client: cli, Store: newMemStore()}
	fixReconcileOnce(t, r, "fix-patch", "default")

	var updated k8saiV1.DiagnosticFix
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "fix-patch", Namespace: "default"}, &updated))
	assert.Equal(t, "Succeeded", updated.Status.Phase)
	assert.NotEmpty(t, updated.Status.RollbackSnapshot, "snapshot should be captured")
	assert.NotNil(t, updated.Status.AppliedAt)
	assert.NotNil(t, updated.Status.CompletedAt)
}

func TestFixReconciler_ApplyPatch_UnsupportedKind_Fails(t *testing.T) {
	fix := &k8saiV1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "fix-bad-kind", Namespace: "default", UID: types.UID("u")},
		Spec: k8saiV1.DiagnosticFixSpec{
			Target:   k8saiV1.FixTarget{Kind: "WidgetCRD", Namespace: "default", Name: "x"},
			Strategy: "patch",
			Patch:    k8saiV1.FixPatch{Type: "strategic-merge", Content: `{}`},
		},
		Status: k8saiV1.DiagnosticFixStatus{Phase: "Approved"},
	}
	cli := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(fix).
		WithStatusSubresource(fix).
		Build()
	r := &reconciler.DiagnosticFixReconciler{Client: cli, Store: newMemStore()}

	fixReconcileOnce(t, r, "fix-bad-kind", "default")

	var got k8saiV1.DiagnosticFix
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "fix-bad-kind", Namespace: "default"}, &got))
	assert.Equal(t, "Failed", got.Status.Phase)
	assert.Contains(t, got.Status.Message, "unsupported target kind")
}

func TestFixReconciler_ApplyPatch_MissingTarget_Fails(t *testing.T) {
	// No Deployment present in the fake client → snapshot Get fails.
	fix := &k8saiV1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "fix-snap", Namespace: "default", UID: types.UID("u")},
		Spec: k8saiV1.DiagnosticFixSpec{
			Target:   k8saiV1.FixTarget{Kind: "Deployment", Namespace: "default", Name: "missing"},
			Strategy: "patch",
			Patch:    k8saiV1.FixPatch{Type: "strategic-merge", Content: `{"spec":{"replicas":3}}`},
			Rollback: k8saiV1.RollbackConfig{SnapshotBefore: true},
		},
		Status: k8saiV1.DiagnosticFixStatus{Phase: "Approved"},
	}
	cli := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(fix).
		WithStatusSubresource(fix).
		Build()
	r := &reconciler.DiagnosticFixReconciler{Client: cli, Store: newMemStore()}

	fixReconcileOnce(t, r, "fix-snap", "default")

	var got k8saiV1.DiagnosticFix
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "fix-snap", Namespace: "default"}, &got))
	assert.Equal(t, "Failed", got.Status.Phase)
}

// ── Reconcile init branches ──────────────────────────────────────────────────

func TestFixReconciler_NotFound_NoOp(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(testScheme()).Build()
	r := &reconciler.DiagnosticFixReconciler{Client: cli, Store: newMemStore()}
	fixReconcileOnce(t, r, "ghost", "default")
}

func TestFixReconciler_AutoApproveSetsApproved(t *testing.T) {
	fix := &k8saiV1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "auto", Namespace: "default", UID: types.UID("u")},
		Spec: k8saiV1.DiagnosticFixSpec{
			Target:           k8saiV1.FixTarget{Kind: "ConfigMap", Namespace: "default", Name: "x"},
			Strategy:         "patch",
			ApprovalRequired: false,
			Patch:            k8saiV1.FixPatch{Type: "strategic-merge", Content: `{}`},
		},
	}
	cli := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(fix).
		WithStatusSubresource(fix).
		Build()
	r := &reconciler.DiagnosticFixReconciler{Client: cli, Store: newMemStore()}

	fixReconcileOnce(t, r, "auto", "default")

	var got k8saiV1.DiagnosticFix
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "auto", Namespace: "default"}, &got))
	assert.Equal(t, "Approved", got.Status.Phase)
	assert.Contains(t, got.Status.Message, "Auto-approved")
}

func TestFixReconciler_PendingApprovalIsTerminal(t *testing.T) {
	fix := &k8saiV1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "pa", Namespace: "default", UID: types.UID("u")},
		Spec: k8saiV1.DiagnosticFixSpec{
			Target:   k8saiV1.FixTarget{Kind: "ConfigMap", Namespace: "default", Name: "x"},
			Strategy: "patch",
			Patch:    k8saiV1.FixPatch{Type: "strategic-merge", Content: `{}`},
		},
		Status: k8saiV1.DiagnosticFixStatus{Phase: "PendingApproval"},
	}
	cli := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(fix).
		WithStatusSubresource(fix).
		Build()
	r := &reconciler.DiagnosticFixReconciler{Client: cli, Store: newMemStore()}

	res := fixReconcileOnce(t, r, "pa", "default")
	assert.False(t, res.Requeue, "PendingApproval should be a no-op")
}

// failFix Notifier path
type fixNotifier struct {
	calls []notificationEvent
}

type notificationEvent struct{ Title string }

func (n *fixNotifier) Notify(_ context.Context, e notificationEventForNotifier) error {
	n.calls = append(n.calls, notificationEvent{Title: e.Title})
	return nil
}

// notificationEventForNotifier is a thin alias used only to keep the
// import surface minimal in this test file.
type notificationEventForNotifier = notification.Event

func TestFixReconciler_FailFix_NotifierEmitsFailedEvent(t *testing.T) {
	fix := &k8saiV1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "fail-notify", Namespace: "default", UID: types.UID("u")},
		Spec: k8saiV1.DiagnosticFixSpec{
			Target:   k8saiV1.FixTarget{Kind: "WidgetCRD", Namespace: "default", Name: "x"},
			Strategy: "patch",
			Patch:    k8saiV1.FixPatch{Type: "strategic-merge", Content: `{}`},
		},
		Status: k8saiV1.DiagnosticFixStatus{Phase: "Approved"},
	}
	cli := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(fix).
		WithStatusSubresource(fix).
		Build()

	notifier := &fixNotifier{}
	r := &reconciler.DiagnosticFixReconciler{
		Client:   cli,
		Store:    newMemStore(),
		Notifier: notifier,
	}
	fixReconcileOnce(t, r, "fail-notify", "default")

	require.NotEmpty(t, notifier.calls, "notifier must be called when failFix runs")
	assert.Contains(t, notifier.calls[0].Title, "Fix Failed")
}

func TestFixReconciler_CreateStrategy_AlreadyExists_Fails(t *testing.T) {
	npJSON := `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":"dup","namespace":"default"},"spec":{"podSelector":{}}}`

	fix := &k8saiV1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "fix-dup", Namespace: "default", UID: types.UID("u")},
		Spec: k8saiV1.DiagnosticFixSpec{
			Target:   k8saiV1.FixTarget{Kind: "NetworkPolicy", Namespace: "default", Name: "dup"},
			Strategy: "create",
			Patch:    k8saiV1.FixPatch{Type: "strategic-merge", Content: npJSON},
		},
		Status: k8saiV1.DiagnosticFixStatus{Phase: "Approved"},
	}

	conflictNP := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "dup", Namespace: "default"},
	}

	cli := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(fix, conflictNP).
		WithStatusSubresource(fix).
		Build()
	r := &reconciler.DiagnosticFixReconciler{Client: cli, Store: newMemStore()}

	fixReconcileOnce(t, r, "fix-dup", "default")

	var got k8saiV1.DiagnosticFix
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "fix-dup", Namespace: "default"}, &got))
	assert.Equal(t, "Failed", got.Status.Phase)
	assert.Contains(t, got.Status.Message, "create resource failed")
}

func TestFixReconciler_ApplyingRequeues(t *testing.T) {
	fix := &k8saiV1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "applying", Namespace: "default", UID: types.UID("u")},
		Spec: k8saiV1.DiagnosticFixSpec{
			Target:   k8saiV1.FixTarget{Kind: "ConfigMap", Namespace: "default", Name: "x"},
			Strategy: "patch",
			Patch:    k8saiV1.FixPatch{Type: "strategic-merge", Content: `{}`},
		},
		Status: k8saiV1.DiagnosticFixStatus{Phase: "Applying"},
	}
	cli := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(fix).
		WithStatusSubresource(fix).
		Build()
	r := &reconciler.DiagnosticFixReconciler{Client: cli, Store: newMemStore()}

	res := fixReconcileOnce(t, r, "applying", "default")
	assert.Greater(t, res.RequeueAfter.Seconds(), 0.0, "Applying should requeue")
}
