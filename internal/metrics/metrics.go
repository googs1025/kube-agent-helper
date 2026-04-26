// Package metrics 定义并暴露控制器自身的 Prometheus 指标。
//
// 指标清单（命名前缀 kah_）：
//
//	┌────────────────────────────────────┬──────────┬─────────────────────────┐
//	│ 名称                                │ 类型      │ 含义                     │
//	├────────────────────────────────────┼──────────┼─────────────────────────┤
//	│ kah_diagnostic_runs_total          │ Counter  │ 诊断任务计数（按 phase）  │
//	│ kah_diagnostic_run_duration_seconds│ Histogram│ 诊断耗时                 │
//	│ kah_findings_total                 │ Counter  │ 发现条数（按 severity）   │
//	│ kah_fixes_total                    │ Counter  │ 修复任务（按 phase）      │
//	│ kah_llm_requests_total             │ Counter  │ LLM 调用数                │
//	│ kah_llm_request_duration_seconds   │ Histogram│ LLM 单次耗时              │
//	│ kah_llm_tokens_total               │ Counter  │ Token 用量                │
//	│ kah_event_collector_events_total   │ Counter  │ 采集到的 K8s 事件         │
//	│ kah_active_runs                    │ Gauge    │ 当前运行中的任务数         │
//	└────────────────────────────────────┴──────────┴─────────────────────────┘
//
// 暴露方式：HTTP server 在 /metrics 注册 promhttp.HandlerFor(m.Registry())。
// Helm chart 提供可选的 ServiceMonitor 让 Prometheus Operator 自动发现。
//
// 实例化注意：使用独立的 prometheus.Registry（非 default），避免和 controller-runtime
// 自带 metrics 混在一起，便于隔离测试。
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds all Prometheus metrics for the kube-agent-helper controller.
type Metrics struct {
	registry *prometheus.Registry

	DiagnosticRunsTotal       *prometheus.CounterVec
	DiagnosticRunDuration     *prometheus.HistogramVec
	FindingsTotal             *prometheus.CounterVec
	FixesTotal                *prometheus.CounterVec
	LLMRequestsTotal          *prometheus.CounterVec
	LLMRequestDuration        *prometheus.HistogramVec
	LLMTokensTotal            *prometheus.CounterVec
	EventCollectorEventsTotal *prometheus.CounterVec
	ActiveRuns                prometheus.Gauge
}

// New creates a new Metrics instance with a dedicated prometheus.Registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		registry: reg,
		DiagnosticRunsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "kah_diagnostic_runs_total", Help: "Total diagnostic runs"},
			[]string{"namespace", "phase", "cluster"},
		),
		DiagnosticRunDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{Name: "kah_diagnostic_run_duration_seconds", Help: "Run duration", Buckets: prometheus.DefBuckets},
			[]string{"namespace", "cluster"},
		),
		FindingsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "kah_findings_total", Help: "Total findings"},
			[]string{"severity", "namespace", "cluster"},
		),
		FixesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "kah_fixes_total", Help: "Total fixes"},
			[]string{"status", "namespace", "cluster"},
		),
		LLMRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "kah_llm_requests_total", Help: "Total LLM requests"},
			[]string{"model", "status"},
		),
		LLMRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{Name: "kah_llm_request_duration_seconds", Help: "LLM request duration"},
			[]string{"model"},
		),
		LLMTokensTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "kah_llm_tokens_total", Help: "Total LLM tokens"},
			[]string{"model", "direction"},
		),
		EventCollectorEventsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "kah_event_collector_events_total", Help: "Total collected events"},
			[]string{"reason", "cluster"},
		),
		ActiveRuns: prometheus.NewGauge(
			prometheus.GaugeOpts{Name: "kah_active_runs", Help: "Currently active runs"},
		),
	}

	reg.MustRegister(
		m.DiagnosticRunsTotal,
		m.DiagnosticRunDuration,
		m.FindingsTotal,
		m.FixesTotal,
		m.LLMRequestsTotal,
		m.LLMRequestDuration,
		m.LLMTokensTotal,
		m.EventCollectorEventsTotal,
		m.ActiveRuns,
	)

	return m
}

// Registry returns the dedicated prometheus.Registry used by this Metrics instance.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// RecordRunCompleted increments kah_diagnostic_runs_total for the given phase.
func (m *Metrics) RecordRunCompleted(namespace, phase, cluster string) {
	m.DiagnosticRunsTotal.WithLabelValues(namespace, phase, cluster).Inc()
}

// ObserveRunDuration records a diagnostic run duration in seconds.
func (m *Metrics) ObserveRunDuration(namespace, cluster string, durationSecs float64) {
	m.DiagnosticRunDuration.WithLabelValues(namespace, cluster).Observe(durationSecs)
}

// RecordFinding increments kah_findings_total.
func (m *Metrics) RecordFinding(severity, namespace, cluster string) {
	m.FindingsTotal.WithLabelValues(severity, namespace, cluster).Inc()
}

// RecordFixCompleted increments kah_fixes_total.
func (m *Metrics) RecordFixCompleted(status, namespace, cluster string) {
	m.FixesTotal.WithLabelValues(status, namespace, cluster).Inc()
}

// IncActiveRuns increments the active runs gauge.
func (m *Metrics) IncActiveRuns() {
	m.ActiveRuns.Inc()
}

// DecActiveRuns decrements the active runs gauge.
func (m *Metrics) DecActiveRuns() {
	m.ActiveRuns.Dec()
}

// RecordEvent increments kah_event_collector_events_total.
func (m *Metrics) RecordEvent(reason, cluster string) {
	m.EventCollectorEventsTotal.WithLabelValues(reason, cluster).Inc()
}
