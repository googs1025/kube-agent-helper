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

func TestSlackSend(t *testing.T) {
	var received map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := NewSlackChannel(srv.URL)
	evt := Event{
		Type:      EventRunCompleted,
		Severity:  "info",
		Title:     "Run completed",
		Message:   "Diagnostic run finished",
		Resource:  "run-abc",
		Namespace: "default",
		Cluster:   "local",
		Timestamp: time.Now(),
	}

	err := ch.Send(context.Background(), evt)
	require.NoError(t, err)

	attachments, ok := received["attachments"].([]interface{})
	require.True(t, ok)
	require.Len(t, attachments, 1)

	att := attachments[0].(map[string]interface{})
	assert.Equal(t, "#36A64F", att["color"]) // info severity = green

	blocks := att["blocks"].([]interface{})
	require.Len(t, blocks, 3) // header, section, context
}

func TestSlackCriticalColor(t *testing.T) {
	var received map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := NewSlackChannel(srv.URL)
	evt := Event{
		Type:      EventCriticalFinding,
		Severity:  "critical",
		Title:     "Critical finding",
		Message:   "Memory OOM detected",
		Resource:  "pod-xyz",
		Timestamp: time.Now(),
	}

	err := ch.Send(context.Background(), evt)
	require.NoError(t, err)

	attachments := received["attachments"].([]interface{})
	att := attachments[0].(map[string]interface{})
	assert.Equal(t, "#FF0000", att["color"]) // critical = red
}

func TestSlackServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	ch := NewSlackChannel(srv.URL)
	err := ch.Send(context.Background(), Event{Type: EventRunFailed, Resource: "r", Timestamp: time.Now()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

func TestSlackName(t *testing.T) {
	ch := NewSlackChannel("http://example.com")
	assert.Equal(t, "slack", ch.Name())
}
