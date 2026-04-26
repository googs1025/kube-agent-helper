# Prometheus Metrics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task.

**Issue:** #29 - Expose Prometheus metrics from controller

## Goal

Expose Prometheus-compatible metrics from the kube-agent-helper controller, covering diagnostic runs, findings, fixes, LLM usage, and event collection. Provide a `/metrics` endpoint, Helm ServiceMonitor, and instrumentation across all major code paths.

## Architecture

The metrics package registers a custom `prometheus.Registry` with 9 metrics. Each reconciler and subsystem calls metric helpers. The HTTP server exposes `/metrics` via `promhttp.HandlerFor()`. A Helm ServiceMonitor enables Prometheus Operator scraping.

## Tech Stack

- `github.com/prometheus/client_golang/prometheus`
- `github.com/prometheus/client_golang/prometheus/promhttp`
- Helm ServiceMonitor CRD

## File Map

| File | Status |
|------|--------|
| `internal/metrics/metrics.go` | New |
| `internal/controller/httpserver/server.go` | Modified |
| `internal/controller/reconciler/diagnosticrun_reconciler.go` | Modified |
| `internal/controller/reconciler/diagnosticfix_reconciler.go` | Modified |
| `internal/controller/eventcollector/collector.go` | Modified |
| `internal/controller/httpserver/findings_handler.go` | Modified |
| `cmd/controller/main.go` | Modified |
| `deploy/helm/templates/service.yaml` | Modified |
| `deploy/helm/templates/servicemonitor.yaml` | New |
| `deploy/helm/values.yaml` | Modified |
| `internal/controller/httpserver/llm_metrics_handler.go` | New |
| `internal/metrics/metrics_test.go` | New |

## Tasks

### Task 1: Create metrics package

- [ ] Create `internal/metrics/metrics.go`

**Files:** `internal/metrics/metrics.go`

**Steps:**

```go
package metrics

import "github.com/prometheus/client_golang/prometheus"

var Registry = prometheus.NewRegistry()

var (
    DiagnosticRunsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "kah_diagnostic_runs_total", Help: "Total diagnostic runs"},
        []string{"namespace", "phase", "cluster"},
    )
    DiagnosticRunDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{Name: "kah_diagnostic_run_duration_seconds", Help: "Run duration", Buckets: prometheus.DefBuckets},
        []string{"namespace", "cluster"},
    )
    FindingsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "kah_findings_total", Help: "Total findings"},
        []string{"severity", "namespace", "cluster"},
    )
    FixesTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "kah_fixes_total", Help: "Total fixes"},
        []string{"status", "namespace", "cluster"},
    )
    LLMRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "kah_llm_requests_total", Help: "Total LLM requests"},
        []string{"model", "status"},
    )
    LLMRequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{Name: "kah_llm_request_duration_seconds", Help: "LLM request duration"},
        []string{"model"},
    )
    LLMTokensTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "kah_llm_tokens_total", Help: "Total LLM tokens"},
        []string{"model", "direction"},
    )
    EventCollectorEventsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "kah_event_collector_events_total", Help: "Total collected events"},
        []string{"reason", "cluster"},
    )
    ActiveRuns = prometheus.NewGauge(
        prometheus.GaugeOpts{Name: "kah_active_runs", Help: "Currently active runs"},
    )
)

func init() {
    Registry.MustRegister(DiagnosticRunsTotal, DiagnosticRunDuration, FindingsTotal,
        FixesTotal, LLMRequestsTotal, LLMRequestDuration, LLMTokensTotal,
        EventCollectorEventsTotal, ActiveRuns)
}
```

**Test:** `go build ./internal/metrics/...`

**Commit:** `feat(metrics): add prometheus metrics registry with 9 metrics`

### Task 2: Add /metrics endpoint to HTTP server

- [ ] Register `/metrics` route using `promhttp.HandlerFor(metrics.Registry, ...)`
- [ ] Ensure endpoint does not require auth

**Files:** `internal/controller/httpserver/server.go`

**Steps:**

- Import `internal/metrics` and `promhttp`
- Add `mux.Handle("/metrics", promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}))` before auth middleware

**Test:** `curl localhost:8080/metrics | grep kah_`

**Commit:** `feat(server): expose /metrics endpoint for prometheus scraping`

### Task 3: Instrument DiagnosticRun reconciler

- [ ] Increment `kah_diagnostic_runs_total` on phase transitions
- [ ] Observe `kah_diagnostic_run_duration_seconds` on completion
- [ ] Inc/Dec `kah_active_runs` gauge on Running/Completed transitions

**Files:** `internal/controller/reconciler/diagnosticrun_reconciler.go`

**Steps:**

- On phase change: `metrics.DiagnosticRunsTotal.WithLabelValues(ns, phase, cluster).Inc()`
- On completion: compute duration from `.Status.StartTime`, observe histogram
- On Running: `metrics.ActiveRuns.Inc()`; on terminal: `metrics.ActiveRuns.Dec()`

**Test:** `go test ./internal/controller/reconciler/ -run TestDiagnosticRun`

**Commit:** `feat(metrics): instrument diagnosticrun reconciler`

