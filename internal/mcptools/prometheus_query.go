package mcptools

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

func NewPrometheusQueryHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Prometheus == nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     "prometheus not configured (use --prometheus-url)",
			})
		}
		args, _ := req.Params.Arguments.(map[string]interface{})
		query, _ := args["query"].(string)
		if query == "" {
			return jsonResult(map[string]interface{}{"error": "query is required"})
		}

		mode, _ := args["mode"].(string)
		if mode == "range" {
			return handleRange(ctx, d.Prometheus, query, args)
		}
		return handleInstant(ctx, d.Prometheus, query, args)
	}
}

func handleInstant(ctx context.Context, api promv1.API, query string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	ts := time.Now()
	if v, ok := args["time"].(string); ok && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			ts = t
		}
	}
	val, warnings, err := api.Query(ctx, query, ts, promv1.WithTimeout(30*time.Second))
	if err != nil {
		return jsonResult(map[string]interface{}{"error": err.Error()})
	}
	return jsonResult(map[string]interface{}{
		"mode":     "instant",
		"warnings": warnings,
		"data":     marshalSamples(val),
	})
}

func handleRange(ctx context.Context, api promv1.API, query string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	end := time.Now()
	start := end.Add(-1 * time.Hour)
	step := 60 * time.Second

	if v, ok := args["start"].(string); ok && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			start = t
		}
	}
	if v, ok := args["end"].(string); ok && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			end = t
		}
	}
	if v, ok := args["step"].(string); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			step = d
		}
	}

	val, warnings, err := api.QueryRange(ctx, query, promv1.Range{Start: start, End: end, Step: step}, promv1.WithTimeout(30*time.Second))
	if err != nil {
		return jsonResult(map[string]interface{}{"error": err.Error()})
	}
	return jsonResult(map[string]interface{}{
		"mode":     "range",
		"warnings": warnings,
		"data":     marshalSamples(val),
	})
}

// marshalSamples converts prometheus model.Value to JSON-friendly structure.
func marshalSamples(v model.Value) interface{} {
	switch val := v.(type) {
	case model.Vector:
		rows := make([]map[string]interface{}, 0, len(val))
		for _, s := range val {
			labels := make(map[string]string, len(s.Metric))
			for k, v := range s.Metric {
				labels[string(k)] = string(v)
			}
			rows = append(rows, map[string]interface{}{
				"metric": labels,
				"value":  fmt.Sprintf("%g", float64(s.Value)),
				"ts":     s.Timestamp.Time().UTC().Format(time.RFC3339),
			})
		}
		return rows
	case model.Matrix:
		series := make([]map[string]interface{}, 0, len(val))
		for _, ss := range val {
			labels := make(map[string]string, len(ss.Metric))
			for k, v := range ss.Metric {
				labels[string(k)] = string(v)
			}
			points := make([]map[string]interface{}, 0, len(ss.Values))
			for _, sp := range ss.Values {
				points = append(points, map[string]interface{}{
					"ts":    sp.Timestamp.Time().UTC().Format(time.RFC3339),
					"value": fmt.Sprintf("%g", float64(sp.Value)),
				})
			}
			series = append(series, map[string]interface{}{
				"metric": labels,
				"values": points,
			})
		}
		return series
	default:
		return v.String()
	}
}
