//go:build e2e

package e2e_test

import (
	"context"
	"fmt"
	"path/filepath"
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
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/registry"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store/sqlite"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// newTestScheme builds the shared scheme used by all E2E tests in this package.
// It registers every API group used across api_lifecycle_test.go and reconciler
// lifecycle tests so either file can call newTestScheme() without collision.
func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = k8saiV1.AddToScheme(s)
	_ = batchv1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = rbacv1.AddToScheme(s)
	_ = networkingv1.AddToScheme(s)
	return s
}

// reconcilerTestScheme is an alias kept for internal clarity within reconciler tests.
func reconcilerTestScheme() *runtime.Scheme { return newTestScheme() }

// newSQLiteStore creates a real SQLiteStore backed by a temp file.
// This is the shared helper used by all E2E tests in this package.
func newSQLiteStore(t *testing.T) *sqlite.SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	dsn := filepath.Join(dir, "test.db")
	s, err := sqlite.New(dsn)
	require.NoError(t, err, "create sqlite store")
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// newTempSQLiteStore is an alias kept for internal readability within reconciler tests.
func newTempSQLiteStore(t *testing.T) *sqlite.SQLiteStore {
	return newSQLiteStore(t)
}

// seedPodHealthSkill inserts the pod-health-analyst skill into the store.
func seedPodHealthSkill(t *testing.T, ctx context.Context, s store.Store) {
	t.Helper()
	err := s.UpsertSkill(ctx, &store.Skill{
		Name:      "pod-health-analyst",
		Dimension: "health",
		Prompt:    "You are a Kubernetes health analyst.",
		ToolsJSON: "[]",
		Enabled:   true,
		Priority:  100,
	})
	require.NoError(t, err, "seed skill")
}

// newReconcilerTranslator creates a Translator backed by the given real store via registry.
func newReconcilerTranslator(s store.Store) *translator.Translator {
	reg := registry.New(s)
	return translator.New(translator.Config{
		AgentImage:    "agent:e2e-test",
		ControllerURL: "http://controller:8080",
	}, reg)
}

// makeDiagnosticRun is the canonical DiagnosticRun for reconciler tests.
func makeDiagnosticRun(name, uid string) *k8saiV1.DiagnosticRun {
	return &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       types.UID(uid),
		},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:         k8saiV1.TargetSpec{Scope: "namespace", Namespaces: []string{"default"}},
			Skills:         []string{"pod-health-analyst"},
			ModelConfigRef: "claude-default",
		},
	}
}

