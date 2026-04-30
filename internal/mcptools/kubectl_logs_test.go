package mcptools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubectlLogs_MissingArgs(t *testing.T) {
	d := &Deps{Typed: fake.NewSimpleClientset()}
	handler := NewKubectlLogsHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestKubectlLogs_MultiContainerRequiresExplicit(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "main"}, {Name: "sidecar"},
			},
		},
	}
	d := &Deps{Typed: fake.NewSimpleClientset(pod)}
	handler := NewKubectlLogsHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"namespace": "prod",
		"pod":       "api",
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textOf(result), "main")
	assert.Contains(t, textOf(result), "sidecar")
}

func TestKubectlLogs_TailLinesTooLarge(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
	}
	d := &Deps{Typed: fake.NewSimpleClientset(pod)}
	handler := NewKubectlLogsHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"namespace": "ns", "pod": "p",
		"tailLines": float64(5000), // > maxTailLines (2000)
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textOf(result), "tailLines")
}

func TestKubectlLogs_TailLinesNegative(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}},
	}
	d := &Deps{Typed: fake.NewSimpleClientset(pod)}
	handler := NewKubectlLogsHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"namespace": "ns", "pod": "p",
		"tailLines": float64(-1),
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, textOf(result), "tailLines")
}

func TestKubectlLogs_SingleContainerSucceeds(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}},
	}
	d := &Deps{Typed: fake.NewSimpleClientset(pod)}
	handler := NewKubectlLogsHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"namespace": "ns", "pod": "p",
		"tailLines": float64(100),
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.IsError, "single-container path should succeed: %s", textOf(result))
	// The result body is JSON with "logs", "truncated", "lineCount" keys.
	assert.Contains(t, textOf(result), "truncated")
	assert.Contains(t, textOf(result), "lineCount")
}

func TestKubectlLogs_ExplicitContainerWithSinceSecondsAndPrevious(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "main"}, {Name: "sidecar"},
		}},
	}
	d := &Deps{Typed: fake.NewSimpleClientset(pod)}
	handler := NewKubectlLogsHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"namespace":    "ns",
		"pod":          "p",
		"container":    "main",
		"sinceSeconds": float64(300),
		"previous":     true,
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.IsError, "explicit container path should succeed: %s", textOf(result))
}

func TestKubectlLogs_PodNotFound(t *testing.T) {
	d := &Deps{Typed: fake.NewSimpleClientset()} // no pods
	handler := NewKubectlLogsHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"namespace": "ns", "pod": "ghost",
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}
