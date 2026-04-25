package reconciler_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
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

// ── memStore ──────────────────────────────────────────────────────────────────

type memStore struct {
	runs     map[string]*store.DiagnosticRun
	findings map[string][]*store.Finding
}

func newMemStore() *memStore {
	return &memStore{
		runs:     map[string]*store.DiagnosticRun{},
		findings: map[string][]*store.Finding{},
	}
}
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
		r.Message = msg
	}
	return nil
}
func (m *memStore) ListRuns(_ context.Context, _ store.ListOpts) ([]*store.DiagnosticRun, error) {
	return nil, nil
}
func (m *memStore) CreateFinding(_ context.Context, f *store.Finding) error {
	m.findings[f.RunID] = append(m.findings[f.RunID], f)
	return nil
}
func (m *memStore) ListFindings(_ context.Context, runID string) ([]*store.Finding, error) {
	return m.findings[runID], nil
}
func (m *memStore) UpsertSkill(_ context.Context, _ *store.Skill) error { return nil }
func (m *memStore) ListSkills(_ context.Context) ([]*store.Skill, error) { return nil, nil }
func (m *memStore) GetSkill(_ context.Context, _ string) (*store.Skill, error) {
	return nil, nil
}
func (m *memStore) DeleteSkill(_ context.Context, _ string) error                          { return nil }
func (m *memStore) CreateFix(_ context.Context, _ *store.Fix) error                        { return nil }
func (m *memStore) GetFix(_ context.Context, _ string) (*store.Fix, error)                 { return nil, store.ErrNotFound }
func (m *memStore) ListFixes(_ context.Context, _ store.ListOpts) ([]*store.Fix, error)    { return nil, nil }
func (m *memStore) ListFixesByRun(_ context.Context, _ string) ([]*store.Fix, error)       { return nil, nil }
func (m *memStore) UpdateFixPhase(_ context.Context, _ string, _ store.FixPhase, _ string) error { return nil }
func (m *memStore) UpdateFixApproval(_ context.Context, _ string, _ string) error          { return nil }
func (m *memStore) UpdateFixSnapshot(_ context.Context, _ string, _ string) error          { return nil }
func (m *memStore) UpsertEvent(_ context.Context, _ *store.Event) error { return nil }
func (m *memStore) ListEvents(_ context.Context, _ store.ListEventsOpts) ([]*store.Event, error) {
	return nil, nil
}
func (m *memStore) InsertMetricSnapshot(_ context.Context, _ *store.MetricSnapshot) error {
	return nil
}
func (m *memStore) QueryMetricHistory(_ context.Context, _ string, _ int) ([]*store.MetricSnapshot, error) {
	return nil, nil
}
func (m *memStore) AppendRunLog(_ context.Context, _ store.RunLog) error { return nil }
func (m *memStore) ListRunLogs(_ context.Context, _ string, _ int64) ([]store.RunLog, error) {
	return nil, nil
}
func (m *memStore) PurgeOldEvents(_ context.Context, _ time.Time) error  { return nil }
func (m *memStore) PurgeOldMetrics(_ context.Context, _ time.Time) error { return nil }
func (m *memStore) Close() error                                         { return nil }

// ── helpers ───────────────────────────────────────────────────────────────────

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = k8saiV1.AddToScheme(s)
	_ = batchv1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = rbacv1.AddToScheme(s)
	_ = networkingv1.AddToScheme(s)
	return s
}

func testRun() *k8saiV1.DiagnosticRun {
	return &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-run", Namespace: "default", UID: "uid-1",
		},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:         k8saiV1.TargetSpec{Scope: "namespace", Namespaces: []string{"default"}},
			Skills:         []string{"pod-health-analyst"},
			ModelConfigRef: "claude-default",
		},
	}
}

func testSkill() *store.Skill {
	return &store.Skill{
		Name: "pod-health-analyst", Dimension: "health",
		Prompt: "You are...", ToolsJSON: "[]", Enabled: true,
	}
}