// reconcileRunOnce calls Reconcile once and requires no error.
func reconcileRunOnce(t *testing.T, r *reconciler.DiagnosticRunReconciler, name, namespace string) ctrl.Result {
	t.Helper()
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
	})
	require.NoError(t, err)
	return result
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestReconcilerLifecycle(t *testing.T) {
	t.Run("Pending_to_Running", func(t *testing.T) {
		ctx := context.Background()
		sqlStore := newTempSQLiteStore(t)
		seedPodHealthSkill(t, ctx, sqlStore)

		run := makeDiagnosticRun("pending-run", "uid-pending-1")
		scheme := reconcilerTestScheme()
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(run).
			WithStatusSubresource(run).
			Build()

		r := &reconciler.DiagnosticRunReconciler{
			Client:     fakeClient,
			Store:      sqlStore,
			Translator: newReconcilerTranslator(sqlStore),
		}

		result := reconcileRunOnce(t, r, run.Name, run.Namespace)
		assert.NotZero(t, result.RequeueAfter, "should requeue to poll Job status")

		// CR status should be Running.
		var updated k8saiV1.DiagnosticRun
		require.NoError(t, fakeClient.Get(ctx,
			types.NamespacedName{Name: run.Name, Namespace: run.Namespace}, &updated))
		assert.Equal(t, "Running", updated.Status.Phase)
		assert.NotNil(t, updated.Status.StartedAt)
		assert.Equal(t, string(run.UID), updated.Status.ReportID)

		// Job was created in the fake client.
		var job batchv1.Job
		require.NoError(t, fakeClient.Get(ctx,
			types.NamespacedName{Name: "agent-" + run.Name, Namespace: run.Namespace}, &job),
			"Job should be created")

		// ServiceAccount was created.
		var sa corev1.ServiceAccount
		require.NoError(t, fakeClient.Get(ctx,
			types.NamespacedName{Name: "run-" + run.Name, Namespace: run.Namespace}, &sa),
			"ServiceAccount should be created")

		// ConfigMap was created.
		var cm corev1.ConfigMap
		require.NoError(t, fakeClient.Get(ctx,
			types.NamespacedName{Name: "skill-bundle-" + run.Name, Namespace: run.Namespace}, &cm),
			"ConfigMap should be created")

		// ClusterRoleBinding was created. The reconciler calls SetNamespace on all
		// generated objects (including the cluster-scoped CRB), so the fake client
		// stores it under the run namespace. Use the same namespace in the lookup.
		var crb rbacv1.ClusterRoleBinding
		require.NoError(t, fakeClient.Get(ctx,
			types.NamespacedName{Name: "run-" + run.Name, Namespace: run.Namespace}, &crb),
			"ClusterRoleBinding should be created")

		// Run is persisted in SQLite and has Running status.
		storedRun, err := sqlStore.GetRun(ctx, string(run.UID))
		require.NoError(t, err, "run should be persisted in SQLite")
		assert.Equal(t, store.PhaseRunning, storedRun.Status)
	})

	t.Run("Running_JobSucceeded", func(t *testing.T) {
		ctx := context.Background()
		sqlStore := newTempSQLiteStore(t)
		seedPodHealthSkill(t, ctx, sqlStore)

		runUID := "uid-succeeded-1"
		run := makeDiagnosticRun("succeeded-run", runUID)
		now := metav1.Now()
		run.Status.Phase = "Running"
		run.Status.ReportID = runUID
		run.Status.StartedAt = &now

		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent-" + run.Name,
				Namespace: run.Namespace,
			},
			Status: batchv1.JobStatus{Succeeded: 1},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(reconcilerTestScheme()).
			WithObjects(run, job).
			WithStatusSubresource(run).
			Build()

		// Pre-populate store so completeRun can update the status.
		require.NoError(t, sqlStore.CreateRun(ctx, &store.DiagnosticRun{
			ID:     runUID,
			Status: store.PhaseRunning,
		}))

		r := &reconciler.DiagnosticRunReconciler{
			Client:     fakeClient,
			Store:      sqlStore,
			Translator: newReconcilerTranslator(sqlStore),
		}

		result := reconcileRunOnce(t, r, run.Name, run.Namespace)
		assert.Zero(t, result.RequeueAfter, "terminal state should not requeue")

		var updated k8saiV1.DiagnosticRun
		require.NoError(t, fakeClient.Get(ctx,
			types.NamespacedName{Name: run.Name, Namespace: run.Namespace}, &updated))

		assert.Equal(t, "Succeeded", updated.Status.Phase)
		assert.NotNil(t, updated.Status.CompletedAt)
		assert.Equal(t, "agent job completed successfully", updated.Status.Message)
	})

	t.Run("Running_JobFailed", func(t *testing.T) {
		ctx := context.Background()
		sqlStore := newTempSQLiteStore(t)
		seedPodHealthSkill(t, ctx, sqlStore)

		runUID := "uid-failed-1"
		run := makeDiagnosticRun("failed-run", runUID)
		now := metav1.Now()
		run.Status.Phase = "Running"
		run.Status.ReportID = runUID
		run.Status.StartedAt = &now

		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent-" + run.Name,
				Namespace: run.Namespace,
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
			WithScheme(reconcilerTestScheme()).
			WithObjects(run, job).
			WithStatusSubresource(run).
			Build()

		require.NoError(t, sqlStore.CreateRun(ctx, &store.DiagnosticRun{
			ID:     runUID,
			Status: store.PhaseRunning,
		}))

		r := &reconciler.DiagnosticRunReconciler{
			Client:     fakeClient,
			Store:      sqlStore,
			Translator: newReconcilerTranslator(sqlStore),
		}

		result := reconcileRunOnce(t, r, run.Name, run.Namespace)
		assert.Zero(t, result.RequeueAfter, "terminal state should not requeue")

		var updated k8saiV1.DiagnosticRun
		require.NoError(t, fakeClient.Get(ctx,
			types.NamespacedName{Name: run.Name, Namespace: run.Namespace}, &updated))

		assert.Equal(t, "Failed", updated.Status.Phase)
		assert.Contains(t, updated.Status.Message, "Back-off limit exceeded")
		assert.NotNil(t, updated.Status.CompletedAt)

		// Verify SQLite also reflects Failed.
		storedRun, err := sqlStore.GetRun(ctx, runUID)
		require.NoError(t, err)
		assert.Equal(t, store.PhaseFailed, storedRun.Status)
	})

	t.Run("Running_Timeout", func(t *testing.T) {
		ctx := context.Background()
		sqlStore := newTempSQLiteStore(t)
		seedPodHealthSkill(t, ctx, sqlStore)

		runUID := "uid-timeout-1"
		timeout := int32(1) // 1 second
		run := makeDiagnosticRun("timeout-run", runUID)
		run.Spec.TimeoutSeconds = &timeout
		run.Status.Phase = "Running"
		run.Status.ReportID = runUID
		// StartedAt was 2 seconds ago — past the 1s timeout.
		past := metav1.NewTime(time.Now().Add(-2 * time.Second))
		run.Status.StartedAt = &past

		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent-" + run.Name,
				Namespace: run.Namespace,
			},
			Status: batchv1.JobStatus{Active: 1},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(reconcilerTestScheme()).
			WithObjects(run, job).
			WithStatusSubresource(run).
			Build()

		require.NoError(t, sqlStore.CreateRun(ctx, &store.DiagnosticRun{
			ID:     runUID,
			Status: store.PhaseRunning,
		}))

		r := &reconciler.DiagnosticRunReconciler{
			Client:     fakeClient,
			Store:      sqlStore,
			Translator: newReconcilerTranslator(sqlStore),
		}

		result := reconcileRunOnce(t, r, run.Name, run.Namespace)
		assert.Zero(t, result.RequeueAfter, "terminal state should not requeue after timeout")

		var updated k8saiV1.DiagnosticRun
		require.NoError(t, fakeClient.Get(ctx,
			types.NamespacedName{Name: run.Name, Namespace: run.Namespace}, &updated))

		assert.Equal(t, "Failed", updated.Status.Phase)
		assert.Contains(t, updated.Status.Message, "timed out", "message should mention timeout")
	})

	t.Run("Findings_WrittenToStatus", func(t *testing.T) {
		ctx := context.Background()
		sqlStore := newTempSQLiteStore(t)
		seedPodHealthSkill(t, ctx, sqlStore)

		runUID := "uid-findings-1"
		run := makeDiagnosticRun("findings-run", runUID)
		now := metav1.Now()
		run.Status.Phase = "Running"
		run.Status.ReportID = runUID
		run.Status.StartedAt = &now

		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "agent-" + run.Name,
				Namespace: run.Namespace,
			},
			Status: batchv1.JobStatus{Succeeded: 1},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(reconcilerTestScheme()).
			WithObjects(run, job).
			WithStatusSubresource(run).
			Build()

		// Persist the run so UpdateRunStatus can find it.
		require.NoError(t, sqlStore.CreateRun(ctx, &store.DiagnosticRun{
			ID:     runUID,
			Status: store.PhaseRunning,
		}))

		// Write findings into SQLite before the completion reconcile.
		require.NoError(t, sqlStore.CreateFinding(ctx, &store.Finding{
			RunID:        runUID,
			Dimension:    "health",
			Severity:     "critical",
			Title:        "Pod CrashLooping",
			ResourceKind: "Pod",
			ResourceName: "nginx",
		}))
		require.NoError(t, sqlStore.CreateFinding(ctx, &store.Finding{
			RunID:        runUID,
			Dimension:    "health",
			Severity:     "medium",
			Title:        "High restart count",
			ResourceKind: "Pod",
			ResourceName: "nginx",
		}))
		require.NoError(t, sqlStore.CreateFinding(ctx, &store.Finding{
			RunID:        runUID,
			Dimension:    "security",
			Severity:     "critical",
			Title:        "Privileged container detected",
			ResourceKind: "Pod",
			ResourceName: "api-server",
		}))

		r := &reconciler.DiagnosticRunReconciler{
			Client:     fakeClient,
			Store:      sqlStore,
			Translator: newReconcilerTranslator(sqlStore),
		}

		result := reconcileRunOnce(t, r, run.Name, run.Namespace)
		assert.Zero(t, result.RequeueAfter, "completed run should not requeue")

		var updated k8saiV1.DiagnosticRun
		require.NoError(t, fakeClient.Get(ctx,
			types.NamespacedName{Name: run.Name, Namespace: run.Namespace}, &updated))

		assert.Equal(t, "Succeeded", updated.Status.Phase)

		// FindingCounts: 2 critical, 1 medium.
		assert.Equal(t, 2, updated.Status.FindingCounts["critical"], "two critical findings expected")
		assert.Equal(t, 1, updated.Status.FindingCounts["medium"], "one medium finding expected")

		// Findings slice should contain all 3.
		assert.Len(t, updated.Status.Findings, 3, "all 3 findings should appear in status")

		// Spot-check finding titles are all present.
		titles := make([]string, 0, len(updated.Status.Findings))
		for _, f := range updated.Status.Findings {
			titles = append(titles, f.Title)
		}
		assert.Contains(t, titles, "Pod CrashLooping")
		assert.Contains(t, titles, "Privileged container detected")
		assert.Contains(t, titles, "High restart count")
	})
}

