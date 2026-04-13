package mcptools

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/kube-agent-helper/kube-agent-helper/internal/trimmer"
)

func newPod(name, ns, phase string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":              name,
				"namespace":         ns,
				"creationTimestamp": "2026-04-10T09:00:00Z",
				"labels":            map[string]interface{}{"app": "api"},
			},
			"spec": map[string]interface{}{
				"nodeName":   "node-1",
				"containers": []interface{}{map[string]interface{}{"name": "main"}},
			},
			"status": map[string]interface{}{
				"phase": phase,
				"containerStatuses": []interface{}{
					map[string]interface{}{
						"name":         "main",
						"ready":        true,
						"restartCount": int64(0),
						"state":        map[string]interface{}{"running": map[string]interface{}{}},
					},
				},
			},
		},
	}
}

func fakeDeps(t *testing.T, objs ...runtime.Object) *Deps {
	t.Helper()
	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	listKinds := map[schema.GroupVersionResource]string{gvr: "PodList"}
	return &Deps{
		Dynamic:    dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objs...),
		Mapper:     &testMapper{gvr: gvr},
		Logger:     slog.Default(),
		Projectors: &trimmer.Projectors{},
		Cluster:    "https://test",
	}
}

func TestKubectlGet_ListMode(t *testing.T) {
	d := fakeDeps(t,
		newPod("api-1", "prod", "Running"),
		newPod("api-2", "prod", "Running"),
	)
	handler := NewKubectlGetHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"kind":      "Pod",
		"namespace": "prod",
	}

	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		Kind          string                   `json:"kind"`
		ReturnedCount int                      `json:"returnedCount"`
		Truncated     bool                     `json:"truncated"`
		Items         []map[string]interface{} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))

	assert.Equal(t, "Pod", payload.Kind)
	assert.Equal(t, 2, payload.ReturnedCount)
	assert.False(t, payload.Truncated)
	assert.Len(t, payload.Items, 2)
	assert.Equal(t, "Running", payload.Items[0]["phase"])
}

func TestKubectlGet_GetMode(t *testing.T) {
	d := fakeDeps(t, newPod("api-1", "prod", "Running"))
	handler := NewKubectlGetHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"kind":      "Pod",
		"namespace": "prod",
		"name":      "api-1",
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var obj map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &obj))
	meta := obj["metadata"].(map[string]interface{})
	assert.Equal(t, "api-1", meta["name"])
	_, hasManagedFields := meta["managedFields"]
	assert.False(t, hasManagedFields)
}

func TestKubectlGet_MissingKind(t *testing.T) {
	d := fakeDeps(t)
	handler := NewKubectlGetHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"namespace": "prod"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func textOf(r *mcp.CallToolResult) string {
	for _, c := range r.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// --- test doubles -----------------------------------------------------------

type testMapper struct{ gvr schema.GroupVersionResource }

func (m *testMapper) ResolveGVR(kind, apiVersion string) (schema.GroupVersionResource, bool, error) {
	return m.gvr, true, nil
}

var _ = metav1.ListOptions{}
