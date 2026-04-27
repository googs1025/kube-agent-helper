package mcptools

import (
	"context"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewEventsListHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]interface{})

		namespace, _ := args["namespace"].(string)
		involvedKind, _ := args["involvedKind"].(string)
		involvedName, _ := args["involvedName"].(string)
		limit := 50
		if v, ok := args["limit"].(float64); ok {
			limit = int(v)
		}
		if limit <= 0 || limit > 50 {
			return mcp.NewToolResultError("limit must be between 1 and 50"), nil
		}

		typeFilter := map[string]bool{}
		if rawTypes, ok := args["types"].([]interface{}); ok {
			for _, t := range rawTypes {
				if s, ok := t.(string); ok {
					typeFilter[s] = true
				}
			}
		}

		list, err := d.Typed.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		filtered := make([]corev1.Event, 0, len(list.Items))
		for _, ev := range list.Items {
			if len(typeFilter) > 0 && !typeFilter[ev.Type] {
				continue
			}
			if involvedKind != "" && ev.InvolvedObject.Kind != involvedKind {
				continue
			}
			if involvedName != "" && ev.InvolvedObject.Name != involvedName {
				continue
			}
			filtered = append(filtered, ev)
		}
		sort.SliceStable(filtered, func(i, j int) bool {
			return filtered[i].LastTimestamp.After(filtered[j].LastTimestamp.Time)
		})

		total := len(filtered)
		truncated := total > limit
		if truncated {
			filtered = filtered[:limit]
		}

		events := make([]map[string]interface{}, 0, len(filtered))
		for _, ev := range filtered {
			events = append(events, map[string]interface{}{
				"namespace":      ev.Namespace,
				"type":           ev.Type,
				"reason":         ev.Reason,
				"message":        ev.Message,
				"firstTimestamp": ev.FirstTimestamp.Format("2006-01-02T15:04:05Z"),
				"lastTimestamp":  ev.LastTimestamp.Format("2006-01-02T15:04:05Z"),
				"count":          ev.Count,
				"involvedObject": map[string]interface{}{
					"kind":      ev.InvolvedObject.Kind,
					"name":      ev.InvolvedObject.Name,
					"namespace": ev.InvolvedObject.Namespace,
				},
			})
		}

		return jsonResult(map[string]interface{}{
			"totalCount":    total,
			"returnedCount": len(events),
			"truncated":     truncated,
			"events":        events,
		})
	}
}