// ── ScheduledRunReconciler lifecycle ─────────────────────────────────────────

func reconcileScheduledOnce(t *testing.T, r *reconciler.ScheduledRunReconciler, name, namespace string) ctrl.Result {
	t.Helper()
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
	})
	require.NoError(t, err)
	return result
}

func TestScheduledRunReconcilerLifecycle(t *testing.T) {
	t.Run("InitializesNextRunAt", func(t *testing.T) {
		// A fresh scheduled CR (no NextRunAt) should have NextRunAt set after the
		// first reconcile and requeue until that time.
		ctx := context.Background()
		parent := &k8saiV1.DiagnosticRun{
			ObjectMeta: metav1.ObjectMeta{Name: "sched-init", Namespace: "default", UID: "sched-uid-init"},
			Spec: k8saiV1.DiagnosticRunSpec{
				Schedule:       "0 * * * *", // hourly
				ModelConfigRef: "creds",
				Target:         k8saiV1.TargetSpec{Scope: "namespace"},
			},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(reconcilerTestScheme()).
			WithObjects(parent).
			WithStatusSubresource(parent).
			Build()
		r := &reconciler.ScheduledRunReconciler{Client: fakeClient}

		result := reconcileScheduledOnce(t, r, parent.Name, parent.Namespace)
		assert.NotZero(t, result.RequeueAfter, "should requeue until the next scheduled time")

		var updated k8saiV1.DiagnosticRun
		require.NoError(t, fakeClient.Get(ctx,
			types.NamespacedName{Name: parent.Name, Namespace: parent.Namespace}, &updated))
		require.NotNil(t, updated.Status.NextRunAt, "NextRunAt should be set after first reconcile")
		assert.True(t, updated.Status.NextRunAt.Time.After(time.Now()),
			"NextRunAt should be in the future")
	})

	t.Run("CreatesChildRunWhenDue", func(t *testing.T) {
		// When NextRunAt is in the past the reconciler must create a child run,
		// advance NextRunAt, set LastRunAt, and add the child to ActiveRuns.
		ctx := context.Background()
		past := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		parent := &k8saiV1.DiagnosticRun{
			ObjectMeta: metav1.ObjectMeta{Name: "sched-fire", Namespace: "default", UID: "sched-uid-fire"},
			Spec: k8saiV1.DiagnosticRunSpec{
				Schedule:       "* * * * *", // every minute
				ModelConfigRef: "creds",
				Target:         k8saiV1.TargetSpec{Scope: "namespace"},
			},
			Status: k8saiV1.DiagnosticRunStatus{NextRunAt: &past},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(reconcilerTestScheme()).
			WithObjects(parent).
			WithStatusSubresource(parent).
			Build()
		r := &reconciler.ScheduledRunReconciler{Client: fakeClient}

		result := reconcileScheduledOnce(t, r, parent.Name, parent.Namespace)
		assert.NotZero(t, result.RequeueAfter, "should requeue for the next scheduled slot")

		var updated k8saiV1.DiagnosticRun
		require.NoError(t, fakeClient.Get(ctx,
			types.NamespacedName{Name: parent.Name, Namespace: parent.Namespace}, &updated))
		require.Len(t, updated.Status.ActiveRuns, 1, "one child run should be registered")
		assert.NotNil(t, updated.Status.LastRunAt, "LastRunAt should be set")
		require.NotNil(t, updated.Status.NextRunAt)
		assert.True(t, updated.Status.NextRunAt.Time.After(past.Time),
			"NextRunAt should advance beyond the trigger time")

		// Verify the child run exists in K8s with the scheduler label
		var children k8saiV1.DiagnosticRunList
		require.NoError(t, fakeClient.List(ctx, &children))
		var childRuns []k8saiV1.DiagnosticRun
		for _, cr := range children.Items {
			if cr.Labels["kube-agent-helper.io/scheduled-by"] == parent.Name {
				childRuns = append(childRuns, cr)
			}
		}
		require.Len(t, childRuns, 1, "exactly one child run should exist in K8s")
		assert.Equal(t, "creds", childRuns[0].Spec.ModelConfigRef,
			"child run should inherit parent modelConfigRef")
	})

	t.Run("HistoryLimitEnforced", func(t *testing.T) {
		// When the number of child runs exceeds HistoryLimit the reconciler must
		// delete old children so that the count stays within the limit.
		// We prime the parent with 2 existing children and historyLimit=1,
		// then trigger a third reconcile cycle and verify eviction occurred.
		ctx := context.Background()
		historyLimit := int32(1)

		// Compute the first child's trigger time (1 minute in the past)
		trigger1 := time.Now().Add(-2 * time.Minute)
		child1Name := fmt.Sprintf("sched-hist2-%d", trigger1.Unix())
		trigger1Meta := metav1.NewTime(trigger1)

		child1 := &k8saiV1.DiagnosticRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      child1Name,
				Namespace: "default",
				Labels:    map[string]string{"kube-agent-helper.io/scheduled-by": "sched-hist2"},
				// Older creation timestamp so it sorts first (oldest)
				CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Minute)),
			},
		}

		// Second trigger: 1 minute ago → reconcile will create child2 and then
		// enforce limit=1, evicting child1 (the oldest).
		trigger2 := time.Now().Add(-1 * time.Minute)
		trigger2Meta := metav1.NewTime(trigger2)

		parent := &k8saiV1.DiagnosticRun{
			ObjectMeta: metav1.ObjectMeta{Name: "sched-hist2", Namespace: "default", UID: "sched-uid-hist2"},
			Spec: k8saiV1.DiagnosticRunSpec{
				Schedule:       "* * * * *",
				ModelConfigRef: "creds",
				Target:         k8saiV1.TargetSpec{Scope: "namespace"},
				HistoryLimit:   &historyLimit,
			},
			Status: k8saiV1.DiagnosticRunStatus{
				LastRunAt:  &trigger1Meta,
				NextRunAt:  &trigger2Meta,
				ActiveRuns: []string{child1Name},
			},
		}
		fakeClient := fake.NewClientBuilder().
			WithScheme(reconcilerTestScheme()).
			WithObjects(parent, child1).
			WithStatusSubresource(parent).
			Build()
		r := &reconciler.ScheduledRunReconciler{Client: fakeClient}

		reconcileScheduledOnce(t, r, parent.Name, parent.Namespace)

		// After eviction: total labeled children must be ≤ historyLimit
		var children k8saiV1.DiagnosticRunList
		require.NoError(t, fakeClient.List(ctx, &children))
		var childRuns []k8saiV1.DiagnosticRun
		for _, cr := range children.Items {
			if cr.Labels["kube-agent-helper.io/scheduled-by"] == parent.Name {
				childRuns = append(childRuns, cr)
			}
		}
		assert.LessOrEqual(t, len(childRuns), int(historyLimit),
			"eviction should keep children count within historyLimit")

		// Parent status must show the cycle advanced
		var updated k8saiV1.DiagnosticRun
		require.NoError(t, fakeClient.Get(ctx,
			types.NamespacedName{Name: parent.Name, Namespace: parent.Namespace}, &updated))
		assert.NotNil(t, updated.Status.LastRunAt)
		require.NotNil(t, updated.Status.NextRunAt)
		assert.True(t, updated.Status.NextRunAt.Time.After(trigger2),
			"NextRunAt should advance beyond the second trigger time")
	})
}

