package sqlite_test

import (
	"context"
	"os"
	"testing"
	"time"

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

func newTestEvent(uid, namespace, name, reason string, lastTime time.Time) *store.Event {
	return &store.Event{
		UID:       uid,
		Namespace: namespace,
		Kind:      "Pod",
		Name:      name,
		Reason:    reason,
		Message:   "test message for " + reason,
		Type:      "Warning",
		Count:     1,
		FirstTime: lastTime.Add(-5 * time.Minute),
		LastTime:  lastTime,
	}
}

func TestUpsertAndListEvents(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	e1 := newTestEvent("uid-1", "default", "pod-a", "OOMKilled", now)
	e2 := newTestEvent("uid-2", "default", "pod-b", "CrashLoopBackOff", now)
	e3 := newTestEvent("uid-3", "kube-system", "pod-c", "BackOff", now)

	require.NoError(t, s.UpsertEvent(ctx, e1))
	require.NoError(t, s.UpsertEvent(ctx, e2))
	require.NoError(t, s.UpsertEvent(ctx, e3))

	// no filter → all 3
	all, err := s.ListEvents(ctx, store.ListEventsOpts{})
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// namespace filter → only 2 in "default"
	byNS, err := s.ListEvents(ctx, store.ListEventsOpts{Namespace: "default"})
	require.NoError(t, err)
	assert.Len(t, byNS, 2)

	// upsert same UID → should update, not duplicate
	e1Updated := newTestEvent("uid-1", "default", "pod-a", "OOMKilled", now.Add(1*time.Minute))
	e1Updated.Count = 5
	require.NoError(t, s.UpsertEvent(ctx, e1Updated))

	allAfterUpsert, err := s.ListEvents(ctx, store.ListEventsOpts{})
	require.NoError(t, err)
	assert.Len(t, allAfterUpsert, 3, "upsert should not create a duplicate row")

	// verify updated count
	defaultEvents, err := s.ListEvents(ctx, store.ListEventsOpts{Namespace: "default", Name: "pod-a"})
	require.NoError(t, err)
	require.Len(t, defaultEvents, 1)
	assert.EqualValues(t, 5, defaultEvents[0].Count)
}

func TestListEvents_SinceMinutes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	old := newTestEvent("uid-old", "default", "pod-old", "OOMKilled", now.Add(-2*time.Hour))
	recent := newTestEvent("uid-recent", "default", "pod-new", "BackOff", now.Add(-5*time.Minute))

	require.NoError(t, s.UpsertEvent(ctx, old))
	require.NoError(t, s.UpsertEvent(ctx, recent))

	// SinceMinutes=60 → only the recent event (5 min ago) should appear
	events, err := s.ListEvents(ctx, store.ListEventsOpts{SinceMinutes: 60})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "uid-recent", events[0].UID)
}

func TestInsertAndQueryMetricHistory(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	snap1 := &store.MetricSnapshot{Query: "cpu_usage", LabelsJSON: `{"pod":"a"}`, Value: 0.8, Ts: now.Add(-30 * time.Minute)}
	snap2 := &store.MetricSnapshot{Query: "cpu_usage", LabelsJSON: `{"pod":"b"}`, Value: 0.5, Ts: now.Add(-90 * time.Minute)}
	snap3 := &store.MetricSnapshot{Query: "mem_usage", LabelsJSON: `{"pod":"a"}`, Value: 512.0, Ts: now.Add(-10 * time.Minute)}

	require.NoError(t, s.InsertMetricSnapshot(ctx, snap1))
	require.NoError(t, s.InsertMetricSnapshot(ctx, snap2))
	require.NoError(t, s.InsertMetricSnapshot(ctx, snap3))

	// query for cpu_usage within last 120 minutes → both cpu_usage snaps
	cpuSnaps, err := s.QueryMetricHistory(ctx, "cpu_usage", 120)
	require.NoError(t, err)
	assert.Len(t, cpuSnaps, 2)
	for _, snap := range cpuSnaps {
		assert.Equal(t, "cpu_usage", snap.Query)
	}

	// query for mem_usage within last 120 minutes → only snap3
	memSnaps, err := s.QueryMetricHistory(ctx, "mem_usage", 120)
	require.NoError(t, err)
	require.Len(t, memSnaps, 1)
	assert.Equal(t, 512.0, memSnaps[0].Value)
}

func TestPurgeOldEvents(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	old := newTestEvent("uid-old", "default", "pod-old", "OOMKilled", now.Add(-2*time.Hour))
	recent := newTestEvent("uid-recent", "default", "pod-new", "BackOff", now.Add(-5*time.Minute))

	require.NoError(t, s.UpsertEvent(ctx, old))
	require.NoError(t, s.UpsertEvent(ctx, recent))

	// purge everything older than 1 hour ago
	require.NoError(t, s.PurgeOldEvents(ctx, now.Add(-1*time.Hour)))

	remaining, err := s.ListEvents(ctx, store.ListEventsOpts{})
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "uid-recent", remaining[0].UID)
}

