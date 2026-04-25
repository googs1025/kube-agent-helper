package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// handleRunLogs serves GET /api/runs/{id}/logs[?follow=true].
//
// Without follow: returns all stored logs as a JSON array.
// With follow=true: streams logs via Server-Sent Events (SSE),
// polling the database every 500ms for new entries.
func (s *Server) handleRunLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract run ID from path: /api/runs/{id}/logs
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

	if !follow {
		logs, err := s.store.ListRunLogs(r.Context(), runID, 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if logs == nil {
			logs = []store.RunLog{}
		}
		writeJSON(w, logs)
		return
	}

	// SSE streaming mode
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

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

			// Check if run is in terminal state and no more logs to send
			run, err := s.store.GetRun(r.Context(), runID)
			if err == nil && run != nil {
				if (run.Status == store.PhaseSucceeded || run.Status == store.PhaseFailed) && len(logs) == 0 {
					// Send a final "done" event so the client knows to disconnect
					fmt.Fprintf(w, "event: done\ndata: {}\n\n")
					flusher.Flush()
					return
				}
			}
		}
	}
}
