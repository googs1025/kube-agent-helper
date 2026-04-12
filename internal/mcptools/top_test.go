package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metricsv1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestTopPods_Unavailable(t *testing.T) {
	d := &Deps{Metrics: nil}
	handler := NewTopPodsHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"namespace": "prod"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError, "unavailable is not an error")

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, false, payload["available"])
	assert.Contains(t, payload["error"], "metrics-server not installed")
}

func TestTopPods_ReturnsMetrics(t *testing.T) {
	pm := &metricsv1.PodMetrics{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
		Containers: []metricsv1.ContainerMetrics{{
			Name: "main",
			Usage: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1250m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		}},
	}
	pmList := &metricsv1.PodMetricsList{Items: []metricsv1.PodMetrics{*pm}}

	metrics := metricsfake.NewSimpleClientset()
	// The fake metrics client uses resource "pods" under metrics.k8s.io/v1beta1,
	// but the object tracker guesses "podmetricses". Inject a reactor instead.
	metrics.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, pmList, nil
	})

	d := &Deps{Metrics: metrics}
	handler := NewTopPodsHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"namespace": "prod"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		Available bool                     `json:"available"`
		Items     []map[string]interface{} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.True(t, payload.Available)
	require.Len(t, payload.Items, 1)
	assert.Equal(t, "api", payload.Items[0]["name"])
	assert.Equal(t, float64(1250), payload.Items[0]["cpuMilli"])
	assert.Equal(t, float64(512), payload.Items[0]["memoryMi"])
}

func TestTopNodes_Unavailable(t *testing.T) {
	d := &Deps{Metrics: nil}
	handler := NewTopNodesHandler(d)
	result, err := handler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	require.False(t, result.IsError)
	assert.Contains(t, textOf(result), "metrics-server not installed")
}