func TestPurgeOldMetrics(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()

	oldSnap := &store.MetricSnapshot{Query: "cpu_usage", LabelsJSON: `{}`, Value: 0.9, Ts: now.Add(-2 * time.Hour)}
	recentSnap := &store.MetricSnapshot{Query: "cpu_usage", LabelsJSON: `{}`, Value: 0.3, Ts: now.Add(-5 * time.Minute)}

	require.NoError(t, s.InsertMetricSnapshot(ctx, oldSnap))
	require.NoError(t, s.InsertMetricSnapshot(ctx, recentSnap))

	// purge everything older than 1 hour ago
	require.NoError(t, s.PurgeOldMetrics(ctx, now.Add(-1*time.Hour)))

	// use a wide window to get all remaining rows
	remaining, err := s.QueryMetricHistory(ctx, "cpu_usage", 120)
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.InDelta(t, 0.3, remaining[0].Value, 0.001)
}

func TestListRunsFilterByCluster(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	// Create runs in different clusters
	run1 := &store.DiagnosticRun{ID: "r1", ClusterName: "local", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending}
	run2 := &store.DiagnosticRun{ID: "r2", ClusterName: "prod", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending}
	run3 := &store.DiagnosticRun{ID: "r3", ClusterName: "prod", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending}

	for _, r := range []*store.DiagnosticRun{run1, run2, run3} {
		if err := st.CreateRun(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}

	// Filter by "prod" cluster
	runs, err := st.ListRuns(context.Background(), store.ListOpts{ClusterName: "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs for prod cluster, got %d", len(runs))
	}
	for _, r := range runs {
		if r.ClusterName != "prod" {
			t.Errorf("expected ClusterName=prod, got %s", r.ClusterName)
		}
	}

	// Filter by "local" cluster
	runs, err = st.ListRuns(context.Background(), store.ListOpts{ClusterName: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run for local cluster, got %d", len(runs))
	}

	// No filter = all clusters
	runs, err = st.ListRuns(context.Background(), store.ListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs total, got %d", len(runs))
	}
}

func TestListFixesFilterByCluster(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	// Create a run first (fixes reference runs)
	run := &store.DiagnosticRun{ID: "r1", ClusterName: "local", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending}
	if err := st.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	fix1 := &store.Fix{ID: "f1", RunID: "r1", ClusterName: "local", FindingTitle: "t1", TargetKind: "Deployment", TargetNamespace: "ns", TargetName: "app", PatchContent: "{}", Phase: store.FixPhasePendingApproval}
	fix2 := &store.Fix{ID: "f2", RunID: "r1", ClusterName: "staging", FindingTitle: "t2", TargetKind: "Deployment", TargetNamespace: "ns", TargetName: "app2", PatchContent: "{}", Phase: store.FixPhasePendingApproval}

	for _, f := range []*store.Fix{fix1, fix2} {
		if err := st.CreateFix(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}

	// Filter by staging
	fixes, err := st.ListFixes(context.Background(), store.ListOpts{ClusterName: "staging"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fixes) != 1 {
		t.Fatalf("expected 1 fix for staging, got %d", len(fixes))
	}
	if fixes[0].ClusterName != "staging" {
		t.Errorf("expected ClusterName=staging, got %s", fixes[0].ClusterName)
	}

	// No filter = all
	fixes, err = st.ListFixes(context.Background(), store.ListOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fixes) != 2 {
		t.Fatalf("expected 2 fixes total, got %d", len(fixes))
	}
}

func TestListEventsFilterByCluster(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	now := time.Now()
	ev1 := &store.Event{UID: "e1", ClusterName: "local", Namespace: "default", Kind: "Pod", Name: "pod-1", Reason: "OOMKilled", Message: "mem", Type: "Warning", Count: 1, FirstTime: now, LastTime: now}
	ev2 := &store.Event{UID: "e2", ClusterName: "prod", Namespace: "default", Kind: "Pod", Name: "pod-2", Reason: "BackOff", Message: "crash", Type: "Warning", Count: 1, FirstTime: now, LastTime: now}

	for _, e := range []*store.Event{ev1, ev2} {
		if err := st.UpsertEvent(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}

	// Filter by prod
	events, err := st.ListEvents(context.Background(), store.ListEventsOpts{ClusterName: "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event for prod, got %d", len(events))
	}
	if events[0].ClusterName != "prod" {
		t.Errorf("expected ClusterName=prod, got %s", events[0].ClusterName)
	}

	// No filter = all
	events, err = st.ListEvents(context.Background(), store.ListEventsOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events total, got %d", len(events))
	}
}

func TestClusterNameDefaultsToLocal(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()

	// Create run without ClusterName — should default to "local"
	run := &store.DiagnosticRun{ID: "r1", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending}
	if err := st.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	got, err := st.GetRun(context.Background(), "r1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ClusterName != "local" {
		t.Errorf("expected default ClusterName=local, got %q", got.ClusterName)
	}

	// Create finding without ClusterName
	f := &store.Finding{ID: "f1", RunID: "r1", Dimension: "health", Severity: "high", Title: "test"}
	if err := st.CreateFinding(context.Background(), f); err != nil {
		t.Fatal(err)
	}

	findings, err := st.ListFindings(context.Background(), "r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].ClusterName != "local" {
		t.Errorf("expected finding ClusterName=local, got %q", findings[0].ClusterName)
	}

	// Create fix without ClusterName
	fix := &store.Fix{
		ID: "fix1", RunID: "r1", FindingTitle: "crash",
		TargetKind: "Deployment", TargetNamespace: "default", TargetName: "app",
		PatchContent: "{}", Phase: store.FixPhasePendingApproval,
	}
	if err := st.CreateFix(context.Background(), fix); err != nil {
		t.Fatal(err)
	}

	gotFix, err := st.GetFix(context.Background(), "fix1")
	if err != nil {
		t.Fatal(err)
	}
	if gotFix.ClusterName != "local" {
		t.Errorf("expected fix ClusterName=local, got %q", gotFix.ClusterName)
	}

	// Upsert event without ClusterName
	ev := &store.Event{
		UID: "ev1", Namespace: "default", Kind: "Pod", Name: "pod-1",
		Reason: "OOM", Message: "oom", Type: "Warning", Count: 1,
		FirstTime: time.Now(), LastTime: time.Now(),
	}
	if err := st.UpsertEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}

	events, err := st.ListEvents(context.Background(), store.ListEventsOpts{ClusterName: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ClusterName != "local" {
		t.Errorf("expected event ClusterName=local, got %v events", len(events))
	}
}

func TestFilterByNonExistentCluster_ReturnsEmpty(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Create data only in "local"
	run := &store.DiagnosticRun{ClusterName: "local", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending}
	require.NoError(t, st.CreateRun(ctx, run))

	fix := &store.Fix{
		RunID: run.ID, ClusterName: "local", FindingTitle: "t",
		TargetKind: "Deployment", TargetNamespace: "ns", TargetName: "app",
		PatchContent: "{}", Phase: store.FixPhasePendingApproval,
	}
	require.NoError(t, st.CreateFix(ctx, fix))

	ev := &store.Event{
		UID: "ev1", ClusterName: "local", Namespace: "default", Kind: "Pod", Name: "p",
		Reason: "r", Message: "m", Type: "Warning", Count: 1,
		FirstTime: time.Now(), LastTime: time.Now(),
	}
	require.NoError(t, st.UpsertEvent(ctx, ev))

	// Query with non-existent cluster → empty results
	runs, err := st.ListRuns(ctx, store.ListOpts{ClusterName: "nonexistent"})
	require.NoError(t, err)
	assert.Empty(t, runs)

	fixes, err := st.ListFixes(ctx, store.ListOpts{ClusterName: "nonexistent"})
	require.NoError(t, err)
	assert.Empty(t, fixes)

	events, err := st.ListEvents(ctx, store.ListEventsOpts{ClusterName: "nonexistent"})
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestClusterNamePreservedWhenExplicitlySet(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Run with explicit ClusterName
	run := &store.DiagnosticRun{ClusterName: "prod", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending}
	require.NoError(t, st.CreateRun(ctx, run))

	got, err := st.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, "prod", got.ClusterName, "explicit ClusterName should be preserved, not overwritten with 'local'")

	// Finding with explicit ClusterName
	f := &store.Finding{RunID: run.ID, ClusterName: "prod", Dimension: "health", Severity: "high", Title: "test"}
	require.NoError(t, st.CreateFinding(ctx, f))

	findings, err := st.ListFindings(ctx, run.ID)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "prod", findings[0].ClusterName)

	// Fix with explicit ClusterName
	fix := &store.Fix{
		RunID: run.ID, ClusterName: "prod", FindingTitle: "t",
		TargetKind: "Deployment", TargetNamespace: "ns", TargetName: "app",
		PatchContent: "{}", Phase: store.FixPhasePendingApproval,
	}
	require.NoError(t, st.CreateFix(ctx, fix))

	gotFix, err := st.GetFix(ctx, fix.ID)
	require.NoError(t, err)
	assert.Equal(t, "prod", gotFix.ClusterName)

	// Event with explicit ClusterName
	ev := &store.Event{
		UID: "ev1", ClusterName: "prod", Namespace: "default", Kind: "Pod", Name: "p",
		Reason: "r", Message: "m", Type: "Warning", Count: 1,
		FirstTime: time.Now(), LastTime: time.Now(),
	}
	require.NoError(t, st.UpsertEvent(ctx, ev))

	events, err := st.ListEvents(ctx, store.ListEventsOpts{ClusterName: "prod"})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "prod", events[0].ClusterName)
}
