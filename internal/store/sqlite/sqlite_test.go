package sqlite_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
	sqlitestore "github.com/kube-agent-helper/kube-agent-helper/internal/store/sqlite"
)

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*.db")
	require.NoError(t, err)
	f.Close()
	s, err := sqlitestore.New(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRun_CreateAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	run := &store.DiagnosticRun{
		TargetJSON: `{"namespaces":["default"]}`,
		SkillsJSON: `["pod-health-analyst"]`,
		Status:     store.PhasePending,
	}
	require.NoError(t, s.CreateRun(ctx, run))
	assert.NotEmpty(t, run.ID)

	got, err := s.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, store.PhasePending, got.Status)
}

func TestRun_UpdateStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	run := &store.DiagnosticRun{TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending}
	require.NoError(t, s.CreateRun(ctx, run))

	require.NoError(t, s.UpdateRunStatus(ctx, run.ID, store.PhaseRunning, ""))
	got, err := s.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, store.PhaseRunning, got.Status)
	assert.NotNil(t, got.StartedAt)

	require.NoError(t, s.UpdateRunStatus(ctx, run.ID, store.PhaseSucceeded, ""))
	got, err = s.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, store.PhaseSucceeded, got.Status)
	assert.NotNil(t, got.CompletedAt)
}

func TestFinding_CreateAndList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	run := &store.DiagnosticRun{TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending}
	require.NoError(t, s.CreateRun(ctx, run))

	f := &store.Finding{
		RunID: run.ID, Dimension: "health", Severity: "critical",
		Title: "Pod crashing", ResourceKind: "Pod",
	}
	require.NoError(t, s.CreateFinding(ctx, f))

	list, err := s.ListFindings(ctx, run.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "critical", list[0].Severity)
}

func TestSkill_Upsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sk := &store.Skill{Name: "pod-health-analyst", Dimension: "health",
		Prompt: "You are...", ToolsJSON: "[]", Source: "builtin", Enabled: true, Priority: 100}
	require.NoError(t, s.UpsertSkill(ctx, sk))

	sk.Priority = 50
	require.NoError(t, s.UpsertSkill(ctx, sk))

	got, err := s.GetSkill(ctx, "pod-health-analyst")
	require.NoError(t, err)
	assert.Equal(t, 50, got.Priority)
}

