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
