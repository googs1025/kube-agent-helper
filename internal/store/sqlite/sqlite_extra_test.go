package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// ── Skills ───────────────────────────────────────────────────────────────────

func TestListSkills_OrderedByPriority(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert in mixed priority order; ListSkills should return ASC by priority.
	for _, sk := range []*store.Skill{
		{Name: "third", Dimension: "perf", Priority: 30, UpdatedAt: time.Now()},
		{Name: "first", Dimension: "health", Priority: 10, UpdatedAt: time.Now()},
		{Name: "second", Dimension: "security", Priority: 20, UpdatedAt: time.Now()},
	} {
		require.NoError(t, s.UpsertSkill(ctx, sk))
	}

	got, err := s.ListSkills(ctx)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "first", got[0].Name)
	assert.Equal(t, "second", got[1].Name)
	assert.Equal(t, "third", got[2].Name)
}

func TestListSkills_Empty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.ListSkills(context.Background())
	require.NoError(t, err)
	assert.Len(t, got, 0)
}

// ── Run logs ─────────────────────────────────────────────────────────────────

func TestRunLog_AppendAndListInOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	run := &store.DiagnosticRun{TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhaseRunning}
	require.NoError(t, s.CreateRun(ctx, run))

	for i, msg := range []string{"first", "second", "third"} {
		require.NoError(t, s.AppendRunLog(ctx, store.RunLog{
			RunID:     run.ID,
			Timestamp: time.Now().Format(time.RFC3339Nano),
			Type:      "step",
			Message:   msg,
			Data:      "",
		}), "append #%d", i)
	}

	all, err := s.ListRunLogs(ctx, run.ID, 0)
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, "first", all[0].Message)
	assert.Equal(t, "second", all[1].Message)
	assert.Equal(t, "third", all[2].Message)
	assert.True(t, all[0].ID < all[1].ID && all[1].ID < all[2].ID)
}

func TestRunLog_ListAfterID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	run := &store.DiagnosticRun{TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhaseRunning}
	require.NoError(t, s.CreateRun(ctx, run))

	for _, msg := range []string{"a", "b", "c"} {
		require.NoError(t, s.AppendRunLog(ctx, store.RunLog{
			RunID:     run.ID,
			Timestamp: time.Now().Format(time.RFC3339Nano),
			Type:      "info",
			Message:   msg,
		}))
	}

	all, err := s.ListRunLogs(ctx, run.ID, 0)
	require.NoError(t, err)
	require.Len(t, all, 3)

	// Tail strictly after the first entry.
	tail, err := s.ListRunLogs(ctx, run.ID, all[0].ID)
	require.NoError(t, err)
	assert.Len(t, tail, 2)
	assert.Equal(t, "b", tail[0].Message)
	assert.Equal(t, "c", tail[1].Message)
}

func TestRunLog_ListUnknownRunReturnsEmpty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.ListRunLogs(context.Background(), "nonexistent", 0)
	require.NoError(t, err)
	assert.Len(t, got, 0)
}

// ── Paginated runs ──────────────────────────────────────────────────────────

func seedRuns(t *testing.T, s store.Store, n int) []*store.DiagnosticRun {
	t.Helper()
	ctx := context.Background()
	out := make([]*store.DiagnosticRun, 0, n)
	for i := 0; i < n; i++ {
		r := &store.DiagnosticRun{
			TargetJSON: "{}",
			SkillsJSON: "[]",
			Status:     store.PhasePending,
		}
		require.NoError(t, s.CreateRun(ctx, r))
		out = append(out, r)
		// Ensure created_at differs so ORDER BY is deterministic.
		time.Sleep(2 * time.Millisecond)
	}
	return out
}

func TestListRunsPaginated_DefaultPageAndSize(t *testing.T) {
	s := newTestStore(t)
	seedRuns(t, s, 3)

	page, err := s.ListRunsPaginated(context.Background(), store.ListOpts{})
	require.NoError(t, err)
	assert.Equal(t, 3, page.Total)
	assert.Equal(t, 1, page.Page)
	assert.Equal(t, 20, page.PageSize)
	assert.Len(t, page.Items, 3)
}