func TestDeleteSkill(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	sk := &store.Skill{
		Name:             "to-delete",
		Dimension:        "health",
		Prompt:           "test prompt",
		ToolsJSON:        `["kubectl_get"]`,
		RequiresDataJSON: `[]`,
		Source:           "cr",
		Enabled:          true,
		Priority:         100,
	}
	require.NoError(t, st.UpsertSkill(ctx, sk))

	got, err := st.GetSkill(ctx, "to-delete")
	require.NoError(t, err)
	assert.Equal(t, "to-delete", got.Name)

	require.NoError(t, st.DeleteSkill(ctx, "to-delete"))

	_, err = st.GetSkill(ctx, "to-delete")
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestDeleteSkill_NotFound(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	err := st.DeleteSkill(ctx, "nonexistent")
	assert.ErrorIs(t, err, store.ErrNotFound)
}

// helper to create a run and return its ID
func createTestRun(t *testing.T, s store.Store) string {
	t.Helper()
	ctx := context.Background()
	run := &store.DiagnosticRun{TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending}
	require.NoError(t, s.CreateRun(ctx, run))
	return run.ID
}

// helper to build a minimal Fix for a given runID
func newTestFix(runID string) *store.Fix {
	return &store.Fix{
		RunID:            runID,
		FindingTitle:     "CrashLoopBackOff detected",
		TargetKind:       "Deployment",
		TargetNamespace:  "default",
		TargetName:       "my-app",
		Strategy:         "restart",
		ApprovalRequired: true,
		PatchType:        "merge",
		PatchContent:     `{"spec":{}}`,
		Phase:            store.FixPhasePendingApproval,
		FindingID:        "finding-abc",
		BeforeSnapshot:   `{"before":"state"}`,
	}
}

func TestCreateFix_Success(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	runID := createTestRun(t, s)

	fix := newTestFix(runID)
	require.NoError(t, s.CreateFix(ctx, fix))
	assert.NotEmpty(t, fix.ID)
	assert.False(t, fix.CreatedAt.IsZero())
	assert.False(t, fix.UpdatedAt.IsZero())

	// round-trip: verify persisted data matches
	got, err := s.GetFix(ctx, fix.ID)
	require.NoError(t, err)
	assert.Equal(t, fix.ID, got.ID)
	assert.Equal(t, runID, got.RunID)
	assert.Equal(t, "CrashLoopBackOff detected", got.FindingTitle)
	assert.Equal(t, "Deployment", got.TargetKind)
	assert.Equal(t, "default", got.TargetNamespace)
	assert.Equal(t, "my-app", got.TargetName)
	assert.Equal(t, "restart", got.Strategy)
	assert.True(t, got.ApprovalRequired)
	assert.Equal(t, "merge", got.PatchType)
	assert.Equal(t, `{"spec":{}}`, got.PatchContent)
	assert.Equal(t, store.FixPhasePendingApproval, got.Phase)
	assert.Equal(t, "finding-abc", got.FindingID)
	assert.Equal(t, `{"before":"state"}`, got.BeforeSnapshot)
}

func TestGetFix_Success(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	runID := createTestRun(t, s)

	fix := newTestFix(runID)
	require.NoError(t, s.CreateFix(ctx, fix))

	got, err := s.GetFix(ctx, fix.ID)
	require.NoError(t, err)
	assert.Equal(t, fix.ID, got.ID)
	assert.Equal(t, store.FixPhasePendingApproval, got.Phase)
}

func TestGetFix_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetFix(ctx, "nonexistent-id")
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestListFixes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	runID := createTestRun(t, s)

	fix1 := newTestFix(runID)
	fix1.FindingTitle = "Fix One"
	require.NoError(t, s.CreateFix(ctx, fix1))

	fix2 := newTestFix(runID)
	fix2.FindingTitle = "Fix Two"
	require.NoError(t, s.CreateFix(ctx, fix2))

	list, err := s.ListFixes(ctx, store.ListOpts{})
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestListFixesByRun(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	runA := createTestRun(t, s)
	runB := createTestRun(t, s)

	fixA := newTestFix(runA)
	require.NoError(t, s.CreateFix(ctx, fixA))

	fixB1 := newTestFix(runB)
	fixB1.FindingTitle = "B Fix 1"
	require.NoError(t, s.CreateFix(ctx, fixB1))

	fixB2 := newTestFix(runB)
	fixB2.FindingTitle = "B Fix 2"
	require.NoError(t, s.CreateFix(ctx, fixB2))

	listA, err := s.ListFixesByRun(ctx, runA)
	require.NoError(t, err)
	require.Len(t, listA, 1)
	assert.Equal(t, fixA.ID, listA[0].ID)

	listB, err := s.ListFixesByRun(ctx, runB)
	require.NoError(t, err)
	assert.Len(t, listB, 2)
}

func TestUpdateFixPhase(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	runID := createTestRun(t, s)

	fix := newTestFix(runID)
	require.NoError(t, s.CreateFix(ctx, fix))

	require.NoError(t, s.UpdateFixPhase(ctx, fix.ID, store.FixPhaseApplying, "applying now"))

	got, err := s.GetFix(ctx, fix.ID)
	require.NoError(t, err)
	assert.Equal(t, store.FixPhaseApplying, got.Phase)
	assert.Equal(t, "applying now", got.Message)
}

func TestUpdateFixApproval(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	runID := createTestRun(t, s)

	fix := newTestFix(runID)
	require.NoError(t, s.CreateFix(ctx, fix))

	require.NoError(t, s.UpdateFixApproval(ctx, fix.ID, "admin"))

	got, err := s.GetFix(ctx, fix.ID)
	require.NoError(t, err)
	assert.Equal(t, "admin", got.ApprovedBy)
	assert.Equal(t, store.FixPhaseApproved, got.Phase)
}

func TestUpdateFixSnapshot(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	runID := createTestRun(t, s)

	fix := newTestFix(runID)
	require.NoError(t, s.CreateFix(ctx, fix))

	snapshot := `{"replicas":1,"image":"nginx:1.25"}`
	require.NoError(t, s.UpdateFixSnapshot(ctx, fix.ID, snapshot))

	got, err := s.GetFix(ctx, fix.ID)
	require.NoError(t, err)
	assert.Equal(t, snapshot, got.RollbackSnapshot)
}
