package mcptools

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// AlertsAPI is the subset of promv1.API we need for fetching alerts.
type AlertsAPI interface {
	Alerts(ctx context.Context) (promv1.AlertsResult, error)
}

func NewPrometheusAlertsHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Prometheus == nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     "prometheus not configured (use --prometheus-url)",
			})
		}

		args, _ := req.Params.Arguments.(map[string]interface{})
		state, _ := args["state"].(string)
		if state == "" {
			state = "firing"
		}
		labelFilter, _ := args["labelFilter"].(string)

		filterMap := make(map[string]string)
		if labelFilter != "" {
			for _, pair := range strings.Split(labelFilter, ",") {
				kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
				if len(kv) == 2 {
					filterMap[kv[0]] = kv[1]
				}
			}
		}

		alerter, ok := d.Prometheus.(AlertsAPI)
		if !ok {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     "prometheus client does not support alerts API",
			})
		}

		result, err := alerter.Alerts(ctx)
		if err != nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     err.Error(),
			})
		}

		sevOrder := map[string]int{"critical": 0, "warning": 1, "info": 2}

		var filtered []map[string]interface{}
		for _, a := range result.Alerts {
			if state != "all" {
				if state == "firing" && a.State != promv1.AlertStateFiring {
					continue
				}
				if state == "pending" && a.State != promv1.AlertStatePending {
					continue
				}
			}
			match := true
			for k, v := range filterMap {
				if string(a.Labels[model.LabelName(k)]) != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}

			labels := make(map[string]string, len(a.Labels))
			for k, v := range a.Labels {
				labels[string(k)] = string(v)
			}
			annotations := make(map[string]string, len(a.Annotations))
			for k, v := range a.Annotations {
				annotations[string(k)] = string(v)
			}

			entry := map[string]interface{}{
				"alertname":   string(a.Labels["alertname"]),
				"state":       string(a.State),
				"severity":    string(a.Labels["severity"]),
				"labels":      labels,
				"annotations": annotations,
			}
			if !a.ActiveAt.IsZero() {
				entry["activeAt"] = a.ActiveAt.UTC().Format(time.RFC3339)
			}
			if a.Value != "" {
				entry["value"] = string(a.Value)
			}

			filtered = append(filtered, entry)
		}

		sort.SliceStable(filtered, func(i, j int) bool {
			si := sevOrder[filtered[i]["severity"].(string)]
			sj := sevOrder[filtered[j]["severity"].(string)]
			if si != sj {
				return si < sj
			}
			ai, _ := filtered[i]["activeAt"].(string)
			aj, _ := filtered[j]["activeAt"].(string)
			return ai > aj
		})

		return jsonResult(map[string]interface{}{
			"available": true,
			"count":     len(filtered),
			"alerts":    filtered,
		})
	}
}
