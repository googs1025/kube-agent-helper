package httpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// handleRunLogs serves GET /api/runs/{id}/logs[?follow=true].
//
// Behavior:
//   - If the run is in a terminal state (Succeeded/Failed) OR the agent pod
//     cannot be located, logs are read from the DB (these were persisted by
//     the reconciler at completion time).
//   - Otherwise, logs are streamed directly from the agent pod via the
//     Kubernetes API. With follow=true the pod log stream is followed live;
//     without follow=true the current pod log buffer is returned as JSON.
func (s *Server) handleRunLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		http.NotFound(w, r)
		return
	}
	runID := parts[2]
	if runID == "" {
		http.Error(w, "missing run ID", http.StatusBadRequest)
		return
	}

	follow := r.URL.Query().Get("follow") == "true"

	// Decide source: pod stream while running, DB after completion (or when
	// pod is unreachable).
	if s.shouldStreamFromPod(r.Context(), runID) {
		if s.streamLogsFromPod(w, r, runID, follow) {
			return
		}
		// streamLogsFromPod returned false → pod not found / not ready →
		// fall through to DB-backed path.
	}

	if !follow {
		s.writeStoredLogs(w, r, runID)
		return
	}
	s.streamStoredLogs(w, r, runID)
}

// shouldStreamFromPod returns true when the run is non-terminal AND a
// clientset is available — i.e. live pod streaming is the right source.
func (s *Server) shouldStreamFromPod(ctx context.Context, runID string) bool {
	if s.clientset == nil || s.k8sClient == nil {
		return false
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil || run == nil {
		// Unknown run in DB but might still exist as CR — try pod path.
		return true
	}
	return run.Status != store.PhaseSucceeded && run.Status != store.PhaseFailed
}

// writeStoredLogs returns all DB-stored logs as a JSON array.
func (s *Server) writeStoredLogs(w http.ResponseWriter, r *http.Request, runID string) {
	logs, err := s.store.ListRunLogs(r.Context(), runID, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if logs == nil {
		logs = []store.RunLog{}
	}
	writeJSON(w, logs)
}

// streamStoredLogs streams DB-stored logs via SSE, polling every 500ms for
// new entries until the run reaches a terminal state. Used as a fallback
// when the pod is unreachable.
func (s *Server) streamStoredLogs(w http.ResponseWriter, r *http.Request, runID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	setSSEHeaders(w)

	var lastID int64
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			logs, err := s.store.ListRunLogs(r.Context(), runID, lastID)
			if err != nil {
				return
			}
			for _, l := range logs {
				data, _ := json.Marshal(l)
				fmt.Fprintf(w, "data: %s\n\n", data)
				lastID = l.ID
			}
			flusher.Flush()

			if run, err := s.store.GetRun(r.Context(), runID); err == nil && run != nil {
				if (run.Status == store.PhaseSucceeded || run.Status == store.PhaseFailed) && len(logs) == 0 {
					fmt.Fprintf(w, "event: done\ndata: {}\n\n")
					flusher.Flush()
					return
				}
			}
		}
	}
}

// streamLogsFromPod streams logs directly from the agent pod's Kubernetes
// log endpoint. Returns true if the pod was located and the response was
// served (in any form), or false to signal the caller should fall back to
// DB-backed logs (e.g. pod not yet created or already deleted).
func (s *Server) streamLogsFromPod(w http.ResponseWriter, r *http.Request, runID string, follow bool) bool {
	pod, ok := s.findAgentPod(r.Context(), runID)
	if !ok {
		return false
	}

	logReq := s.clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Follow:    follow,
		Container: "agent",
	})
	stream, err := logReq.Stream(r.Context())
	if err != nil {
		return false
	}
	defer stream.Close()

	if !follow {
		writeJSON(w, collectPodLogLines(stream, runID))
		return true
	}

	flusher, fok := w.(http.Flusher)
	if !fok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return true
	}
	setSSEHeaders(w)

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var seq int64
	for scanner.Scan() {
		seq++
		entry := parsePodLogLine(scanner.Text(), runID, seq)
		data, _ := json.Marshal(entry)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
	fmt.Fprintf(w, "event: done\ndata: {}\n\n")
	flusher.Flush()
	return true
}

// findAgentPod resolves a DiagnosticRun UID to its agent pod, if any.
// Returns the most recently created pod when multiple exist (e.g. job retries).
func (s *Server) findAgentPod(ctx context.Context, runID string) (*corev1.Pod, bool) {
	var crList v1alpha1.DiagnosticRunList
	if err := s.k8sClient.List(ctx, &crList); err != nil {
		return nil, false
	}
	var cr *v1alpha1.DiagnosticRun
	for i := range crList.Items {
		if string(crList.Items[i].UID) == runID {
			cr = &crList.Items[i]
			break
		}
	}
	if cr == nil {
		return nil, false
	}

	jobName := fmt.Sprintf("agent-%s", cr.Name)
	var podList corev1.PodList
	if err := s.k8sClient.List(ctx, &podList,
		client.InNamespace(cr.Namespace),
		client.MatchingLabels{"job-name": jobName},
	); err != nil {
		return nil, false
	}
	if len(podList.Items) == 0 {
		return nil, false
	}

	latest := &podList.Items[0]
	for i := 1; i < len(podList.Items); i++ {
		if podList.Items[i].CreationTimestamp.After(latest.CreationTimestamp.Time) {
			latest = &podList.Items[i]
		}
	}
	return latest, true
}

// collectPodLogLines reads every line from r and returns a slice of RunLog
// entries. Used for the non-follow case so the response shape matches the
// DB-backed path.
func collectPodLogLines(r io.Reader, runID string) []store.RunLog {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	out := []store.RunLog{}
	var seq int64
	for scanner.Scan() {
		seq++
		out = append(out, parsePodLogLine(scanner.Text(), runID, seq))
	}
	return out
}

// parsePodLogLine matches the structured-JSON shape emitted by the agent
// runtime; lines that don't parse become "info" entries.
func parsePodLogLine(line, runID string, seq int64) store.RunLog {
	var raw struct {
		Timestamp string      `json:"timestamp"`
		RunID     string      `json:"run_id"`
		Type      string      `json:"type"`
		Message   string      `json:"message"`
		Data      interface{} `json:"data"`
	}
	entry := store.RunLog{ID: seq, RunID: runID}
	if err := json.Unmarshal([]byte(line), &raw); err == nil && raw.Message != "" {
		entry.Timestamp = raw.Timestamp
		entry.Type = raw.Type
		entry.Message = raw.Message
		if raw.Data != nil {
			b, _ := json.Marshal(raw.Data)
			entry.Data = string(b)
		}
	} else {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
		entry.Type = "info"
		entry.Message = line
	}
	if entry.Type == "" {
		entry.Type = "info"
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return entry
}

func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}
