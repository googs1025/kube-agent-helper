package mcptools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAlertsAPI struct {
	promv1.API
	alerts []promv1.Alert
}

func (f *fakeAlertsAPI) Alerts(ctx context.Context) (promv1.AlertsResult, error) {
	return promv1.AlertsResult{Alerts: f.alerts}, nil
}

func TestPrometheusAlerts_Unavailable(t *testing.T) {
	d := &Deps{Prometheus: nil}
	handler := NewPrometheusAlertsHandler(d)
	result, err := handler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, false, payload["available"])
}

func TestPrometheusAlerts_FilterFiring(t *testing.T) {
	api := &fakeAlertsAPI{
		alerts: []promv1.Alert{
			{
				Labels:   model.LabelSet{"alertname": "HighCPU", "severity": "critical", "namespace": "prod"},
				State:    promv1.AlertStateFiring,
				ActiveAt: mustParseTime("2026-04-17T10:00:00Z"),
			},
			{
				Labels: model.LabelSet{"alertname": "DiskSlow", "severity": "warning"},
				State:  promv1.AlertStatePending,
			},
		},
	}
	d := &Deps{Prometheus: api}
	handler := NewPrometheusAlertsHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"state": "firing"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		Available bool                     `json:"available"`
		Alerts    []map[string]interface{} `json:"alerts"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.True(t, payload.Available)
	require.Len(t, payload.Alerts, 1)
	assert.Equal(t, "HighCPU", payload.Alerts[0]["alertname"])
}

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}