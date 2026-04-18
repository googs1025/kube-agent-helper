package reconciler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/reconciler"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// skillMemStore is a memStore variant with richer skill tracking for assertions.
type skillMemStore struct {
	memStore
	skills      map[string]*store.Skill
	deleteCalls []string
}

func newSkillMemStore() *skillMemStore {
	return &skillMemStore{
		memStore: memStore{
			runs:     map[string]*store.DiagnosticRun{},
			findings: map[string][]*store.Finding{},
		},
		skills: map[string]*store.Skill{},
	}
}

func (s *skillMemStore) UpsertSkill(_ context.Context, sk *store.Skill) error {
	s.skills[sk.Name] = sk
	return nil
}

func (s *skillMemStore) GetSkill(_ context.Context, name string) (*store.Skill, error) {
	sk, ok := s.skills[name]
	if !ok {
		return nil, store.ErrNotFound
	}
	return sk, nil
}

func (s *skillMemStore) DeleteSkill(_ context.Context, name string) error {
	s.deleteCalls = append(s.deleteCalls, name)
	delete(s.skills, name)
	return nil
}

func (s *skillMemStore) ListSkills(_ context.Context) ([]*store.Skill, error) {
	out := make([]*store.Skill, 0, len(s.skills))
	for _, sk := range s.skills {
		out = append(out, sk)
	}
	return out, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func makeSkillCR(name string, enabled bool, priority *int) *k8saiV1.DiagnosticSkill {
	return &k8saiV1.DiagnosticSkill{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: k8saiV1.DiagnosticSkillSpec{
			Dimension:   "health",
			Description: "Test skill",
			Prompt:      "You are a test skill.",
			Tools:       []string{"get_pods", "describe_pod"},
			Enabled:     enabled,
			Priority:    priority,
		},
	}
}

func reconcileSkillOnce(t *testing.T, r *reconciler.DiagnosticSkillReconciler, name string) ctrl.Result {
	t.Helper()
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: "default"},
	})
	require.NoError(t, err)
	return result
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestSkillReconciler_CreateSkill(t *testing.T) {
	cr := makeSkillCR("pod-health-analyst", true, nil)

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(cr).
		Build()

	ms := newSkillMemStore()
	r := &reconciler.DiagnosticSkillReconciler{
		Client: fakeClient,
		Store:  ms,
	}

	_ = reconcileSkillOnce(t, r, "pod-health-analyst")

	got, ok := ms.skills["pod-health-analyst"]
	require.True(t, ok, "skill should be present in store after create")

	assert.Equal(t, "pod-health-analyst", got.Name)
	assert.Equal(t, "health", got.Dimension)
	assert.Equal(t, "You are a test skill.", got.Prompt)
	assert.Equal(t, "cr", got.Source)
	assert.True(t, got.Enabled)
	assert.Equal(t, 100, got.Priority, "default priority should be 100")
	assert.Contains(t, got.ToolsJSON, "get_pods")
}

func TestSkillReconciler_UpdateSkill(t *testing.T) {
	cr := makeSkillCR("pod-health-analyst", true, nil)

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(cr).
		Build()

	ms := newSkillMemStore()
	// Pre-seed with an older version.
	ms.skills["pod-health-analyst"] = &store.Skill{
		Name:      "pod-health-analyst",
		Dimension: "security",
		Prompt:    "old prompt",
		Source:    "cr",
		Enabled:   true,
		Priority:  50,
	}

	r := &reconciler.DiagnosticSkillReconciler{
		Client: fakeClient,
		Store:  ms,
	}

	_ = reconcileSkillOnce(t, r, "pod-health-analyst")

	got := ms.skills["pod-health-analyst"]
	require.NotNil(t, got)

	// Fields should reflect the CR, not the old store value.
	assert.Equal(t, "health", got.Dimension, "dimension should be updated")
	assert.Equal(t, "You are a test skill.", got.Prompt, "prompt should be updated")
	assert.Equal(t, 100, got.Priority, "priority should be updated to default 100")
	assert.Equal(t, "cr", got.Source)
}

func TestSkillReconciler_DeleteSkill(t *testing.T) {
	// Build a fake client with NO skill CR (simulating deletion).
	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		Build()

	ms := newSkillMemStore()
	// Pre-seed the store so the reconciler finds a CR-sourced skill to delete.
	ms.skills["pod-health-analyst"] = &store.Skill{
		Name:   "pod-health-analyst",
		Source: "cr",
	}

	r := &reconciler.DiagnosticSkillReconciler{
		Client: fakeClient,
		Store:  ms,
	}

	_ = reconcileSkillOnce(t, r, "pod-health-analyst")

	assert.Contains(t, ms.deleteCalls, "pod-health-analyst", "DeleteSkill should have been called")
	assert.NotContains(t, ms.skills, "pod-health-analyst", "skill should be removed from store")
}

func TestSkillReconciler_DisabledSkill(t *testing.T) {
	cr := makeSkillCR("disabled-skill", false, nil)

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(cr).
		Build()

	ms := newSkillMemStore()
	r := &reconciler.DiagnosticSkillReconciler{
		Client: fakeClient,
		Store:  ms,
	}

	_ = reconcileSkillOnce(t, r, "disabled-skill")

	got, ok := ms.skills["disabled-skill"]
	require.True(t, ok, "disabled skill should still be upserted into store")
	assert.False(t, got.Enabled, "Enabled field should be false")
	assert.Equal(t, "cr", got.Source)
}
