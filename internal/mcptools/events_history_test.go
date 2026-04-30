package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// mockEventsStore overrides ListEvents on top of nopStore.
type mockEventsStore struct {
	nopStore
	events       []*store.Event
	err          error
	capturedOpts store.ListEventsOpts
}

func (m *mockEventsStore) ListEvents(_ context.Context, opts store.ListEventsOpts) ([]*store.Event, error) {
	m.capturedOpts = opts
	return m.events, m.err
}

// --- Tests ---

func TestEventsHistory_NoStore(t *testing.T) {
	d := &Deps{Store: nil}
	handler := NewEventsHistoryHandler(d)

	req := mcp.CallToolRequest{}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, false, payload["available"])
	assert.Equal(t, "event store not available", payload["error"])
}

func TestEventsHistory_DefaultOpts(t *testing.T) {
	ms := &mockEventsStore{}
	d := &Deps{Store: ms}
	handler := NewEventsHistoryHandler(d)

	req := mcp.CallToolRequest{}
	// No arguments — all defaults apply.
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	assert.Equal(t, 100, ms.capturedOpts.Limit, "expected default Limit=100")
	assert.Equal(t, "", ms.capturedOpts.Namespace)
	assert.Equal(t, "", ms.capturedOpts.Name)
	assert.Equal(t, 0, ms.capturedOpts.SinceMinutes)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, float64(0), payload["count"])
}

func TestEventsHistory_AllArgsParsed(t *testing.T) {
	ms := &mockEventsStore{}
	d := &Deps{Store: ms}
	handler := NewEventsHistoryHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"namespace":     "kube-system",
		"name":          "coredns",
		"since_minutes": float64(30),
		"limit":         float64(50),
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	assert.Equal(t, "kube-system", ms.capturedOpts.Namespace)
	assert.Equal(t, "coredns", ms.capturedOpts.Name)
	assert.Equal(t, 30, ms.capturedOpts.SinceMinutes)
	assert.Equal(t, 50, ms.capturedOpts.Limit)
}

func TestEventsHistory_ZeroLimitKeepsDefault(t *testing.T) {
	ms := &mockEventsStore{}
	d := &Deps{Store: ms}
	handler := NewEventsHistoryHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"limit": float64(0), // zero — must NOT override the default 100
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	assert.Equal(t, 100, ms.capturedOpts.Limit, "zero limit must not override default of 100")
}

func TestEventsHistory_StoreError(t *testing.T) {
	ms := &mockEventsStore{err: errors.New("db down")}
	d := &Deps{Store: ms}
	handler := NewEventsHistoryHandler(d)

	req := mcp.CallToolRequest{}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, "db down", payload["error"])
}
