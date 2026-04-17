package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestNetworkPolicyCheck_MissingArgs(t *testing.T) {
	client := k8sfake.NewSimpleClientset()
	d := &Deps{Typed: client}
	handler := NewNetworkPolicyCheckHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestNetworkPolicyCheck_MatchingPolicy(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web-1", Namespace: "prod",
			Labels: map[string]string{"app": "web", "tier": "frontend"},
		},
	}
	port80 := intstr.FromInt(80)
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-frontend", Namespace: "prod"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "api"},
					},
				}},
				Ports: []networkingv1.NetworkPolicyPort{{
					Port: &port80,
				}},
			}},
		},
	}

	client := k8sfake.NewSimpleClientset(pod, policy)
	d := &Deps{Typed: client}
	handler := NewNetworkPolicyCheckHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"namespace": "prod", "podName": "web-1",
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		PodLabels        map[string]string        `json:"podLabels"`
		MatchingPolicies []map[string]interface{} `json:"matchingPolicies"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, "web", payload.PodLabels["app"])
	require.Len(t, payload.MatchingPolicies, 1)
	assert.Equal(t, "allow-frontend", payload.MatchingPolicies[0]["name"])
}