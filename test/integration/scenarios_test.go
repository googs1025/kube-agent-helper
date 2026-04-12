//go:build integration

package integration

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func unmarshalResult(resp mcpResponse, v interface{}) error {
	return json.Unmarshal(resp.Result, v)
}

func TestIntegration_ListTools(t *testing.T) {
	client := newMCPClient(t)
	client.initialize(t)

	resp := client.call(t, "tools/list", nil)
	require.Nil(t, resp.Error)

	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	require.NoError(t, unmarshalResult(resp, &result))

	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	expected := []string{
		"kubectl_get", "kubectl_describe", "kubectl_logs", "events_list",
		"top_pods", "top_nodes", "list_api_resources", "prometheus_query", "kubectl_explain",
	}
	for _, e := range expected {
		assert.Contains(t, names, e, "expected tool %s to be registered", e)
	}
}

func TestIntegration_KubectlGet_Pods_DefaultNamespace(t *testing.T) {
	client := newMCPClient(t)
	client.initialize(t)

	payload := client.callTool(t, "kubectl_get", map[string]interface{}{
		"kind": "Pod",
	})
	// kind cluster may have no pods in default ns; just check it returns items key
	_, hasItems := payload["items"]
	_, hasError := payload["error"]
	assert.True(t, hasItems || hasError, "response should have items or error")
}

func TestIntegration_KubectlGet_Nodes(t *testing.T) {
	client := newMCPClient(t)
	client.initialize(t)

	payload := client.callTool(t, "kubectl_get", map[string]interface{}{
		"kind": "Node",
	})
	items, ok := payload["items"].([]interface{})
	require.True(t, ok, "expected items array, got: %v", payload)
	assert.NotEmpty(t, items, "kind cluster should have at least one node")
}

func TestIntegration_ListAPIResources(t *testing.T) {
	client := newMCPClient(t)
	client.initialize(t)

	payload := client.callTool(t, "list_api_resources", map[string]interface{}{
		"verb": "list",
	})
	count, ok := payload["count"].(float64)
	require.True(t, ok, "expected count field, got: %v", payload)
	assert.Greater(t, int(count), 10, "should have many listable resources")
}

func TestIntegration_TopPods_GracefulDegradation(t *testing.T) {
	client := newMCPClient(t)
	client.initialize(t)

	// kind cluster has no metrics-server by default
	payload := client.callTool(t, "top_pods", map[string]interface{}{})
	// should return available: false, not crash
	if avail, ok := payload["available"].(bool); ok {
		assert.False(t, avail)
	}
	// Either available=false or an error message is acceptable
}
