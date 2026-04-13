package mcptools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEventsList_FiltersByType(t *testing.T) {
	base := metav1.NewTime(time.Now())
	typed := fake.NewSimpleClientset(
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Namespace: "prod", Name: "e1"},
			Type:           "Warning",
			Reason:         "BackOff",
			Message:        "x",
			LastTimestamp:  base,
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "api"},
		},
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Namespace: "prod", Name: "e2"},
			Type:           "Normal",
			Reason:         "Scheduled",
			Message:        "ok",
			LastTimestamp:  base,
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "api"},
		},
	)
	d := &Deps{Typed: typed}
	handler := NewEventsListHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"namespace": "prod",
		"types":     []interface{}{"Warning"},
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		ReturnedCount int                      `json:"returnedCount"`
		Events        []map[string]interface{} `json:"events"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, 1, payload.ReturnedCount)
	assert.Equal(t, "BackOff", payload.Events[0]["reason"])
}
