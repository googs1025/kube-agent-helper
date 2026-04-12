package mcptools

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/kube-agent-helper/kube-agent-helper/internal/audit"
)

// RegisterCore adds the four core diagnostic tools (M5) to the server.
func RegisterCore(s *server.MCPServer, d *Deps) {
	register(s, d, "kubectl_get",
		"Get a Kubernetes resource (list mode if no name, get mode if name is provided)",
		[]string{"kind", "apiVersion", "namespace", "name", "labelSelector", "fieldSelector", "limit"},
		NewKubectlGetHandler(d))

	register(s, d, "kubectl_describe",
		"Describe a single resource with related events",
		[]string{"kind", "apiVersion", "namespace", "name"},
		NewKubectlDescribeHandler(d))

	register(s, d, "kubectl_logs",
		"Fetch container logs (supports tailLines, previous, sinceSeconds)",
		[]string{"namespace", "pod", "container", "tailLines", "previous", "sinceSeconds"},
		NewKubectlLogsHandler(d))

	register(s, d, "events_list",
		"List events, optionally filtered by type or involvedObject",
		[]string{"namespace", "involvedKind", "involvedName", "types", "limit"},
		NewEventsListHandler(d))
}

// RegisterExtension adds the five extension tools (M6) to the server.
func RegisterExtension(s *server.MCPServer, d *Deps) {
	register(s, d, "top_pods",
		"Show CPU/memory usage for pods, sorted by cpu or memory (requires metrics-server)",
		[]string{"namespace", "labelSelector", "sortBy", "limit"},
		NewTopPodsHandler(d))

	register(s, d, "top_nodes",
		"Show CPU/memory usage for nodes, sorted by cpu or memory (requires metrics-server)",
		[]string{"sortBy", "limit"},
		NewTopNodesHandler(d))

	register(s, d, "list_api_resources",
		"List available API resource types, optionally filtered by verb or namespaced",
		[]string{"verb", "namespaced"},
		NewListAPIResourcesHandler(d))

	register(s, d, "prometheus_query",
		"Execute a PromQL instant or range query (requires --prometheus-url)",
		[]string{"query", "mode", "time", "start", "end", "step"},
		NewPrometheusQueryHandler(d))

	register(s, d, "kubectl_explain",
		"Show OpenAPI schema for a Kubernetes resource kind or field path (e.g. Pod.spec.containers)",
		[]string{"resource"},
		NewKubectlExplainHandler(d))
}

// RegisterAll registers all core and extension tools.
func RegisterAll(s *server.MCPServer, d *Deps) {
	RegisterCore(s, d)
	RegisterExtension(s, d)
}

func register(s *server.MCPServer, d *Deps, name, desc string, whitelist []string, handler audit.Handler) {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	tool := mcp.NewTool(name, mcp.WithDescription(desc))
	wrapped := audit.Wrap(logger, audit.ToolSpec{
		Name:         name,
		ArgWhitelist: whitelist,
		Cluster:      d.Cluster,
	}, handler)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return wrapped(ctx, req)
	})
}
