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
