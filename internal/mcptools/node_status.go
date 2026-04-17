package mcptools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewNodeStatusSummaryHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Typed == nil {
			return mcp.NewToolResultError("kubernetes typed client not available"), nil
		}
		args, _ := req.Params.Arguments.(map[string]interface{})
		name, _ := args["name"].(string)
		labelSelector, _ := args["labelSelector"].(string)

		var nodes []corev1.Node
		if name != "" {
			node, err := d.Typed.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get node: %v", err)), nil
			}
			nodes = []corev1.Node{*node}
		} else {
			list, err := d.Typed.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("list nodes: %v", err)), nil
			}
			nodes = list.Items
			if len(nodes) > 20 {
				nodes = nodes[:20]
			}
		}

		podList, err := d.Typed.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list pods: %v", err)), nil
		}
		podsByNode := make(map[string][]corev1.Pod)
		for _, p := range podList.Items {
			if p.Spec.NodeName != "" {
				podsByNode[p.Spec.NodeName] = append(podsByNode[p.Spec.NodeName], p)
			}
		}

		result := make([]map[string]interface{}, 0, len(nodes))
		for _, node := range nodes {
			nodePods := podsByNode[node.Name]

			var cpuReqMilli, memReqMi int64
			for _, p := range nodePods {
				for _, c := range p.Spec.Containers {
					if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
						cpuReqMilli += q.MilliValue()
					}
					if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
						memReqMi += q.Value() / (1024 * 1024)
					}
				}
			}

			cpuAlloc := node.Status.Allocatable[corev1.ResourceCPU]
			memAlloc := node.Status.Allocatable[corev1.ResourceMemory]
			podAlloc := node.Status.Allocatable[corev1.ResourcePods]

			cpuAllocMilli := cpuAlloc.MilliValue()
			memAllocMi := memAlloc.Value() / (1024 * 1024)

			var cpuPct, memPct float64
			if cpuAllocMilli > 0 {
				cpuPct = float64(cpuReqMilli) / float64(cpuAllocMilli) * 100
			}
			if memAllocMi > 0 {
				memPct = float64(memReqMi) / float64(memAllocMi) * 100
			}

			ready := false
			conditions := make([]map[string]interface{}, 0)
			for _, c := range node.Status.Conditions {
				conditions = append(conditions, map[string]interface{}{
					"type":    string(c.Type),
					"status":  string(c.Status),
					"reason":  c.Reason,
					"message": c.Message,
				})
				if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
					ready = true
				}
			}

			taints := make([]map[string]interface{}, 0, len(node.Spec.Taints))
			for _, t := range node.Spec.Taints {
				taints = append(taints, map[string]interface{}{
					"key": t.Key, "value": t.Value, "effect": string(t.Effect),
				})
			}

			result = append(result, map[string]interface{}{
				"name":          node.Name,
				"ready":         ready,
				"unschedulable": node.Spec.Unschedulable,
				"conditions":    conditions,
				"allocatable": map[string]interface{}{
					"cpuMilli": cpuAllocMilli,
					"memoryMi": memAllocMi,
					"pods":     podAlloc.Value(),
				},
				"allocated": map[string]interface{}{
					"cpuMilli": cpuReqMilli,
					"memoryMi": memReqMi,
				},
				"utilizationPct": map[string]interface{}{
					"cpu":    int(cpuPct),
					"memory": int(memPct),
				},
				"podCount": len(nodePods),
				"taints":   taints,
			})
		}

		return jsonResult(map[string]interface{}{"nodes": result})
	}
}