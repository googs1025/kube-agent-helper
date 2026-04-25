package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_RegistersAllMetrics(t *testing.T) {
	m := New()
	require.NotNil(t, m)
	require.NotNil(t, m.Registry())

	// Populate all metrics so they appear in Gather output.
	m.RecordRunCompleted("default", "Succeeded", "local")
	m.ObserveRunDuration("default", "local", 5.0)
	m.RecordFinding("critical", "default", "local")
	m.RecordFixCompleted("Succeeded", "default", "local")
	m.IncActiveRuns()
	m.RecordEvent("OOMKilled", "local")
	m.LLMRequestsTotal.WithLabelValues("gpt-4", "ok").Inc()
	m.LLMRequestDuration.WithLabelValues("gpt-4").Observe(1.5)
	m.LLMTokensTotal.WithLabelValues("gpt-4", "prompt").Add(100)

	families, err := m.Registry().Gather()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, f := range families {
		names[f.GetName()] = true
	}

	expected := []string{
		"kah_diagnostic_runs_total",
		"kah_diagnostic_run_duration_seconds",
		"kah_findings_total",
		"kah_fixes_total",
		"kah_llm_requests_total",
		"kah_llm_request_duration_seconds",
		"kah_llm_tokens_total",
		"kah_event_collector_events_total",
		"kah_active_runs",
	}
	for _, name := range expected {
		assert.True(t, names[name], "missing metric: %s", name)
	}
}

func TestNew_NoConflicts(t *testing.T) {
	m1 := New()
	m2 := New()
	require.NotNil(t, m1)
	require.NotNil(t, m2)

	m1.RecordRunCompleted("ns1", "Succeeded", "local")
	m2.RecordRunCompleted("ns2", "Failed", "remote")

	f1, err := m1.Registry().Gather()
	require.NoError(t, err)
	f2, err := m2.Registry().Gather()
	require.NoError(t, err)
	assert.NotEmpty(t, f1)
	assert.NotEmpty(t, f2)
}

func TestCounterIncrements(t *testing.T) {
	m := New()

	m.RecordRunCompleted("default", "Succeeded", "local")
	m.RecordRunCompleted("default", "Succeeded", "local")
	m.RecordRunCompleted("default", "Failed", "local")

	families, err := m.Registry().Gather()
	require.NoError(t, err)

	for _, f := range families {
		if f.GetName() != "kah_diagnostic_runs_total" {
			continue
		}
		for _, metric := range f.GetMetric() {
			labels := map[string]string{}
			for _, lp := range metric.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			if labels["phase"] == "Succeeded" && labels["namespace"] == "default" && labels["cluster"] == "local" {
				assert.Equal(t, 2.0, metric.GetCounter().GetValue())
			}
			if labels["phase"] == "Failed" && labels["namespace"] == "default" && labels["cluster"] == "local" {
				assert.Equal(t, 1.0, metric.GetCounter().GetValue())
			}
		}
	}
}

func TestActiveRunsGauge(t *testing.T) {
	m := New()

	m.IncActiveRuns()
	m.IncActiveRuns()
	m.DecActiveRuns()

	families, err := m.Registry().Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() == "kah_active_runs" {
			require.Len(t, f.GetMetric(), 1)
			assert.Equal(t, 1.0, f.GetMetric()[0].GetGauge().GetValue())
			return
		}
	}
	t.Fatal("kah_active_runs metric not found")
}

func TestMetricsEndpoint(t *testing.T) {
	m := New()
	m.RecordRunCompleted("default", "Succeeded", "local")
	m.RecordFinding("warning", "default", "local")

	handler := promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	bodyStr := string(body)

	assert.True(t, strings.Contains(bodyStr, "kah_diagnostic_runs_total"), "should contain kah_diagnostic_runs_total")
	assert.True(t, strings.Contains(bodyStr, "kah_findings_total"), "should contain kah_findings_total")
}
