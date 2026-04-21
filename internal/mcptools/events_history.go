package mcptools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

func NewEventsHistoryHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Store == nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     "event store not available",
			})
		}

		args, _ := req.Params.Arguments.(map[string]interface{})
		opts := store.ListEventsOpts{Limit: 100}
		if v, ok := args["namespace"].(string); ok {
			opts.Namespace = v
		}
		if v, ok := args["name"].(string); ok {
			opts.Name = v
		}
		if v, ok := args["since_minutes"].(float64); ok && int(v) > 0 {
			opts.SinceMinutes = int(v)
		}
		if v, ok := args["limit"].(float64); ok && int(v) > 0 {
			opts.Limit = int(v)
		}

		events, err := d.Store.ListEvents(ctx, opts)
		if err != nil {
			return jsonResult(map[string]interface{}{"error": err.Error()})
		}
		return jsonResult(map[string]interface{}{
			"count":  len(events),
			"events": events,
		})
	}
}
