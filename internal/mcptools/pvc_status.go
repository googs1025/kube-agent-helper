package mcptools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewPVCStatusHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Typed == nil {
			return mcp.NewToolResultError("kubernetes typed client not available"), nil
		}
		args, _ := req.Params.Arguments.(map[string]interface{})
		namespace, _ := args["namespace"].(string)
		if namespace == "" {
			return mcp.NewToolResultError("namespace is required"), nil
		}
		name, _ := args["name"].(string)
		labelSelector, _ := args["labelSelector"].(string)

		var pvcs []corev1.PersistentVolumeClaim
		if name != "" {
			pvc, err := d.Typed.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get pvc: %v", err)), nil
			}
			pvcs = []corev1.PersistentVolumeClaim{*pvc}
		} else {
			list, err := d.Typed.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("list pvcs: %v", err)), nil
			}
			pvcs = list.Items
		}

		items := make([]map[string]interface{}, 0, len(pvcs))
		for _, pvc := range pvcs {
			storageClass := ""
			if pvc.Spec.StorageClassName != nil {
				storageClass = *pvc.Spec.StorageClassName
			}
			requestedStorage := ""
			if q, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
				requestedStorage = q.String()
			}
			actualCapacity := ""
			if q, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
				actualCapacity = q.String()
			}
			accessModes := make([]string, len(pvc.Spec.AccessModes))
			for i, m := range pvc.Spec.AccessModes {
				accessModes[i] = string(m)
			}

			entry := map[string]interface{}{
				"name":             pvc.Name,
				"namespace":        pvc.Namespace,
				"phase":            string(pvc.Status.Phase),
				"storageClass":     storageClass,
				"requestedStorage": requestedStorage,
				"actualCapacity":   actualCapacity,
				"volumeName":       pvc.Spec.VolumeName,
				"accessModes":      accessModes,
			}

			if pvc.Status.Phase == corev1.ClaimPending {
				events, _ := d.Typed.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
					FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=PersistentVolumeClaim", pvc.Name),
				})
				if events != nil && len(events.Items) > 0 {
					evList := make([]map[string]interface{}, 0)
					for _, ev := range events.Items {
						evList = append(evList, map[string]interface{}{
							"type":    ev.Type,
							"reason":  ev.Reason,
							"message": ev.Message,
						})
					}
					entry["events"] = evList
				}
			}

			items = append(items, entry)
		}

		return jsonResult(map[string]interface{}{"items": items})
	}
}