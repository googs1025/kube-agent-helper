package mcptools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

func NewMetricHistoryHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Store == nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     "metric store not available",
			})
		}

		args, _ := req.Params.Arguments.(map[string]interface{})
		query, _ := args["query"].(string)
		if query == "" {
			return jsonResult(map[string]interface{}{"error": "query is required"})
		}
		sinceMinutes := 60
		if v, ok := args["since_minutes"].(float64); ok && int(v) > 0 {
			sinceMinutes = int(v)
		}

		snaps, err := d.Store.QueryMetricHistory(ctx, query, sinceMinutes)
		if err != nil {
			return jsonResult(map[string]interface{}{"error": err.Error()})
		}
		return jsonResult(map[string]interface{}{
			"count":   len(snaps),
			"metrics": snaps,
		})
	}
}