type mockSkillProvider struct {
	skills []*store.Skill
}

func (m *mockSkillProvider) ListEnabled(_ context.Context) ([]*store.Skill, error) {
	var enabled []*store.Skill
	for _, s := range m.skills {
		if s.Enabled {
			enabled = append(enabled, s)
		}
	}
	return enabled, nil
}

func testTranslator() *translator.Translator {
	return translator.New(translator.Config{
		AgentImage: "agent:test", ControllerURL: "http://ctrl:8080",
	}, &mockSkillProvider{skills: []*store.Skill{testSkill()}})
}

func reconcileOnce(t *testing.T, r *reconciler.DiagnosticRunReconciler) ctrl.Result {
	t.Helper()
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-run", Namespace: "default"},
	})
	require.NoError(t, err)
	return result
}

func getRunStatus(t *testing.T, cl interface{ Get(context.Context, types.NamespacedName, ...interface{}) error }, name string) k8saiV1.DiagnosticRun {
	t.Helper()
	// Use the reconciler's client directly via type assertion
	return k8saiV1.DiagnosticRun{}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestRunReconciler_PendingToRunning(t *testing.T) {
	run := testRun()
	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run).
		WithStatusSubresource(run).
		Build()

	ms := newMemStore()
	r := &reconciler.DiagnosticRunReconciler{
		Client: fakeClient, Store: ms, Translator: testTranslator(),
	}

	result := reconcileOnce(t, r)
	assert.NotZero(t, result.RequeueAfter, "should requeue to poll Job status")

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "test-run", Namespace: "default"}, &updated))
	assert.Equal(t, "Running", updated.Status.Phase)
	assert.NotNil(t, updated.Status.StartedAt)
}

func TestRunReconciler_RunningToSucceeded(t *testing.T) {
	run := testRun()
	run.Status.Phase = "Running"
	run.Status.ReportID = "uid-1"

	// Create a succeeded Job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run", Namespace: "default",
		},
		Status: batchv1.JobStatus{Succeeded: 1},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run, job).
		WithStatusSubresource(run).
		Build()

	// Pre-populate store
	ms := newMemStore()
	ms.runs["uid-1"] = &store.DiagnosticRun{ID: "uid-1", Status: store.PhaseRunning}
	ms.findings["uid-1"] = []*store.Finding{
		{RunID: "uid-1", Dimension: "health", Severity: "critical", Title: "Pod CrashLooping", ResourceKind: "Pod", ResourceName: "nginx"},
		{RunID: "uid-1", Dimension: "health", Severity: "medium", Title: "High restart count", ResourceKind: "Pod", ResourceName: "nginx"},
	}

	r := &reconciler.DiagnosticRunReconciler{
		Client: fakeClient, Store: ms, Translator: testTranslator(),
	}

	result := reconcileOnce(t, r)
	assert.Zero(t, result.RequeueAfter, "terminal state should not requeue")

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "test-run", Namespace: "default"}, &updated))

	assert.Equal(t, "Succeeded", updated.Status.Phase)
	assert.NotNil(t, updated.Status.CompletedAt)
	assert.Equal(t, "agent job completed successfully", updated.Status.Message)

	// Findings written back
	assert.Len(t, updated.Status.Findings, 2)
	assert.Equal(t, 1, updated.Status.FindingCounts["critical"])
	assert.Equal(t, 1, updated.Status.FindingCounts["medium"])
	assert.Equal(t, "Pod CrashLooping", updated.Status.Findings[0].Title)

	// Store also updated
	assert.Equal(t, store.PhaseSucceeded, ms.runs["uid-1"].Status)
}

