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

func TestDingTalkSend(t *testing.T) {
	var received map[string]interface{}
	var reqURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqURL = r.URL.String()
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := NewDingTalkChannel(srv.URL+"?access_token=test", "mysecret")
	evt := Event{
		Type:      EventRunCompleted,
		Severity:  "info",
		Title:     "Run done",
		Message:   "All good",
		Resource:  "run-1",
		Namespace: "default",
		Cluster:   "local",
		Timestamp: time.Now(),
	}

	err := ch.Send(context.Background(), evt)
	require.NoError(t, err)

	// Check payload structure
	assert.Equal(t, "markdown", received["msgtype"])
	md, ok := received["markdown"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Run done", md["title"])
	assert.Contains(t, md["text"], "## Run done")
	assert.Contains(t, md["text"], "Cluster: local")

	// Check signing params in URL
	assert.Contains(t, reqURL, "timestamp=")
	assert.Contains(t, reqURL, "sign=")
}

func TestDingTalkNoSecret(t *testing.T) {
	var reqURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqURL = r.URL.String()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := NewDingTalkChannel(srv.URL+"?access_token=test", "")
	evt := Event{Type: EventFixApplied, Resource: "fix-1", Timestamp: time.Now()}

	err := ch.Send(context.Background(), evt)
	require.NoError(t, err)

	// Without secret, no sign params
	assert.NotContains(t, reqURL, "sign=")
	assert.NotContains(t, reqURL, "timestamp=")
}

func TestDingTalkServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	ch := NewDingTalkChannel(srv.URL, "")
	err := ch.Send(context.Background(), Event{Type: EventRunFailed, Resource: "r", Timestamp: time.Now()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}

func TestDingTalkName(t *testing.T) {
	ch := NewDingTalkChannel("http://example.com", "")
	assert.Equal(t, "dingtalk", ch.Name())
}
