package reconciler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/reconciler"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type memStore struct{ runs map[string]*store.DiagnosticRun }

func newMemStore() *memStore { return &memStore{runs: map[string]*store.DiagnosticRun{}} }
func (m *memStore) CreateRun(_ context.Context, r *store.DiagnosticRun) error {
	if r.ID == "" {
		r.ID = "test-id"
	}
	m.runs[r.ID] = r
	return nil
}
func (m *memStore) GetRun(_ context.Context, id string) (*store.DiagnosticRun, error) {
	return m.runs[id], nil
}
func (m *memStore) UpdateRunStatus(_ context.Context, id string, p store.Phase, msg string) error {
	if r, ok := m.runs[id]; ok {
		r.Status = p
	}
	return nil
}
func (m *memStore) ListRuns(_ context.Context, _ store.ListOpts) ([]*store.DiagnosticRun, error) {
	return nil, nil
}
func (m *memStore) CreateFinding(_ context.Context, _ *store.Finding) error { return nil }
func (m *memStore) ListFindings(_ context.Context, _ string) ([]*store.Finding, error) {
	return nil, nil
}
func (m *memStore) UpsertSkill(_ context.Context, _ *store.Skill) error { return nil }
func (m *memStore) ListSkills(_ context.Context) ([]*store.Skill, error) { return nil, nil }
func (m *memStore) GetSkill(_ context.Context, _ string) (*store.Skill, error) {
	return nil, nil
}
func (m *memStore) Close() error { return nil }

func TestRunReconciler_PendingToRunning(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = k8saiV1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)

	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-run", Namespace: "default", UID: "uid-1",
		},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:         k8saiV1.TargetSpec{Scope: "namespace", Namespaces: []string{"default"}},
			Skills:         []string{"pod-health-analyst"},
			ModelConfigRef: "claude-default",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(run).
		WithStatusSubresource(run).
		Build()

	skill := &store.Skill{
		Name: "pod-health-analyst", Dimension: "health",
		Prompt: "You are...", ToolsJSON: "[]", Enabled: true,
	}
	tr := translator.New(translator.Config{
		AgentImage: "agent:test", ControllerURL: "http://ctrl:8080",
	}, []*store.Skill{skill})

	ms := newMemStore()
	r := &reconciler.DiagnosticRunReconciler{
		Client:     fakeClient,
		Store:      ms,
		Translator: tr,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-run", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.False(t, result.Requeue)

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "test-run", Namespace: "default"}, &updated))
	assert.Equal(t, "Running", updated.Status.Phase)
}
