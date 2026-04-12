package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type fakeMapper struct {
	gvr        schema.GroupVersionResource
	namespaced bool
}

func (f *fakeMapper) ResolveGVR(kind, apiVersion string) (schema.GroupVersionResource, bool, error) {
	return f.gvr, f.namespaced, nil
}

type errMapper struct{}

func (e *errMapper) ResolveGVR(kind, apiVersion string) (schema.GroupVersionResource, bool, error) {
	return schema.GroupVersionResource{}, false, errors.New("not found")
}

func TestKubectlExplain_NoResource(t *testing.T) {
	d := &Deps{Mapper: &fakeMapper{gvr: schema.GroupVersionResource{Resource: "pods", Version: "v1"}}}
	handler := NewKubectlExplainHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.Contains(t, textOf(result), "resource is required")
}

func TestKubectlExplain_UnknownKind(t *testing.T) {
	d := &Deps{Mapper: &errMapper{}}
	handler := NewKubectlExplainHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"resource": "Frobnicator"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Contains(t, payload["error"], "unknown resource")
}
