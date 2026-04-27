package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/kube-agent-helper/kube-agent-helper/internal/sanitize"
)

const (
	defaultListLimit = 100
	maxListLimit     = 200
)

// NewKubectlGetHandler returns an mcp-go handler implementing kubectl_get.
func NewKubectlGetHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]interface{})

		kind, _ := args["kind"].(string)
		if kind == "" {
			return mcp.NewToolResultError("missing required argument: kind"), nil
		}
		apiVersion, _ := args["apiVersion"].(string)
		namespace, _ := args["namespace"].(string)
		name, _ := args["name"].(string)
		labelSelector, _ := args["labelSelector"].(string)
		fieldSelector, _ := args["fieldSelector"].(string)
		limit := defaultListLimit
		if v, ok := args["limit"].(float64); ok {
			limit = int(v)
		}
		if limit <= 0 || limit > maxListLimit {
			return mcp.NewToolResultError(fmt.Sprintf("limit must be between 1 and %d", maxListLimit)), nil
		}

		gvr, namespaced, err := d.Mapper.ResolveGVR(kind, apiVersion)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("unsupported kind, try list_api_resources: %v", err)), nil
		}

		ri := d.Dynamic.Resource(gvr)

		// --- get mode -------------------------------------------------------
		if name != "" {
			if namespaced && namespace == "" {
				return mcp.NewToolResultError("namespace is required for namespaced kinds in get mode"), nil
			}
			var obj *unstructured.Unstructured
			if namespaced {
				obj, err = ri.Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			} else {
				obj, err = ri.Get(ctx, name, metav1.GetOptions{})
			}
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			cleaned := sanitize.Clean(obj, d.SanitizeOpts)
			return jsonResult(cleaned.Object)
		}

		// --- list mode ------------------------------------------------------
		listOpts := metav1.ListOptions{
			LabelSelector: labelSelector,
			FieldSelector: fieldSelector,
			Limit:         int64(limit),
		}
		var list *unstructured.UnstructuredList
		if namespaced && namespace != "" {
			list, err = ri.Namespace(namespace).List(ctx, listOpts)
		} else {
			list, err = ri.List(ctx, listOpts)
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		items := make([]map[string]interface{}, 0, len(list.Items))
		for i := range list.Items {
			cleaned := sanitize.Clean(&list.Items[i], d.SanitizeOpts)
			items = append(items, d.Projectors.Project(cleaned))
		}

		truncated := int64(len(items)) >= int64(limit)
		total := int64(len(items))
		countAccurate := true
		if rem := list.GetRemainingItemCount(); rem != nil {
			total = int64(len(items)) + *rem
		} else {
			countAccurate = !truncated
		}

		payload := map[string]interface{}{
			"kind":          kind,
			"apiVersion":    apiVersion,
			"totalCount":    total,
			"returnedCount": len(items),
			"truncated":     truncated,
			"countAccurate": countAccurate,
			"items":         items,
		}
		return jsonResult(payload)
	}
}

// jsonResult marshals payload to JSON and wraps it in a tool result.
func jsonResult(payload interface{}) (*mcp.CallToolResult, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return mcp.NewToolResultError(errors.Join(errors.New("marshal result"), err).Error()), nil
	}
	return mcp.NewToolResultText(string(raw)), nil
}
