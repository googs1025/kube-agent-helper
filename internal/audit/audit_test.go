package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaskArgs_KeepsKnownFields(t *testing.T) {
	whitelist := []string{"kind", "namespace", "name", "labelSelector"}
	in := map[string]interface{}{
		"kind":          "Pod",
		"namespace":     "prod",
		"labelSelector": "app=api",
		"token":         "should-be-dropped", // not in whitelist
	}
	got := MaskArgs(in, whitelist)
	assert.Equal(t, map[string]interface{}{
		"kind":          "Pod",
		"namespace":     "prod",
		"labelSelector": "app=api",
	}, got)
}

func TestMaskArgs_EmptyWhitelistDropsAll(t *testing.T) {
	got := MaskArgs(map[string]interface{}{"x": 1}, nil)
	assert.Empty(t, got)
}

func captureLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(h), buf
}

func TestMiddleware_LogsSuccess(t *testing.T) {
	logger, buf := captureLogger()
	called := false
	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText(`{"items":[]}`), nil
	}

	spec := ToolSpec{
		Name:         "kubectl_get",
		ArgWhitelist: []string{"kind", "namespace"},
		Cluster:      "https://example:6443",
	}
	wrapped := Wrap(logger, spec, handler)

	req := mcp.CallToolRequest{}
	req.Params.Name = "kubectl_get"
	req.Params.Arguments = map[string]interface{}{
		"kind":      "Pod",
		"namespace": "prod",
		"dropped":   "should-not-appear",
	}

	_, err := wrapped(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, called)

	var entry map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))

	assert.Equal(t, "tool_call", entry["msg"])
	assert.Equal(t, "kubectl_get", entry["tool"])
	assert.Equal(t, "https://example:6443", entry["cluster"])

	args := entry["args"].(map[string]interface{})
	assert.Equal(t, "Pod", args["kind"])
	assert.Equal(t, "prod", args["namespace"])
	_, hasDropped := args["dropped"]
	assert.False(t, hasDropped)

	result := entry["result"].(map[string]interface{})
	assert.Equal(t, true, result["ok"])
	assert.NotNil(t, entry["trace_id"])
	assert.NotNil(t, entry["latency_ms"])
}

func TestMiddleware_LogsError(t *testing.T) {
	logger, buf := captureLogger()
	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultError("pod not found"), nil
	}
	wrapped := Wrap(logger, ToolSpec{Name: "kubectl_logs"}, handler)

	req := mcp.CallToolRequest{}
	req.Params.Name = "kubectl_logs"
	req.Params.Arguments = map[string]interface{}{}
	_, err := wrapped(context.Background(), req)
	require.NoError(t, err)

	var entry map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "ERROR", entry["level"])
	result := entry["result"].(map[string]interface{})
	assert.Equal(t, false, result["ok"])
	assert.Contains(t, entry["error"], "pod not found")
}

func TestMiddleware_TimestampIncreasing(t *testing.T) {
	// Sanity: latency_ms >= 0
	logger, buf := captureLogger()
	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(5 * time.Millisecond)
		return mcp.NewToolResultText("ok"), nil
	}
	wrapped := Wrap(logger, ToolSpec{Name: "t"}, handler)
	_, _ = wrapped(context.Background(), mcp.CallToolRequest{})

	var entry map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	lat, _ := entry["latency_ms"].(float64)
	assert.GreaterOrEqual(t, lat, float64(0))
}

func TestMiddleware_RecoversPanic(t *testing.T) {
	logger, buf := captureLogger()
	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		panic("something went wrong")
	}
	wrapped := Wrap(logger, ToolSpec{Name: "kubectl_panic", Cluster: "https://panic:6443"}, handler)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := wrapped(context.Background(), req)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsError)

	var entry map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
	assert.Equal(t, "ERROR", entry["level"])
	assert.Equal(t, "panic", entry["error"])
}

func TestNew_InfoLevel(t *testing.T) {
	logger := New("info")
	assert.NotNil(t, logger)
}

func TestNew_DebugLevel(t *testing.T) {
	logger := New("debug")
	assert.NotNil(t, logger)
}

func TestNew_DefaultsToInfo(t *testing.T) {
	logger := New("unknown")
	assert.NotNil(t, logger)
}
