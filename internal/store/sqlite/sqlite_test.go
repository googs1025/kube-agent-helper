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
