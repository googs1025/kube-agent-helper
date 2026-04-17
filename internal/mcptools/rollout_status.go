package mcptools

import (
	"context"
	"fmt"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewRolloutStatusHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]interface{})
		kind, _ := args["kind"].(string)
		name, _ := args["name"].(string)
		namespace, _ := args["namespace"].(string)
		if kind == "" || name == "" || namespace == "" {
			return mcp.NewToolResultError("kind, name, and namespace are required"), nil
		}

		switch kind {
		case "Deployment":
			return rolloutDeployment(ctx, d, namespace, name)
		case "StatefulSet":
			return rolloutStatefulSet(ctx, d, namespace, name)
		default:
			return mcp.NewToolResultError("kind must be Deployment or StatefulSet"), nil
		}
	}
}

func rolloutDeployment(ctx context.Context, d *Deps, ns, name string) (*mcp.CallToolResult, error) {
	deploy, err := d.Typed.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get deployment: %v", err)), nil
	}

	rsList, err := d.Typed.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list replicasets: %v", err)), nil
	}

	var owned []map[string]interface{}
	for _, rs := range rsList.Items {
		for _, ref := range rs.OwnerReferences {
			if ref.Controller != nil && *ref.Controller && ref.UID == deploy.UID {
				var image string
				if len(rs.Spec.Template.Spec.Containers) > 0 {
					image = rs.Spec.Template.Spec.Containers[0].Image
				}
				owned = append(owned, map[string]interface{}{
					"name":          rs.Name,
					"replicas":      rs.Status.Replicas,
					"readyReplicas": rs.Status.ReadyReplicas,
					"image":         image,
					"createdAt":     rs.CreationTimestamp.Format("2006-01-02T15:04:05Z"),
				})
			}
		}
	}
	sort.SliceStable(owned, func(i, j int) bool {
		return owned[i]["createdAt"].(string) > owned[j]["createdAt"].(string)
	})

	status := "complete"
	if deploy.Status.UpdatedReplicas < *deploy.Spec.Replicas {
		status = "progressing"
	} else if deploy.Status.AvailableReplicas < *deploy.Spec.Replicas {
		status = "progressing"
	}
	for _, c := range deploy.Status.Conditions {
		if c.Type == "Progressing" && c.Reason == "ProgressDeadlineExceeded" {
			status = "degraded"
		}
	}

	conditions := make([]map[string]interface{}, 0, len(deploy.Status.Conditions))
	for _, c := range deploy.Status.Conditions {
		conditions = append(conditions, map[string]interface{}{
			"type":    string(c.Type),
			"status":  string(c.Status),
			"reason":  c.Reason,
			"message": c.Message,
		})
	}

	return jsonResult(map[string]interface{}{
		"name":              deploy.Name,
		"namespace":         deploy.Namespace,
		"status":            status,
		"desiredReplicas":   *deploy.Spec.Replicas,
		"updatedReplicas":   deploy.Status.UpdatedReplicas,
		"readyReplicas":     deploy.Status.ReadyReplicas,
		"availableReplicas": deploy.Status.AvailableReplicas,
		"conditions":        conditions,
		"replicaSets":       owned,
	})
}

func rolloutStatefulSet(ctx context.Context, d *Deps, ns, name string) (*mcp.CallToolResult, error) {
	sts, err := d.Typed.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get statefulset: %v", err)), nil
	}

	status := "complete"
	if sts.Status.UpdatedReplicas < *sts.Spec.Replicas {
		status = "progressing"
	} else if sts.Status.ReadyReplicas < *sts.Spec.Replicas {
		status = "progressing"
	}

	return jsonResult(map[string]interface{}{
		"name":            sts.Name,
		"namespace":       sts.Namespace,
		"status":          status,
		"desiredReplicas": *sts.Spec.Replicas,
		"updatedReplicas": sts.Status.UpdatedReplicas,
		"readyReplicas":   sts.Status.ReadyReplicas,
		"currentRevision": sts.Status.CurrentRevision,
		"updateRevision":  sts.Status.UpdateRevision,
	})
}
