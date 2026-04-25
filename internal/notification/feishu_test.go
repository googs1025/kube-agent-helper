package notification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeishuSend(t *testing.T) {
	var received map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := NewFeishuChannel(srv.URL, "feishu-secret")
	evt := Event{
		Type:      EventCriticalFinding,
		Severity:  "critical",
		Title:     "Critical Issue",
		Message:   "OOM detected",
		Resource:  "pod-xyz",
		Namespace: "production",
		Cluster:   "prod-cluster",
		Timestamp: time.Now(),
	}

	err := ch.Send(context.Background(), evt)
	require.NoError(t, err)

	// Check msg_type
	assert.Equal(t, "interactive", received["msg_type"])

	// Check signing fields are present
	assert.NotEmpty(t, received["timestamp"])
	assert.NotEmpty(t, received["sign"])

	// Check card structure
	card, ok := received["card"].(map[string]interface{})
	require.True(t, ok)
	header := card["header"].(map[string]interface{})
	assert.Equal(t, "red", header["template"])

	elements := card["elements"].([]interface{})
	require.Len(t, elements, 1)
}

func TestFeishuNoSecret(t *testing.T) {
	var received map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := NewFeishuChannel(srv.URL, "")
	evt := Event{Type: EventRunCompleted, Resource: "run-1", Severity: "info", Timestamp: time.Now()}

	err := ch.Send(context.Background(), evt)
	require.NoError(t, err)

	// No signing fields
	_, hasTS := received["timestamp"]
	_, hasSign := received["sign"]
	assert.False(t, hasTS)
	assert.False(t, hasSign)

	// Template should be green for info
	card := received["card"].(map[string]interface{})
	header := card["header"].(map[string]interface{})
	assert.Equal(t, "green", header["template"])
}

func TestFeishuServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ch := NewFeishuChannel(srv.URL, "")
	err := ch.Send(context.Background(), Event{Type: EventRunFailed, Resource: "r", Timestamp: time.Now()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

func TestFeishuName(t *testing.T) {
	ch := NewFeishuChannel("http://example.com", "")
	assert.Equal(t, "feishu", ch.Name())
}