// ── DiagnosticRun reconciler — timeout=0 regression ──────────────────────────

func TestRunReconcilerLifecycle_TimeoutZero(t *testing.T) {
	// A run with timeoutSeconds=0 must NOT be timed out even if StartedAt is
	// far in the past. The reconciler must treat 0 as "no timeout".
	ctx := context.Background()
	sqlStore := newTempSQLiteStore(t)
	seedPodHealthSkill(t, ctx, sqlStore)

	runUID := "uid-timeout-zero-e2e"
	zero := int32(0)
	run := makeDiagnosticRun("timeout-zero-run", runUID)
	run.Spec.TimeoutSeconds = &zero
	run.Status.Phase = "Running"
	run.Status.ReportID = runUID
	// 2 hours ago — would immediately fail if zero were treated as 0-second timeout
	past := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	run.Status.StartedAt = &past

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-timeout-zero-run", Namespace: "default"},
		Status:     batchv1.JobStatus{Active: 1},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(reconcilerTestScheme()).
		WithObjects(run, job).
		WithStatusSubresource(run).
		Build()

	require.NoError(t, sqlStore.CreateRun(ctx, &store.DiagnosticRun{
		ID: runUID, Status: store.PhaseRunning,
	}))

	r := &reconciler.DiagnosticRunReconciler{
		Client:     fakeClient,
		Store:      sqlStore,
		Translator: newReconcilerTranslator(sqlStore),
	}

	result := reconcileRunOnce(t, r, run.Name, run.Namespace)
	assert.NotZero(t, result.RequeueAfter, "timeoutSeconds=0 should not trigger timeout")

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(ctx,
		types.NamespacedName{Name: run.Name, Namespace: run.Namespace}, &updated))
	assert.Equal(t, "Running", updated.Status.Phase,
		"run should remain Running when timeoutSeconds=0")
}