func TestRunReconciler_RunningToFailed(t *testing.T) {
	run := testRun()
	run.Status.Phase = "Running"
	run.Status.ReportID = "uid-1"

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run", Namespace: "default",
		},
		Status: batchv1.JobStatus{
			Failed: 1,
			Conditions: []batchv1.JobCondition{{
				Type:    batchv1.JobFailed,
				Status:  "True",
				Message: "Back-off limit exceeded",
			}},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run, job).
		WithStatusSubresource(run).
		Build()

	ms := newMemStore()
	ms.runs["uid-1"] = &store.DiagnosticRun{ID: "uid-1", Status: store.PhaseRunning}

	r := &reconciler.DiagnosticRunReconciler{
		Client: fakeClient, Store: ms, Translator: testTranslator(),
	}

	result := reconcileOnce(t, r)
	assert.Zero(t, result.RequeueAfter)

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "test-run", Namespace: "default"}, &updated))

	assert.Equal(t, "Failed", updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "Back-off limit exceeded")
	assert.NotNil(t, updated.Status.CompletedAt)
}

func TestRunReconciler_RunningJobStillActive(t *testing.T) {
	run := testRun()
	run.Status.Phase = "Running"

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run", Namespace: "default",
		},
		Status: batchv1.JobStatus{Active: 1},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run, job).
		WithStatusSubresource(run).
		Build()

	ms := newMemStore()
	r := &reconciler.DiagnosticRunReconciler{
		Client: fakeClient, Store: ms, Translator: testTranslator(),
	}

	result := reconcileOnce(t, r)
	assert.NotZero(t, result.RequeueAfter, "should requeue while job is active")

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "test-run", Namespace: "default"}, &updated))
	assert.Equal(t, "Running", updated.Status.Phase, "should stay Running")
}

func TestRunReconciler_RunningPodImagePullBackOff(t *testing.T) {
	run := testRun()
	run.Status.Phase = "Running"
	run.Status.ReportID = "uid-1"
	now := metav1.Now()
	run.Status.StartedAt = &now

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run", Namespace: "default",
		},
		Status: batchv1.JobStatus{Active: 1},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run-abc", Namespace: "default",
			Labels: map[string]string{"job-name": "agent-test-run"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "agent",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason:  "ImagePullBackOff",
						Message: "Back-off pulling image \"ghcr.io/kube-agent-helper/agent-runtime:latest\"",
					},
				},
			}},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run, job, pod).
		WithStatusSubresource(run).
		Build()

	ms := newMemStore()
	ms.runs["uid-1"] = &store.DiagnosticRun{ID: "uid-1", Status: store.PhaseRunning}

	r := &reconciler.DiagnosticRunReconciler{
		Client: fakeClient, Store: ms, Translator: testTranslator(),
	}

	result := reconcileOnce(t, r)
	assert.NotZero(t, result.RequeueAfter, "should still requeue — not terminal yet")

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "test-run", Namespace: "default"}, &updated))
	assert.Equal(t, "Running", updated.Status.Phase, "phase stays Running")
	assert.Contains(t, updated.Status.Message, "ImagePullBackOff")
}

func TestRunReconciler_RunningPodCrashLoopBackOff(t *testing.T) {
	run := testRun()
	run.Status.Phase = "Running"
	run.Status.ReportID = "uid-1"
	now := metav1.Now()
	run.Status.StartedAt = &now

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run", Namespace: "default",
		},
		Status: batchv1.JobStatus{Active: 1},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run-xyz", Namespace: "default",
			Labels: map[string]string{"job-name": "agent-test-run"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "agent",
				State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{
						Reason:  "CrashLoopBackOff",
						Message: "back-off 5m0s restarting failed container",
					},
				},
			}},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run, job, pod).
		WithStatusSubresource(run).
		Build()

	ms := newMemStore()
	ms.runs["uid-1"] = &store.DiagnosticRun{ID: "uid-1", Status: store.PhaseRunning}

	r := &reconciler.DiagnosticRunReconciler{
		Client: fakeClient, Store: ms, Translator: testTranslator(),
	}

	result := reconcileOnce(t, r)
	assert.NotZero(t, result.RequeueAfter)

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "test-run", Namespace: "default"}, &updated))
	assert.Equal(t, "Running", updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "CrashLoopBackOff")
}