func TestListRunsPaginated_MultiPage(t *testing.T) {
	s := newTestStore(t)
	seedRuns(t, s, 5)
	ctx := context.Background()

	p1, err := s.ListRunsPaginated(ctx, store.ListOpts{Page: 1, PageSize: 2, SortOrder: "desc"})
	require.NoError(t, err)
	assert.Equal(t, 5, p1.Total)
	assert.Len(t, p1.Items, 2)

	p2, err := s.ListRunsPaginated(ctx, store.ListOpts{Page: 2, PageSize: 2, SortOrder: "desc"})
	require.NoError(t, err)
	assert.Len(t, p2.Items, 2)

	p3, err := s.ListRunsPaginated(ctx, store.ListOpts{Page: 3, PageSize: 2, SortOrder: "desc"})
	require.NoError(t, err)
	assert.Len(t, p3.Items, 1)

	// No overlap across pages.
	seen := map[string]bool{}
	for _, r := range append(append(p1.Items, p2.Items...), p3.Items...) {
		assert.False(t, seen[r.ID], "duplicate id %q across pages", r.ID)
		seen[r.ID] = true
	}
}

func TestListRunsPaginated_ASCOrder(t *testing.T) {
	s := newTestStore(t)
	runs := seedRuns(t, s, 3)
	ctx := context.Background()

	asc, err := s.ListRunsPaginated(ctx, store.ListOpts{Page: 1, PageSize: 10, SortOrder: "asc"})
	require.NoError(t, err)
	require.Len(t, asc.Items, 3)
	// First seeded should be the earliest (oldest first).
	assert.Equal(t, runs[0].ID, asc.Items[0].ID)
}

func TestListRunsPaginated_PageSizeCappedAt100(t *testing.T) {
	s := newTestStore(t)
	seedRuns(t, s, 5)

	page, err := s.ListRunsPaginated(context.Background(), store.ListOpts{Page: 1, PageSize: 9999})
	require.NoError(t, err)
	assert.Equal(t, 100, page.PageSize, "PageSize > 100 should be capped to 100")
}

