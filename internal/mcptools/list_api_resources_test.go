package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestListAPIResources_Basic(t *testing.T) {
	fakeDisc := &fake.FakeDiscovery{
		Fake: &k8stesting.Fake{},
	}
	fakeDisc.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "watch"}},
				{Name: "nodes", Kind: "Node", Namespaced: false, Verbs: metav1.Verbs{"get", "list", "watch"}},
				{Name: "pods/log", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"get"}}, // subresource, skip
			},
		},
	}
	d := &Deps{Discovery: fakeDisc}
	handler := NewListAPIResourcesHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		Count     int                      `json:"count"`
		Resources []map[string]interface{} `json:"resources"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, 2, payload.Count) // pods/log subresource excluded
}

func TestListAPIResources_VerbFilter(t *testing.T) {
	fakeDisc := &fake.FakeDiscovery{
		Fake: &k8stesting.Fake{},
	}
	fakeDisc.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"get", "list"}},
				{Name: "events", Kind: "Event", Namespaced: true, Verbs: metav1.Verbs{"get", "list", "create"}},
			},
		},
	}
	d := &Deps{Discovery: fakeDisc}
	handler := NewListAPIResourcesHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"verb": "create"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)

	var payload struct {
		Count int `json:"count"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, 1, payload.Count)
}