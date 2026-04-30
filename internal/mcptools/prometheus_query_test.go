package mcptools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// fakePrometheus implements promv1.API for testing.
type fakePrometheus struct {
	promv1.API // embed to satisfy interface
	queryResult model.Value
}

func (f *fakePrometheus) Query(_ context.Context, _ string, _ time.Time, _ ...promv1.Option) (model.Value, promv1.Warnings, error) {
	return f.queryResult, nil, nil
}

func (f *fakePrometheus) QueryRange(_ context.Context, _ string, _ promv1.Range, _ ...promv1.Option) (model.Value, promv1.Warnings, error) {
	return f.queryResult, nil, nil
}

func TestPrometheusQuery_Unavailable(t *testing.T) {
	d := &Deps{Prometheus: nil}
	handler := NewPrometheusQueryHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"query": "up"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Contains(t, textOf(result), "prometheus not configured")
}

func TestPrometheusQuery_Instant(t *testing.T) {
	vec := model.Vector{
		&model.Sample{
			Metric:    model.Metric{"__name__": "up", "job": "k8s"},
			Value:     1,
			Timestamp: model.TimeFromUnix(1700000000),
		},
	}
	d := &Deps{Prometheus: &fakePrometheus{queryResult: vec}}
	handler := NewPrometheusQueryHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"query": "up", "mode": "instant"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, "instant", payload["mode"])
	data, ok := payload["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 1)
}

func TestPrometheusQuery_MissingQuery(t *testing.T) {
	d := &Deps{Prometheus: &fakePrometheus{}}
	handler := NewPrometheusQueryHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Contains(t, textOf(result), "query is required")
}

func TestPrometheusQuery_RangeDefaults(t *testing.T) {
	mat := model.Matrix{
		&model.SampleStream{
			Metric: model.Metric{"__name__": "up"},
			Values: []model.SamplePair{
				{Timestamp: model.TimeFromUnix(1700000000), Value: 1},
				{Timestamp: model.TimeFromUnix(1700000060), Value: 1},
			},
		},
	}
	d := &Deps{Prometheus: &fakePrometheus{queryResult: mat}}
	handler := NewPrometheusQueryHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"query": "up", "mode": "range"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, "range", payload["mode"])
	data, ok := payload["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 1)
}

func TestPrometheusQuery_RangeCustomStartEndStep(t *testing.T) {
	d := &Deps{Prometheus: &fakePrometheus{queryResult: model.Matrix{}}}
	handler := NewPrometheusQueryHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"query": "rate(http_requests_total[5m])",
		"mode":  "range",
		"start": "2026-04-30T10:00:00Z",
		"end":   "2026-04-30T11:00:00Z",
		"step":  "30s",
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, "range", payload["mode"])
}
