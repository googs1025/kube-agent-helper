package mcptools

import (
	"context"
	"fmt"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kube-agent-helper/kube-agent-helper/internal/sanitize"
)

const maxRelatedEvents = 20

func NewKubectlDescribeHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]interface{})

		kind, _ := args["kind"].(string)
		apiVersion, _ := args["apiVersion"].(string)
		namespace, _ := args["namespace"].(string)
		name, _ := args["name"].(string)
		if kind == "" || name == "" {
			return mcp.NewToolResultError("kind and name are required"), nil
		}

		gvr, namespaced, err := d.Mapper.ResolveGVR(kind, apiVersion)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("unsupported kind: %v", err)), nil
		}
		if namespaced && namespace == "" {
			return mcp.NewToolResultError("namespace is required for namespaced kinds"), nil
		}

		ri := d.Dynamic.Resource(gvr)
		var got map[string]interface{}
		if namespaced {
			obj, err := ri.Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			got = sanitize.Clean(obj, d.SanitizeOpts).Object
		} else {
			obj, err := ri.Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			got = sanitize.Clean(obj, d.SanitizeOpts).Object
		}

		uid, _ := extractUID(got)
		events, err := listRelatedEvents(ctx, d, namespace, uid)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(map[string]interface{}{
			"object":        got,
			"relatedEvents": events,
		})
	}
}

func extractUID(obj map[string]interface{}) (string, bool) {
	meta, ok := obj["metadata"].(map[string]interface{})
	if !ok {
		return "", false
	}
	uid, ok := meta["uid"].(string)
	return uid, ok
}

func listRelatedEvents(ctx context.Context, d *Deps, namespace, uid string) ([]map[string]interface{}, error) {
	if d.Typed == nil {
		return nil, nil
	}
	evList, err := d.Typed.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	filtered := make([]corev1.Event, 0, len(evList.Items))
	for _, ev := range evList.Items {
		if uid != "" && string(ev.InvolvedObject.UID) != uid {
			continue
		}
		filtered = append(filtered, ev)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].LastTimestamp.After(filtered[j].LastTimestamp.Time)
	})
	if len(filtered) > maxRelatedEvents {
		filtered = filtered[:maxRelatedEvents]
	}

	out := make([]map[string]interface{}, 0, len(filtered))
	for _, ev := range filtered {
		out = append(out, map[string]interface{}{
			"type":           ev.Type,
			"reason":         ev.Reason,
			"message":        ev.Message,
			"firstTimestamp": ev.FirstTimestamp.Format("2006-01-02T15:04:05Z"),
			"lastTimestamp":  ev.LastTimestamp.Format("2006-01-02T15:04:05Z"),
			"count":          ev.Count,
		})
	}
	return out, nil
}