func TestListRunsPaginated_SortByWhitelist(t *testing.T) {
	s := newTestStore(t)
	seedRuns(t, s, 2)

	// Bogus SortBy must silently fall back to created_at — not panic, not error.
	page, err := s.ListRunsPaginated(context.Background(), store.ListOpts{
		Page:     1,
		PageSize: 10,
		SortBy:   "; DROP TABLE diagnostic_runs;--",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, page.Total, "table must still be intact after malicious SortBy")
}

func TestListRunsPaginated_FilterByPhase(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r1 := &store.DiagnosticRun{TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending}
	r2 := &store.DiagnosticRun{TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhaseRunning}
	require.NoError(t, s.CreateRun(ctx, r1))
	require.NoError(t, s.CreateRun(ctx, r2))

	page, err := s.ListRunsPaginated(ctx, store.ListOpts{
		Page: 1, PageSize: 10,
		Filters: map[string]string{"phase": string(store.PhaseRunning)},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, r2.ID, page.Items[0].ID)
}

func TestListRunsPaginated_FilterByCluster(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r1 := &store.DiagnosticRun{ClusterName: "prod", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending}
	r2 := &store.DiagnosticRun{ClusterName: "dev", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending}
	require.NoError(t, s.CreateRun(ctx, r1))
	require.NoError(t, s.CreateRun(ctx, r2))

	page, err := s.ListRunsPaginated(ctx, store.ListOpts{Page: 1, PageSize: 10, ClusterName: "prod"})
	require.NoError(t, err)
	assert.Equal(t, 1, page.Total)
	assert.Equal(t, r1.ID, page.Items[0].ID)
}

// ── Paginated fixes ──────────────────────────────────────────────────────────

func TestListFixesPaginated_FilterByPhaseAndNamespace(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, f := range []*store.Fix{
		{TargetKind: "Deployment", TargetNamespace: "default", TargetName: "a", Phase: store.FixPhasePendingApproval},
		{TargetKind: "Deployment", TargetNamespace: "default", TargetName: "b", Phase: store.FixPhaseSucceeded},
		{TargetKind: "Deployment", TargetNamespace: "kube-system", TargetName: "c", Phase: store.FixPhaseSucceeded},
	} {
		require.NoError(t, s.CreateFix(ctx, f))
	}

	page, err := s.ListFixesPaginated(ctx, store.ListOpts{
		Page: 1, PageSize: 10,
		Filters: map[string]string{
			"phase":     string(store.FixPhaseSucceeded),
			"namespace": "default",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "b", page.Items[0].TargetName)
}

func TestListFixesPaginated_DefaultsAndCap(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		require.NoError(t, s.CreateFix(ctx, &store.Fix{
			TargetKind: "Deployment", TargetName: "x", Phase: store.FixPhasePendingApproval,
		}))
	}
	page, err := s.ListFixesPaginated(ctx, store.ListOpts{PageSize: 9999})
	require.NoError(t, err)
	assert.Equal(t, 1, page.Page)
	assert.Equal(t, 100, page.PageSize)
	assert.Equal(t, 3, page.Total)
}

// ── Paginated events ─────────────────────────────────────────────────────────

func TestListEventsPaginated_PageAndFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Now()
	events := []*store.Event{
		{UID: "u1", Namespace: "ns1", Kind: "Pod", Name: "p1", Reason: "BackOff", Type: "Warning", Count: 1, FirstTime: now, LastTime: now},
		{UID: "u2", Namespace: "ns1", Kind: "Pod", Name: "p2", Reason: "BackOff", Type: "Warning", Count: 1, FirstTime: now, LastTime: now},
		{UID: "u3", Namespace: "ns2", Kind: "Pod", Name: "p3", Reason: "Created", Type: "Normal", Count: 1, FirstTime: now, LastTime: now},
	}
	for _, ev := range events {
		require.NoError(t, s.UpsertEvent(ctx, ev))
	}

	page, err := s.ListEventsPaginated(ctx, store.ListEventsOpts{Namespace: "ns1"}, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 2, page.Total)
	assert.Len(t, page.Items, 2)

	// Multi-page split.
	p1, err := s.ListEventsPaginated(ctx, store.ListEventsOpts{}, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, p1.Total)
	assert.Len(t, p1.Items, 2)
	p2, err := s.ListEventsPaginated(ctx, store.ListEventsOpts{}, 2, 2)
	require.NoError(t, err)
	assert.Len(t, p2.Items, 1)
}

func TestListEventsPaginated_SinceMinutes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	require.NoError(t, s.UpsertEvent(ctx, &store.Event{
		UID: "old", Namespace: "ns", Kind: "Pod", Name: "p", Reason: "X", Type: "Warning",
		FirstTime: now.Add(-2 * time.Hour), LastTime: now.Add(-2 * time.Hour), Count: 1,
	}))
	require.NoError(t, s.UpsertEvent(ctx, &store.Event{
		UID: "new", Namespace: "ns", Kind: "Pod", Name: "p", Reason: "Y", Type: "Warning",
		FirstTime: now, LastTime: now, Count: 1,
	}))

	// Last 30 minutes only.
	page, err := s.ListEventsPaginated(ctx, store.ListEventsOpts{SinceMinutes: 30}, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, page.Total)
	assert.Equal(t, "new", page.Items[0].UID)
}

func TestListEventsPaginated_DefaultsAndCap(t *testing.T) {
	s := newTestStore(t)
	page, err := s.ListEventsPaginated(context.Background(), store.ListEventsOpts{}, 0, 9999)
	require.NoError(t, err)
	assert.Equal(t, 1, page.Page)
	assert.Equal(t, 100, page.PageSize)
}

// ── Notification configs ────────────────────────────────────────────────────

func TestNotificationConfig_LifecycleAndIDAutoFill(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cfg := &store.NotificationConfig{
		Name: "ops-slack", Type: "slack",
		WebhookURL: "https://slack.example/hook",
		Events:     "fix.applied,run.failed",
		Enabled:    true,
	}
	require.NoError(t, s.CreateNotificationConfig(ctx, cfg))
	assert.NotEmpty(t, cfg.ID, "ID should be auto-filled")

	got, err := s.GetNotificationConfig(ctx, cfg.ID)
	require.NoError(t, err)
	assert.Equal(t, "ops-slack", got.Name)
	assert.True(t, got.Enabled)

	// Update.
	cfg.Name = "ops-slack-v2"
	cfg.Enabled = false
	require.NoError(t, s.UpdateNotificationConfig(ctx, cfg))

	got, err = s.GetNotificationConfig(ctx, cfg.ID)
	require.NoError(t, err)
	assert.Equal(t, "ops-slack-v2", got.Name)
	assert.False(t, got.Enabled)

	// List shows it.
	all, err := s.ListNotificationConfigs(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, cfg.ID, all[0].ID)

	// Delete.
	require.NoError(t, s.DeleteNotificationConfig(ctx, cfg.ID))
	_, err = s.GetNotificationConfig(ctx, cfg.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestNotificationConfig_GetOrUpdateOrDeleteMissing(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetNotificationConfig(ctx, "ghost")
	assert.ErrorIs(t, err, store.ErrNotFound)

	err = s.UpdateNotificationConfig(ctx, &store.NotificationConfig{ID: "ghost", Name: "x", Type: "slack"})
	assert.ErrorIs(t, err, store.ErrNotFound)

	err = s.DeleteNotificationConfig(ctx, "ghost")
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestNotificationConfig_ListEmpty(t *testing.T) {
	s := newTestStore(t)
	all, err := s.ListNotificationConfigs(context.Background())
	require.NoError(t, err)
	assert.Empty(t, all)
}

// ── Batch operations ────────────────────────────────────────────────────────

func TestDeleteRuns_CascadesToChildTables(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	run := &store.DiagnosticRun{TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhaseSucceeded}
	require.NoError(t, s.CreateRun(ctx, run))
	require.NoError(t, s.CreateFinding(ctx, &store.Finding{
		RunID: run.ID, Dimension: "health", Severity: "high", Title: "t",
	}))
	require.NoError(t, s.CreateFix(ctx, &store.Fix{
		RunID: run.ID, TargetKind: "Deployment", TargetName: "x", Phase: store.FixPhaseSucceeded,
	}))
	require.NoError(t, s.AppendRunLog(ctx, store.RunLog{
		RunID: run.ID, Timestamp: time.Now().Format(time.RFC3339Nano), Type: "info", Message: "hi",
	}))

	require.NoError(t, s.DeleteRuns(ctx, []string{run.ID}))

	_, err := s.GetRun(ctx, run.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)

	logs, err := s.ListRunLogs(ctx, run.ID, 0)
	require.NoError(t, err)
	assert.Empty(t, logs, "child run_logs should be cascaded")

	findings, err := s.ListFindings(ctx, run.ID)
	require.NoError(t, err)
	assert.Empty(t, findings, "child findings should be cascaded")

	fixes, err := s.ListFixesByRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Empty(t, fixes, "child fixes should be cascaded")
}

func TestDeleteRuns_EmptyIsNoop(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.DeleteRuns(context.Background(), nil))
	require.NoError(t, s.DeleteRuns(context.Background(), []string{}))
}

func TestDeleteRuns_UnknownIDsAreIdempotent(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.DeleteRuns(context.Background(), []string{"nope-1", "nope-2"}))
}

func TestBatchUpdateFixPhase_UpdatesAllListed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	f1 := &store.Fix{TargetKind: "Deployment", TargetName: "a", Phase: store.FixPhasePendingApproval}
	f2 := &store.Fix{TargetKind: "Deployment", TargetName: "b", Phase: store.FixPhasePendingApproval}
	f3 := &store.Fix{TargetKind: "Deployment", TargetName: "c", Phase: store.FixPhasePendingApproval}
	require.NoError(t, s.CreateFix(ctx, f1))
	require.NoError(t, s.CreateFix(ctx, f2))
	require.NoError(t, s.CreateFix(ctx, f3))

	require.NoError(t, s.BatchUpdateFixPhase(ctx,
		[]string{f1.ID, f2.ID}, store.FixPhaseApproved, "ok by user"))

	got, _ := s.GetFix(ctx, f1.ID)
	assert.Equal(t, store.FixPhaseApproved, got.Phase)
	assert.Equal(t, "ok by user", got.Message)

	got, _ = s.GetFix(ctx, f2.ID)
	assert.Equal(t, store.FixPhaseApproved, got.Phase)

	// f3 untouched.
	got, _ = s.GetFix(ctx, f3.ID)
	assert.Equal(t, store.FixPhasePendingApproval, got.Phase)
}

func TestBatchUpdateFixPhase_EmptyIsNoop(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.BatchUpdateFixPhase(context.Background(), nil, store.FixPhaseFailed, ""))
	require.NoError(t, s.BatchUpdateFixPhase(context.Background(), []string{}, store.FixPhaseFailed, ""))
}
