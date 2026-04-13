package mcptools

import (
	"context"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewTopPodsHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Metrics == nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     "metrics-server not installed",
			})
		}
		args, _ := req.Params.Arguments.(map[string]interface{})
		namespace, _ := args["namespace"].(string)
		labelSelector, _ := args["labelSelector"].(string)
		sortBy, _ := args["sortBy"].(string)
		if sortBy == "" {
			sortBy = "cpu"
		}
		limit := 50
		if v, ok := args["limit"].(float64); ok {
			limit = int(v)
		}

		list, err := d.Metrics.MetricsV1beta1().PodMetricses(namespace).
			List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     err.Error(),
			})
		}

		type row struct {
			Name      string
			Namespace string
			CPUMilli  int64
			MemoryMi  int64
		}
		rows := make([]row, 0, len(list.Items))
		for _, pm := range list.Items {
			var cpu, mem int64
			for _, c := range pm.Containers {
				if q, ok := c.Usage["cpu"]; ok {
					cpu += q.MilliValue()
				}
				if q, ok := c.Usage["memory"]; ok {
					mem += q.Value() / (1024 * 1024)
				}
			}
			rows = append(rows, row{
				Name: pm.Name, Namespace: pm.Namespace,
				CPUMilli: cpu, MemoryMi: mem,
			})
		}
		sort.SliceStable(rows, func(i, j int) bool {
			if sortBy == "memory" {
				return rows[i].MemoryMi > rows[j].MemoryMi
			}
			return rows[i].CPUMilli > rows[j].CPUMilli
		})
		if len(rows) > limit {
			rows = rows[:limit]
		}

		items := make([]map[string]interface{}, 0, len(rows))
		for _, r := range rows {
			items = append(items, map[string]interface{}{
				"name":      r.Name,
				"namespace": r.Namespace,
				"cpuMilli":  r.CPUMilli,
				"memoryMi":  r.MemoryMi,
			})
		}
		return jsonResult(map[string]interface{}{
			"available": true,
			"items":     items,
		})
	}
}

func NewTopNodesHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Metrics == nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     "metrics-server not installed",
			})
		}
		args, _ := req.Params.Arguments.(map[string]interface{})
		sortBy, _ := args["sortBy"].(string)
		if sortBy == "" {
			sortBy = "cpu"
		}
		limit := 50
		if v, ok := args["limit"].(float64); ok {
			limit = int(v)
		}

		list, err := d.Metrics.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
		if err != nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     err.Error(),
			})
		}

		type row struct {
			Name     string
			CPUMilli int64
			MemoryMi int64
		}
		rows := make([]row, 0, len(list.Items))
		for _, nm := range list.Items {
			cpu := nm.Usage["cpu"]
			mem := nm.Usage["memory"]
			rows = append(rows, row{
				Name:     nm.Name,
				CPUMilli: cpu.MilliValue(),
				MemoryMi: mem.Value() / (1024 * 1024),
			})
		}
		sort.SliceStable(rows, func(i, j int) bool {
			if sortBy == "memory" {
				return rows[i].MemoryMi > rows[j].MemoryMi
			}
			return rows[i].CPUMilli > rows[j].CPUMilli
		})
		if len(rows) > limit {
			rows = rows[:limit]
		}

		items := make([]map[string]interface{}, 0, len(rows))
		for _, r := range rows {
			items = append(items, map[string]interface{}{
				"name":     r.Name,
				"cpuMilli": r.CPUMilli,
				"memoryMi": r.MemoryMi,
			})
		}
		return jsonResult(map[string]interface{}{
			"available": true,
			"items":     items,
		})
	}
}
