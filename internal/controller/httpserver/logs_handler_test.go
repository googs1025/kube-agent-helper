package httpserver_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/httpserver"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

func TestGetRunLogs_NoFollow_ReturnsJSON(t *testing.T) {
	fs := &fakeStore{}
	fs.runLogs = []store.RunLog{
		{ID: 1, RunID: "run-1", Timestamp: "2026-01-01T00:00:00Z", Type: "step", Message: "starting"},
		{ID: 2, RunID: "run-1", Timestamp: "2026-01-01T00:00:01Z", Type: "finding", Message: "found issue"},
		{ID: 3, RunID: "run-2", Timestamp: "2026-01-01T00:00:02Z", Type: "info", Message: "other run"},
	}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/run-1/logs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var logs []store.RunLog
	require.NoError(t, json.NewDecoder(w.Body).Decode(&logs))
	assert.Len(t, logs, 2)
	assert.Equal(t, "starting", logs[0].Message)
	assert.Equal(t, "found issue", logs[1].Message)
}

func TestGetRunLogs_NoFollow_EmptyReturnsEmptyArray(t *testing.T) {
	fs := &fakeStore{}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/no-logs/logs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var logs []store.RunLog
	require.NoError(t, json.NewDecoder(w.Body).Decode(&logs))
	assert.Len(t, logs, 0)
}

func TestGetRunLogs_MethodNotAllowed(t *testing.T) {
	fs := &fakeStore{}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/runs/run-1/logs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestGetRunLogs_Follow_SSE(t *testing.T) {
	// completedFakeStore returns a completed run so the SSE loop terminates
	fs := &completedFakeStore{}
	fs.runs = []*store.DiagnosticRun{
		{ID: "run-sse", Status: store.PhaseSucceeded},
	}
	fs.runLogs = []store.RunLog{
		{ID: 1, RunID: "run-sse", Timestamp: "2026-01-01T00:00:00Z", Type: "step", Message: "hello"},
	}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	// Use httptest server to get proper streaming behavior
	ts := httptest.NewServer(srv)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/runs/run-sse/logs?follow=true", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	// Read SSE events
	scanner := bufio.NewScanner(resp.Body)
	var events []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			events = append(events, line[6:])
		}
		if strings.HasPrefix(line, "event: done") {
			break
		}
	}

	// Should have received at least the log entry
	require.NotEmpty(t, events)
	var firstLog store.RunLog
	require.NoError(t, json.Unmarshal([]byte(events[0]), &firstLog))
	assert.Equal(t, "hello", firstLog.Message)
	assert.Equal(t, "step", firstLog.Type)
}

func TestGetRunLogs_Persistence_RoundTrip(t *testing.T) {
	fs := &fakeStore{}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	// Simulate appending a log entry (as the reconciler would)
	_ = fs.AppendRunLog(context.Background(), store.RunLog{
		RunID:     "run-rt",
		Timestamp: "2026-01-01T00:00:00Z",
		Type:      "error",
		Message:   "something failed",
		Data:      `{"detail":"oom"}`,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/runs/run-rt/logs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var logs []store.RunLog
	require.NoError(t, json.NewDecoder(w.Body).Decode(&logs))
	require.Len(t, logs, 1)
	assert.Equal(t, "error", logs[0].Type)
	assert.Equal(t, "something failed", logs[0].Message)
}

// completedFakeStore is a fakeStore that has a static set of runs (with completed status)
// so the SSE loop can detect termination.
type completedFakeStore struct {
	fakeStore
}

func TestGetRunLogs_Running_StreamsFromPod(t *testing.T) {
	const runUID = "run-running-uid"

	// fakeStore reports the run as Running so the handler picks the pod path.
	fs := &fakeStore{}
	fs.runs = []*store.DiagnosticRun{
		{ID: runUID, Status: store.PhaseRunning},
	}

	// Controller-runtime fake client carrying the matching DiagnosticRun CR
	// and the agent pod (label job-name=agent-{name}).
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	cr := &v1alpha1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-run",
			Namespace: "default",
			UID:       types.UID(runUID),
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-my-run-xyz",
			Namespace: "default",
			Labels:    map[string]string{"job-name": "agent-my-run"},
		},
	}
	k8sClient := ctrlfake.NewClientBuilder().WithScheme(scheme).
		WithObjects(cr, pod).Build()

	// kubernetes/fake clientset with the same pod so GetLogs(...).Stream works.
	clientset := fake.NewSimpleClientset(pod)

	srv := httpserver.New(fs, k8sClient, nil, httpserver.WithClientset(clientset))

	req := httptest.NewRequest(http.MethodGet, "/api/runs/"+runUID+"/logs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var logs []store.RunLog
	require.NoError(t, json.NewDecoder(w.Body).Decode(&logs))
	// The fake clientset returns "fake logs" as the pod log body.
	require.NotEmpty(t, logs)
	assert.Equal(t, runUID, logs[0].RunID)
}

func TestGetRunLogs_Running_FallsBackToDBWhenPodMissing(t *testing.T) {
	const runUID = "run-missing-pod"

	fs := &fakeStore{}
	fs.runs = []*store.DiagnosticRun{
		{ID: runUID, Status: store.PhaseRunning},
	}
	fs.runLogs = []store.RunLog{
		{ID: 1, RunID: runUID, Timestamp: "2026-01-01T00:00:00Z", Type: "step", Message: "from-db"},
	}

	// k8s client with no CR/pod for this UID.
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	k8sClient := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	clientset := fake.NewSimpleClientset()

	srv := httpserver.New(fs, k8sClient, nil, httpserver.WithClientset(clientset))

	req := httptest.NewRequest(http.MethodGet, "/api/runs/"+runUID+"/logs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var logs []store.RunLog
	require.NoError(t, json.NewDecoder(w.Body).Decode(&logs))
	require.Len(t, logs, 1)
	assert.Equal(t, "from-db", logs[0].Message)
}
