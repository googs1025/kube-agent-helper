package httpserver

import (
	"encoding/json"
	"net/http"
)

// llmEvent is one entry in the batch payload sent by agent-runtime
// reporter.flush_llm_metrics. Schema:
//
//	{"events": [
//	  {"type":"retry",     "labels":{"model":"sonnet","reason":"http_503"}},
//	  {"type":"fallback",  "labels":{"from_model":"sonnet","to_model":"haiku","reason":"retries_exhausted"}},
//	  {"type":"exhausted", "labels":{"endpoints":"3"}}
//	]}
type llmEvent struct {
	Type   string            `json:"type"`
	Labels map[string]string `json:"labels"`
}

// POST /internal/llm-metrics — accepts either:
//
//  1. New batch schema (ModelChain retry/fallback/exhausted events): {"events":[...]}
//  2. Legacy single-call schema (per-LLM-request usage): {"model":..., "duration_ms":..., ...}
//
// The two formats are disjoint at the top level, so we decode into a unified
// struct and dispatch on whichever fields populated.
func (s *Server) handleLLMMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Events []llmEvent `json:"events,omitempty"`

		// Legacy single-call fields
		Model            string  `json:"model,omitempty"`
		DurationMs       float64 `json:"duration_ms,omitempty"`
		PromptTokens     int     `json:"prompt_tokens,omitempty"`
		CompletionTokens int     `json:"completion_tokens,omitempty"`
		Status           string  `json:"status,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	if len(req.Events) > 0 {
		s.recordLLMEvents(req.Events)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Legacy path: single-call usage
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

func (s *Server) recordLLMEvents(events []llmEvent) {
	if s.metrics == nil {
		return
	}
	for _, e := range events {
		switch e.Type {
		case "retry":
			s.metrics.RecordLLMRetry(
				labelOr(e.Labels, "model", "unknown"),
				labelOr(e.Labels, "reason", "unknown"),
			)
		case "fallback":
			s.metrics.RecordLLMFallback(
				labelOr(e.Labels, "from_model", "unknown"),
				labelOr(e.Labels, "to_model", "unknown"),
				labelOr(e.Labels, "reason", "unknown"),
			)
		case "exhausted":
			s.metrics.RecordLLMChainExhausted(
				labelOr(e.Labels, "endpoints", "0"),
			)
		}
	}
}

func labelOr(labels map[string]string, key, fallback string) string {
	if v, ok := labels[key]; ok && v != "" {
		return v
	}
	return fallback
}
