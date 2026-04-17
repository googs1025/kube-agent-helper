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
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestPVCStatus_BoundPVC(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data-vol", Namespace: "prod"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
			StorageClassName: strPtr("standard"),
			VolumeName:       "pv-123",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("10Gi"),
			},
		},
	}

	client := k8sfake.NewSimpleClientset(pvc)
	d := &Deps{Typed: client}
	handler := NewPVCStatusHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"namespace": "prod"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		Items []struct {
			Name         string `json:"name"`
			Phase        string `json:"phase"`
			VolumeName   string `json:"volumeName"`
			StorageClass string `json:"storageClass"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	require.Len(t, payload.Items, 1)
	assert.Equal(t, "data-vol", payload.Items[0].Name)
	assert.Equal(t, "Bound", payload.Items[0].Phase)
	assert.Equal(t, "pv-123", payload.Items[0].VolumeName)
}

func TestPVCStatus_MissingNamespace(t *testing.T) {
	client := k8sfake.NewSimpleClientset()
	d := &Deps{Typed: client}
	handler := NewPVCStatusHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func strPtr(s string) *string { return &s }