// ── DiagnosticFix reconciler lifecycle ───────────────────────────────────────

func reconcileFixOnce(t *testing.T, r *reconciler.DiagnosticFixReconciler, name, namespace string) ctrl.Result {
	t.Helper()
	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
	})
	require.NoError(t, err)
	return result
}

func TestFixReconcilerLifecycle(t *testing.T) {
	t.Run("ApproveAndApply_Create", func(t *testing.T) {
		// Starting from phase=Approved with strategy=create the reconciler must
		// create the target resource in K8s, update the CR to Succeeded, and
		// reflect the new phase in SQLite.
		ctx := context.Background()
		sqlStore := newTempSQLiteStore(t)

		fixUID := "fix-e2e-uid-apply-1"
		npJSON := `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy",` +
			`"metadata":{"name":"deny-all","namespace":"default"},` +
			`"spec":{"podSelector":{}}}`

		fix := &k8saiV1.DiagnosticFix{
			ObjectMeta: metav1.ObjectMeta{Name: "fix-e2e-1", Namespace: "default", UID: types.UID(fixUID)},
			Spec: k8saiV1.DiagnosticFixSpec{
				DiagnosticRunRef: "run-1",
				FindingTitle:     "Missing NetworkPolicy",
				Target:           k8saiV1.FixTarget{Kind: "NetworkPolicy", Namespace: "default", Name: "deny-all"},
				Strategy:         "create",
				ApprovalRequired: true,
				Patch:            k8saiV1.FixPatch{Type: "strategic-merge", Content: npJSON},
			},
		}
		fix.Status.Phase = "Approved"

		require.NoError(t, sqlStore.CreateFix(ctx, &store.Fix{
			ID:           fixUID,
			RunID:        "run-1",
			FindingID:    "f-1",
			FindingTitle: "Missing NetworkPolicy",
			Phase:        store.FixPhaseApproved,
		}))

		fakeClient := fake.NewClientBuilder().
			WithScheme(reconcilerTestScheme()).
			WithObjects(fix).
			WithStatusSubresource(fix).
			Build()

		r := &reconciler.DiagnosticFixReconciler{Client: fakeClient, Store: sqlStore}

		result := reconcileFixOnce(t, r, fix.Name, fix.Namespace)
		assert.Zero(t, result.RequeueAfter, "completed fix should not requeue")

		// CR should reflect Succeeded
		var updated k8saiV1.DiagnosticFix
		require.NoError(t, fakeClient.Get(ctx,
			types.NamespacedName{Name: fix.Name, Namespace: fix.Namespace}, &updated))
		assert.Equal(t, "Succeeded", updated.Status.Phase)
		assert.Contains(t, updated.Status.Message, "created")
		assert.NotNil(t, updated.Status.CompletedAt)
		assert.NotNil(t, updated.Status.AppliedAt)

		// The NetworkPolicy must exist in K8s
		var np networkingv1.NetworkPolicy
		require.NoError(t, fakeClient.Get(ctx,
			types.NamespacedName{Name: "deny-all", Namespace: "default"}, &np),
			"NetworkPolicy should have been created by the fix reconciler")

		// SQLite must reflect Succeeded
		storedFix, err := sqlStore.GetFix(ctx, fixUID)
		require.NoError(t, err)
		assert.Equal(t, store.FixPhaseSucceeded, storedFix.Phase)
	})

	t.Run("DryRun_GoesToDryRunComplete", func(t *testing.T) {
		// A fix with strategy=dry-run must transition to DryRunComplete on the
		// first reconcile without applying any patch.
		ctx := context.Background()
		sqlStore := newTempSQLiteStore(t)

		fixUID := "fix-e2e-uid-dryrun-1"
		fix := &k8saiV1.DiagnosticFix{
			ObjectMeta: metav1.ObjectMeta{Name: "fix-e2e-dry", Namespace: "default", UID: types.UID(fixUID)},
			Spec: k8saiV1.DiagnosticFixSpec{
				DiagnosticRunRef: "run-1",
				FindingTitle:     "Replica mismatch",
				Target:           k8saiV1.FixTarget{Kind: "Deployment", Namespace: "default", Name: "web"},
				Strategy:         "dry-run",
				ApprovalRequired: true,
				Patch:            k8saiV1.FixPatch{Type: "strategic-merge", Content: `{"spec":{"replicas":3}}`},
			},
		}

		require.NoError(t, sqlStore.CreateFix(ctx, &store.Fix{
			ID:    fixUID,
			RunID: "run-1",
			Phase: store.FixPhasePendingApproval,
		}))

		fakeClient := fake.NewClientBuilder().
			WithScheme(reconcilerTestScheme()).
			WithObjects(fix).
			WithStatusSubresource(fix).
			Build()

		r := &reconciler.DiagnosticFixReconciler{Client: fakeClient, Store: sqlStore}

		// First reconcile: "" → DryRunComplete (requeued so reconciler can process next state)
		reconcileFixOnce(t, r, fix.Name, fix.Namespace)

		var updated k8saiV1.DiagnosticFix
		require.NoError(t, fakeClient.Get(ctx,
			types.NamespacedName{Name: fix.Name, Namespace: fix.Namespace}, &updated))
		assert.Equal(t, "DryRunComplete", updated.Status.Phase)
		assert.Contains(t, updated.Status.Message, "Dry-run")

		// Second reconcile on DryRunComplete must be a no-op (terminal state)
		result := reconcileFixOnce(t, r, fix.Name, fix.Namespace)
		assert.Zero(t, result.RequeueAfter, "DryRunComplete is terminal — should not requeue")
	})
}
