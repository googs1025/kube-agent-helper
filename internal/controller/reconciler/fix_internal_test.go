package reconciler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

func internalTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, k8saiV1.AddToScheme(s))
	require.NoError(t, appsv1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	return s
}

// inMemFixStore is a tiny store stub satisfying the methods rollback() uses.
// Same package access lets us avoid implementing the entire Store interface.
type inMemFixStore struct {
	store.Store
	updates []phaseUpdate
}

type phaseUpdate struct {
	id    string
	phase store.FixPhase
	msg   string
}

func (s *inMemFixStore) UpdateFixPhase(_ context.Context, id string, p store.FixPhase, msg string) error {
	s.updates = append(s.updates, phaseUpdate{id, p, msg})
	return nil
}

func (s *inMemFixStore) UpdateFixSnapshot(_ context.Context, _ string, _ string) error { return nil }

// ── rollback ─────────────────────────────────────────────────────────────────

func TestRollback_RestoresSnapshotAndUpdatesPhase(t *testing.T) {
	// A pre-existing Deployment with a known snapshot stored on the fix.
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nginx", Namespace: "default",
		},
	}
	snapBytes, err := json.Marshal(map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "nginx", "namespace": "default"},
		"spec":       map[string]interface{}{"replicas": 1},
	})
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString(snapBytes)

	fix := &k8saiV1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "fix-rb", Namespace: "default", UID: "uid-rb"},
		Spec: k8saiV1.DiagnosticFixSpec{
			Target: k8saiV1.FixTarget{Kind: "Deployment", Namespace: "default", Name: "nginx"},
		},
		Status: k8saiV1.DiagnosticFixStatus{RollbackSnapshot: encoded},
	}

	cli := fake.NewClientBuilder().
		WithScheme(internalTestScheme(t)).
		WithObjects(dep, fix).
		WithStatusSubresource(fix).
		Build()
	st := &inMemFixStore{}
	r := &DiagnosticFixReconciler{Client: cli, Store: st}

	err = r.rollback(context.Background(), fix)
	require.NoError(t, err)

	assert.Equal(t, "RolledBack", fix.Status.Phase)
	assert.NotNil(t, fix.Status.CompletedAt)
	require.Len(t, st.updates, 1)
	assert.Equal(t, store.FixPhaseRolledBack, st.updates[0].phase)
}

func TestRollback_BadBase64ReturnsError(t *testing.T) {
	fix := &k8saiV1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "fix-rb-bad", Namespace: "default", UID: "u"},
		Status:     k8saiV1.DiagnosticFixStatus{RollbackSnapshot: "!!!not-base64!!!"},
	}
	cli := fake.NewClientBuilder().WithScheme(internalTestScheme(t)).Build()
	r := &DiagnosticFixReconciler{Client: cli, Store: &inMemFixStore{}}

	err := r.rollback(context.Background(), fix)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode rollback snapshot")
}

func TestRollback_BadJSONReturnsError(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("not json"))
	fix := &k8saiV1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "fix-rb-bad", Namespace: "default", UID: "u"},
		Status:     k8saiV1.DiagnosticFixStatus{RollbackSnapshot: encoded},
	}
	cli := fake.NewClientBuilder().WithScheme(internalTestScheme(t)).Build()
	r := &DiagnosticFixReconciler{Client: cli, Store: &inMemFixStore{}}

	err := r.rollback(context.Background(), fix)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal rollback snapshot")
}
