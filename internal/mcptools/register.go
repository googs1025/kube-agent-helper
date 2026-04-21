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
	registerTool(s, d, mcp.NewTool("kubectl_get",
		mcp.WithDescription("Get or list Kubernetes resources. kind is required. Omit name to list all."),
		mcp.WithString("kind", mcp.Required(), mcp.Description("Resource kind, e.g. Pod, Deployment, Node")),
		mcp.WithString("apiVersion", mcp.Description("API version, e.g. apps/v1 (optional, auto-detected)")),
		mcp.WithString("namespace", mcp.Description("Namespace (required for namespaced kinds in get mode)")),
		mcp.WithString("name", mcp.Description("Resource name for single-resource get (omit to list)")),
		mcp.WithString("labelSelector", mcp.Description("Label selector, e.g. app=nginx")),
		mcp.WithString("fieldSelector", mcp.Description("Field selector, e.g. status.phase=Running")),
		mcp.WithNumber("limit", mcp.Description("Max results to return (default 100, max 500)")),
	), []string{"kind", "apiVersion", "namespace", "name", "labelSelector", "fieldSelector", "limit"}, NewKubectlGetHandler(d))

	registerTool(s, d, mcp.NewTool("kubectl_describe",
		mcp.WithDescription("Describe a single Kubernetes resource including events"),
		mcp.WithString("kind", mcp.Required(), mcp.Description("Resource kind, e.g. Pod")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Resource name")),
		mcp.WithString("namespace", mcp.Description("Namespace (required for namespaced kinds)")),
		mcp.WithString("apiVersion", mcp.Description("API version (optional, auto-detected)")),
	), []string{"kind", "apiVersion", "namespace", "name"}, NewKubectlDescribeHandler(d))

	registerTool(s, d, mcp.NewTool("kubectl_logs",
		mcp.WithDescription("Fetch container logs from a Pod"),
		mcp.WithString("namespace", mcp.Required(), mcp.Description("Namespace of the Pod")),
		mcp.WithString("pod", mcp.Required(), mcp.Description("Pod name")),
		mcp.WithString("container", mcp.Description("Container name (optional if Pod has one container)")),
		mcp.WithNumber("tailLines", mcp.Description("Number of lines from the end (default 100)")),
		mcp.WithBoolean("previous", mcp.Description("Return logs from the previous container instance")),
		mcp.WithNumber("sinceSeconds", mcp.Description("Return logs newer than this many seconds")),
	), []string{"namespace", "pod", "container", "tailLines", "previous", "sinceSeconds"}, NewKubectlLogsHandler(d))

	registerTool(s, d, mcp.NewTool("events_list",
		mcp.WithDescription("List Kubernetes events, optionally filtered by namespace or involved object"),
		mcp.WithString("namespace", mcp.Description("Namespace to filter events (omit for all namespaces)")),
		mcp.WithString("involvedKind", mcp.Description("Filter by involvedObject kind, e.g. Pod")),
		mcp.WithString("involvedName", mcp.Description("Filter by involvedObject name")),
		mcp.WithString("types", mcp.Description("Comma-separated event types to include, e.g. Warning,Normal")),
		mcp.WithNumber("limit", mcp.Description("Max events to return (default 100)")),
	), []string{"namespace", "involvedKind", "involvedName", "types", "limit"}, NewEventsListHandler(d))
}

