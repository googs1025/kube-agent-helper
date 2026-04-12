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
