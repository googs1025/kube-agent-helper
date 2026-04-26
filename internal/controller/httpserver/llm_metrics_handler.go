package httpserver

import (
	"encoding/json"
	"net/http"
)

// POST /internal/llm-metrics — called by agent pods to report LLM usage.
func (s *Server) handleLLMMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Model            string  `json:"model"`
		DurationMs       float64 `json:"duration_ms"`
		PromptTokens     int     `json:"prompt_tokens"`
		CompletionTokens int     `json:"completion_tokens"`
		Status           string  `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.Model == "" {
		req.Model = "unknown"
	}
	if req.Status == "" {
		req.Status = "ok"
	}

	if s.metrics != nil {
		s.metrics.LLMRequestsTotal.WithLabelValues(req.Model, req.Status).Inc()
		s.metrics.LLMRequestDuration.WithLabelValues(req.Model).Observe(req.DurationMs / 1000)
		s.metrics.LLMTokensTotal.WithLabelValues(req.Model, "prompt").Add(float64(req.PromptTokens))
		s.metrics.LLMTokensTotal.WithLabelValues(req.Model, "completion").Add(float64(req.CompletionTokens))
	}

	w.WriteHeader(http.StatusNoContent)
}