### Task 4: Instrument DiagnosticFix reconciler

- [ ] Increment `kah_fixes_total` on status changes (Applied, Failed, Rejected)

**Files:** `internal/controller/reconciler/diagnosticfix_reconciler.go`

**Steps:**

- On fix status transition: `metrics.FixesTotal.WithLabelValues(status, ns, cluster).Inc()`

**Test:** `go test ./internal/controller/reconciler/ -run TestDiagnosticFix`

**Commit:** `feat(metrics): instrument diagnosticfix reconciler`

### Task 5: Instrument event collector

- [ ] Increment `kah_event_collector_events_total` per collected event

**Files:** `internal/controller/eventcollector/collector.go`

**Steps:**

- After event is stored: `metrics.EventCollectorEventsTotal.WithLabelValues(event.Reason, cluster).Inc()`

**Test:** `go test ./internal/controller/eventcollector/...`

**Commit:** `feat(metrics): instrument event collector`

### Task 6: Instrument findings endpoint

- [ ] Increment `kah_findings_total` when findings are created/stored

**Files:** `internal/controller/httpserver/findings_handler.go`

**Steps:**

- In the handler that persists findings: `metrics.FindingsTotal.WithLabelValues(severity, ns, cluster).Inc()`

**Test:** `go test ./internal/controller/httpserver/ -run TestFindings`

**Commit:** `feat(metrics): instrument findings creation`

### Task 7: Wire metrics into main.go

- [ ] Import metrics package to trigger `init()` registration
- [ ] Pass registry to HTTP server if needed

**Files:** `cmd/controller/main.go`

**Steps:**

- Add `_ "github.com/.../internal/metrics"` import
- Ensure HTTP server setup references metrics registry

**Test:** `go build ./cmd/controller/`

**Commit:** `feat(main): wire prometheus metrics registry`

### Task 8: Helm chart Service + ServiceMonitor

- [ ] Add `metrics` port (9090 or reuse 8080) to Service
- [ ] Create ServiceMonitor template gated by `metrics.serviceMonitor.enabled`
- [ ] Add values: `metrics.enabled`, `metrics.serviceMonitor.enabled`, `metrics.serviceMonitor.interval`

**Files:** `deploy/helm/templates/service.yaml`, `deploy/helm/templates/servicemonitor.yaml`, `deploy/helm/values.yaml`

**Steps:**

```yaml
# servicemonitor.yaml
{{- if .Values.metrics.serviceMonitor.enabled }}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: {{ include "kube-agent-helper.fullname" . }}
spec:
  selector:
    matchLabels: {{ include "kube-agent-helper.selectorLabels" . | nindent 6 }}
  endpoints:
    - port: http
      path: /metrics
      interval: {{ .Values.metrics.serviceMonitor.interval | default "30s" }}
{{- end }}
```

**Test:** `helm template ./deploy/helm --set metrics.serviceMonitor.enabled=true | grep ServiceMonitor`

**Commit:** `feat(helm): add ServiceMonitor for prometheus metrics`

### Task 9: LLM metrics callback endpoint

- [ ] Create `POST /internal/llm-metrics` handler for agent pods to report LLM usage
- [ ] Accept JSON `{model, duration_ms, prompt_tokens, completion_tokens, status}`
- [ ] Update LLM counters and histogram

**Files:** `internal/controller/httpserver/llm_metrics_handler.go`

**Steps:**

```go
func (s *Server) handleLLMMetrics(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Model            string  `json:"model"`
        DurationMs       float64 `json:"duration_ms"`
        PromptTokens     int     `json:"prompt_tokens"`
        CompletionTokens int     `json:"completion_tokens"`
        Status           string  `json:"status"`
    }
    json.NewDecoder(r.Body).Decode(&req)
    metrics.LLMRequestsTotal.WithLabelValues(req.Model, req.Status).Inc()
    metrics.LLMRequestDuration.WithLabelValues(req.Model).Observe(req.DurationMs / 1000)
    metrics.LLMTokensTotal.WithLabelValues(req.Model, "prompt").Add(float64(req.PromptTokens))
    metrics.LLMTokensTotal.WithLabelValues(req.Model, "completion").Add(float64(req.CompletionTokens))
}
```

**Test:** `curl -X POST localhost:8080/internal/llm-metrics -d '{"model":"gpt-4","duration_ms":1200,"prompt_tokens":500,"completion_tokens":100,"status":"ok"}'`

**Commit:** `feat(server): add LLM metrics callback endpoint`

### Task 10: Integration test

- [ ] Create `internal/metrics/metrics_test.go`
- [ ] Test that all metrics register without conflict
- [ ] Test counter increments and histogram observations
- [ ] Test `/metrics` endpoint returns expected metric names

**Files:** `internal/metrics/metrics_test.go`

**Steps:**

- Use `prometheus.NewRegistry()` in tests to avoid global state conflicts
- HTTP test: start server, GET `/metrics`, assert contains `kah_diagnostic_runs_total`

**Test:** `go test ./internal/metrics/ -v`

**Commit:** `test(metrics): add integration tests for prometheus metrics`
