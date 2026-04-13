package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kube-agent-helper/kube-agent-helper/internal/trimmer"
)

func TestKubectlDescribe_ReturnsObjectAndEvents(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pod := newPod("api-1", "prod", "Running")
	pod.SetUID(types.UID("pod-uid-1"))

	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	listKinds := map[schema.GroupVersionResource]string{gvr: "PodList"}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, pod)

	typed := fake.NewSimpleClientset(&corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "evt1"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod", Name: "api-1", Namespace: "prod", UID: types.UID("pod-uid-1"),
		},
		Type:    "Warning",
		Reason:  "BackOff",
		Message: "Back-off restarting",
		Count:   5,
	})

	d := &Deps{
		Dynamic: dyn, Typed: typed,
		Mapper:     &testMapper{gvr: gvr},
		Projectors: &trimmer.Projectors{},
	}
	handler := NewKubectlDescribeHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"kind":      "Pod",
		"namespace": "prod",
		"name":      "api-1",
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		Object        map[string]interface{}   `json:"object"`
		RelatedEvents []map[string]interface{} `json:"relatedEvents"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))

	assert.Equal(t, "api-1", payload.Object["metadata"].(map[string]interface{})["name"])
	require.Len(t, payload.RelatedEvents, 1)
	assert.Equal(t, "BackOff", payload.RelatedEvents[0]["reason"])
}
