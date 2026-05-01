package reconciler_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/reconciler"
	"github.com/kube-agent-helper/kube-agent-helper/internal/notification"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

func fakeKubeClientset() kubernetes.Interface { return clientsetfake.NewSimpleClientset() }

// recordingNotifier records all events for assertion. Implements
// reconciler.NotifyDispatcher.
type recordingNotifier struct {
	mu     sync.Mutex
	calls  atomic.Int32
	events []notification.Event
}

func (n *recordingNotifier) Notify(_ context.Context, e notification.Event) error {
	n.calls.Add(1)
	n.mu.Lock()
	n.events = append(n.events, e)
	n.mu.Unlock()
	return nil
}

func (n *recordingNotifier) eventTypes() []notification.EventType {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]notification.EventType, len(n.events))
	for i, ev := range n.events {
		out[i] = ev.Type
	}
	return out
}

// ── completeRun via Reconcile: success notification + critical findings ──────

func TestRunReconciler_SuccessNotificationAndCriticalFindingsEmitted(t *testing.T) {
	run := testRun()
	run.Status.Phase = "Running"
	run.Status.ReportID = "uid-1"

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-test-run", Namespace: "default"},
		Status:     batchv1.JobStatus{Succeeded: 1, CompletionTime: &metav1.Time{}},
	}
	cli := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run, job).
		WithStatusSubresource(run).
		Build()

	ms := newMemStore()
	// Pre-seed the store with one critical and one info finding.
	require.NoError(t, ms.CreateFinding(context.Background(), &store.Finding{
		ID: "f1", RunID: "uid-1", Dimension: "health", Severity: "critical",
		Title: "Pod OOMKilled", Description: "container memory exceeded",
		ResourceKind: "Pod", ResourceNamespace: "default", ResourceName: "api",
	}))
	require.NoError(t, ms.CreateFinding(context.Background(), &store.Finding{
		ID: "f2", RunID: "uid-1", Dimension: "health", Severity: "info",
		Title: "everything fine",
	}))

	notifier := &recordingNotifier{}
	r := &reconciler.DiagnosticRunReconciler{
		Client: cli, Store: ms, Translator: testTranslator(), Notifier: notifier,
	}
	reconcileOnce(t, r)

	var got k8saiV1.DiagnosticRun
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "test-run", Namespace: "default"}, &got))
	assert.Equal(t, "Succeeded", got.Status.Phase)

	// Expect: one EventRunCompleted + one EventCriticalFinding
	types := notifier.eventTypes()
	require.GreaterOrEqual(t, len(types), 2, "expected at least RunCompleted + CriticalFinding events")
	assert.Contains(t, types, notification.EventRunCompleted)
	assert.Contains(t, types, notification.EventCriticalFinding)
}

// ── ModelConfigReconciler: empty APIKeyRef.Key + missing Secret ──────────────

func TestModelConfigReconciler_EmptyKeyAndMissingSecret_StillSucceeds(t *testing.T) {
	mc := &k8saiV1.ModelConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "mc-empty-key", Namespace: "default"},
		Spec: k8saiV1.ModelConfigSpec{
			Provider:  "anthropic",
			Model:     "claude-sonnet-4-6",
			APIKeyRef: k8saiV1.SecretKeyRef{Name: "no-such-secret"},
			// Key intentionally empty
		},
	}
	cli := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(mc).
		Build()
	r := &reconciler.ModelConfigReconciler{Client: cli}

	_, err := r.Reconcile(context.Background(), ctrlReq("mc-empty-key", "default"))
	require.NoError(t, err, "missing-secret + empty Key must not be a hard error")
}

// ctrlReq is a tiny helper that defers `ctrl.Request` import noise.
func ctrlReq(name, namespace string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}}
}

// ── collectPodLogs: clientset set, no pods → list path exercised ─────────────

func TestRunReconciler_CompleteWithClientset_NoPodsToCollect(t *testing.T) {
	run := testRun()
	run.Status.Phase = "Running"
	run.Status.ReportID = "uid-1"

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-test-run", Namespace: "default"},
		Status:     batchv1.JobStatus{Succeeded: 1, CompletionTime: &metav1.Time{}},
	}
	cli := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run, job).
		WithStatusSubresource(run).
		Build()

	r := &reconciler.DiagnosticRunReconciler{
		Client:     cli,
		Store:      newMemStore(),
		Translator: testTranslator(),
		Clientset:  fakeKubeClientset(),
	}
	reconcileOnce(t, r)

	var got k8saiV1.DiagnosticRun
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "test-run", Namespace: "default"}, &got))
	assert.Equal(t, "Succeeded", got.Status.Phase)
	// We don't assert on collectPodLogs side-effects: with a Clientset set
	// but zero pods labelled job-name=..., the function exits cleanly.
}

func TestRunReconciler_FailedRun_EmitsRunFailedNotification(t *testing.T) {
	run := testRun()
	run.Status.Phase = "Running"
	run.Status.ReportID = "uid-1"

	// Job in Failed condition.
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-test-run", Namespace: "default"},
		Status: batchv1.JobStatus{
			Failed: 1,
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobFailed, Status: "True", Reason: "BackoffLimitExceeded"},
			},
		},
	}
	cli := fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(run, job).
		WithStatusSubresource(run).
		Build()

	notifier := &recordingNotifier{}
	r := &reconciler.DiagnosticRunReconciler{
		Client: cli, Store: newMemStore(), Translator: testTranslator(), Notifier: notifier,
	}
	reconcileOnce(t, r)

	assert.Contains(t, notifier.eventTypes(), notification.EventRunFailed)
}
