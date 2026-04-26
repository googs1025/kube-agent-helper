package notification

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookSend(t *testing.T) {
	var received []byte
	var headers http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := NewWebhookChannel(srv.URL, "")
	evt := Event{
		Type:      EventRunCompleted,
		Severity:  "info",
		Title:     "Run completed",
		Message:   "All checks passed",
		Resource:  "run-123",
		Namespace: "default",
		Cluster:   "local",
		Timestamp: time.Now(),
	}

	err := ch.Send(context.Background(), evt)
	require.NoError(t, err)

	assert.Equal(t, "application/json", headers.Get("Content-Type"))
	assert.Empty(t, headers.Get("X-Signature-256"))

	var got Event
	require.NoError(t, json.Unmarshal(received, &got))
	assert.Equal(t, EventRunCompleted, got.Type)
	assert.Equal(t, "run-123", got.Resource)
}

func TestWebhookHMAC(t *testing.T) {
	secret := "my-secret-key"
	var received []byte
	var sigHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sigHeader = r.Header.Get("X-Signature-256")
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := NewWebhookChannel(srv.URL, secret)
	evt := Event{Type: EventFixApplied, Resource: "fix-1", Timestamp: time.Now()}

	err := ch.Send(context.Background(), evt)
	require.NoError(t, err)

	// Verify signature
	require.NotEmpty(t, sigHeader)
	assert.True(t, len(sigHeader) > 7) // "sha256=" prefix
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(received)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	assert.Equal(t, expected, sigHeader)
}

func TestWebhookServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ch := NewWebhookChannel(srv.URL, "")
	evt := Event{Type: EventRunFailed, Resource: "run-err", Timestamp: time.Now()}

	err := ch.Send(context.Background(), evt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestWebhookName(t *testing.T) {
	ch := NewWebhookChannel("http://example.com", "")
	assert.Equal(t, "webhook", ch.Name())
}