func TestRunReconciler_RunningTimeout(t *testing.T) {
	timeout := int32(60) // 60 seconds
	run := testRun()
	run.Spec.TimeoutSeconds = &timeout
	run.Status.Phase = "Running"
	run.Status.ReportID = "uid-1"
	// StartedAt was 2 minutes ago — past the 60s timeout
	past := metav1.NewTime(time.Now().Add(-2 * time.Minute))
	run.Status.StartedAt = &past

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run", Namespace: "default",
		},
		Status: batchv1.JobStatus{Active: 1},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run, job).
		WithStatusSubresource(run).
		Build()

	ms := newMemStore()
	ms.runs["uid-1"] = &store.DiagnosticRun{ID: "uid-1", Status: store.PhaseRunning}

	r := &reconciler.DiagnosticRunReconciler{
		Client: fakeClient, Store: ms, Translator: testTranslator(),
	}

	result := reconcileOnce(t, r)
	assert.Zero(t, result.RequeueAfter, "terminal — should not requeue")

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "test-run", Namespace: "default"}, &updated))
	assert.Equal(t, "Failed", updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "timed out")
}

func TestRunReconciler_RunningNoTimeoutWhenNil(t *testing.T) {
	run := testRun()
	// No TimeoutSeconds set — run.Spec.TimeoutSeconds is nil
	run.Status.Phase = "Running"
	run.Status.ReportID = "uid-1"
	past := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	run.Status.StartedAt = &past

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run", Namespace: "default",
		},
		Status: batchv1.JobStatus{Active: 1},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run, job).
		WithStatusSubresource(run).
		Build()

	ms := newMemStore()
	ms.runs["uid-1"] = &store.DiagnosticRun{ID: "uid-1", Status: store.PhaseRunning}

	r := &reconciler.DiagnosticRunReconciler{
		Client: fakeClient, Store: ms, Translator: testTranslator(),
	}

	result := reconcileOnce(t, r)
	assert.NotZero(t, result.RequeueAfter, "should keep polling — no timeout configured")

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "test-run", Namespace: "default"}, &updated))
	assert.Equal(t, "Running", updated.Status.Phase)
}

func TestRunReconciler_TerminalNoOp(t *testing.T) {
	run := testRun()
	run.Status.Phase = "Succeeded"

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run).
		WithStatusSubresource(run).
		Build()

	ms := newMemStore()
	r := &reconciler.DiagnosticRunReconciler{
		Client: fakeClient, Store: ms, Translator: testTranslator(),
	}

	result := reconcileOnce(t, r)
	assert.Zero(t, result.RequeueAfter)
}

// TestRunReconciler_RunningNoTimeoutWhenZero ensures that timeoutSeconds=0
// is treated as "no timeout" (the > 0 guard fix). A run that started 2h ago
// with timeout=0 should NOT be failed.
func TestRunReconciler_RunningNoTimeoutWhenZero(t *testing.T) {
	zero := int32(0)
	run := testRun()
	run.Spec.TimeoutSeconds = &zero
	run.Status.Phase = "Running"
	run.Status.ReportID = "uid-1"
	// StartedAt was 2 hours ago — would trigger timeout if 0 were respected
	past := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	run.Status.StartedAt = &past

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "agent-test-run", Namespace: "default",
		},
		Status: batchv1.JobStatus{Active: 1},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run, job).
		WithStatusSubresource(run).
		Build()

	ms := newMemStore()
	ms.runs["uid-1"] = &store.DiagnosticRun{ID: "uid-1", Status: store.PhaseRunning}

	r := &reconciler.DiagnosticRunReconciler{
		Client: fakeClient, Store: ms, Translator: testTranslator(),
	}

	result := reconcileOnce(t, r)
	assert.NotZero(t, result.RequeueAfter, "timeoutSeconds=0 should not trigger timeout")

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "test-run", Namespace: "default"}, &updated))
	assert.Equal(t, "Running", updated.Status.Phase, "should remain Running with timeout=0")
}