// RegisterExtension adds the five extension tools (M6) to the server.
func RegisterExtension(s *server.MCPServer, d *Deps) {
	registerTool(s, d, mcp.NewTool("top_pods",
		mcp.WithDescription("Show CPU/memory usage for pods (requires metrics-server)"),
		mcp.WithString("namespace", mcp.Description("Namespace to filter (omit for all namespaces)")),
		mcp.WithString("labelSelector", mcp.Description("Label selector to filter pods")),
		mcp.WithString("sortBy", mcp.Description("Sort by: cpu or memory (default cpu)")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
	), []string{"namespace", "labelSelector", "sortBy", "limit"}, NewTopPodsHandler(d))

	registerTool(s, d, mcp.NewTool("top_nodes",
		mcp.WithDescription("Show CPU/memory usage for nodes (requires metrics-server)"),
		mcp.WithString("sortBy", mcp.Description("Sort by: cpu or memory (default cpu)")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
	), []string{"sortBy", "limit"}, NewTopNodesHandler(d))

	registerTool(s, d, mcp.NewTool("list_api_resources",
		mcp.WithDescription("List available Kubernetes API resource types"),
		mcp.WithString("verb", mcp.Description("Filter by supported verb, e.g. list, watch")),
		mcp.WithBoolean("namespaced", mcp.Description("Filter to only namespaced (true) or cluster-scoped (false) resources")),
	), []string{"verb", "namespaced"}, NewListAPIResourcesHandler(d))

	registerTool(s, d, mcp.NewTool("prometheus_query",
		mcp.WithDescription("Execute a PromQL query (requires --prometheus-url flag)"),
		mcp.WithString("query", mcp.Required(), mcp.Description("PromQL expression")),
		mcp.WithString("mode", mcp.Description("instant or range (default instant)")),
		mcp.WithString("time", mcp.Description("Evaluation time for instant queries (RFC3339)")),
		mcp.WithString("start", mcp.Description("Start time for range queries (RFC3339)")),
		mcp.WithString("end", mcp.Description("End time for range queries (RFC3339)")),
		mcp.WithString("step", mcp.Description("Step duration for range queries, e.g. 1m")),
	), []string{"query", "mode", "time", "start", "end", "step"}, NewPrometheusQueryHandler(d))

	registerTool(s, d, mcp.NewTool("kubectl_explain",
		mcp.WithDescription("Show OpenAPI schema for a Kubernetes resource kind or field path"),
		mcp.WithString("resource", mcp.Required(), mcp.Description("Kind or field path, e.g. Pod or Pod.spec.containers")),
	), []string{"resource"}, NewKubectlExplainHandler(d))

	registerTool(s, d, mcp.NewTool("node_status_summary",
		mcp.WithDescription("Show node conditions, capacity, allocated resources, and taints"),
		mcp.WithString("name", mcp.Description("Specific node name (omit for all nodes, max 20)")),
		mcp.WithString("labelSelector", mcp.Description("Label selector to filter nodes")),
	), []string{"name", "labelSelector"}, NewNodeStatusSummaryHandler(d))

	registerTool(s, d, mcp.NewTool("prometheus_alerts",
		mcp.WithDescription("List active Prometheus alerts, sorted by severity"),
		mcp.WithString("state", mcp.Description("Filter: firing, pending, or all (default firing)")),
		mcp.WithString("labelFilter", mcp.Description("Filter by labels, e.g. namespace=prod,severity=critical")),
	), []string{"state", "labelFilter"}, NewPrometheusAlertsHandler(d))

	registerTool(s, d, mcp.NewTool("kubectl_rollout_status",
		mcp.WithDescription("Show Deployment or StatefulSet rollout status with ReplicaSet history"),
		mcp.WithString("kind", mcp.Required(), mcp.Description("Deployment or StatefulSet")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Resource name")),
		mcp.WithString("namespace", mcp.Required(), mcp.Description("Namespace")),
	), []string{"kind", "name", "namespace"}, NewRolloutStatusHandler(d))

	registerTool(s, d, mcp.NewTool("pvc_status",
		mcp.WithDescription("List PersistentVolumeClaim status, capacity, and binding info"),
		mcp.WithString("namespace", mcp.Required(), mcp.Description("Namespace")),
		mcp.WithString("name", mcp.Description("Specific PVC name (omit to list all)")),
		mcp.WithString("labelSelector", mcp.Description("Label selector to filter PVCs")),
	), []string{"namespace", "name", "labelSelector"}, NewPVCStatusHandler(d))

	registerTool(s, d, mcp.NewTool("network_policy_check",
		mcp.WithDescription("Analyze NetworkPolicies affecting a specific Pod"),
		mcp.WithString("namespace", mcp.Required(), mcp.Description("Namespace of the target Pod")),
		mcp.WithString("podName", mcp.Required(), mcp.Description("Name of the Pod to analyze")),
	), []string{"namespace", "podName"}, NewNetworkPolicyCheckHandler(d))

	registerTool(s, d, mcp.NewTool("events_history",
		mcp.WithDescription("List recent Kubernetes Warning events from the local store"),
		mcp.WithString("namespace", mcp.Description("Namespace to filter (omit for all namespaces)")),
		mcp.WithString("name", mcp.Description("Involved object name to filter")),
		mcp.WithNumber("since_minutes", mcp.Description("Return events from the last N minutes (default all time)")),
		mcp.WithNumber("limit", mcp.Description("Max events to return (default 100)")),
	), []string{"namespace", "name", "since_minutes", "limit"}, NewEventsHistoryHandler(d))

	registerTool(s, d, mcp.NewTool("metric_history",
		mcp.WithDescription("Query stored Prometheus metric snapshots"),
		mcp.WithString("query", mcp.Required(), mcp.Description("PromQL metric name or expression used when scraping")),
		mcp.WithNumber("since_minutes", mcp.Description("Return snapshots from the last N minutes (default 60)")),
	), []string{"query", "since_minutes"}, NewMetricHistoryHandler(d))
}

// RegisterAll registers all core and extension tools.
func RegisterAll(s *server.MCPServer, d *Deps) {
	RegisterCore(s, d)
	RegisterExtension(s, d)
}

func registerTool(s *server.MCPServer, d *Deps, tool mcp.Tool, whitelist []string, handler audit.Handler) {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	wrapped := audit.Wrap(logger, audit.ToolSpec{
		Name:         tool.Name,
		ArgWhitelist: whitelist,
		Cluster:      d.Cluster,
	}, handler)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return wrapped(ctx, req)
	})
}
