package notification

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebhookChannel sends events as JSON POST requests to a generic webhook URL.
// If Secret is set, the payload is signed with HMAC-SHA256 and the signature
// is sent in the X-Signature-256 header.
type WebhookChannel struct {
	URL     string
	Secret  string
	Timeout time.Duration
	client  *http.Client
}

// NewWebhookChannel creates a WebhookChannel with sensible defaults.
func NewWebhookChannel(url, secret string) *WebhookChannel {
	timeout := 10 * time.Second
	return &WebhookChannel{
		URL:     url,
		Secret:  secret,
		Timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

func (w *WebhookChannel) Name() string { return "webhook" }

func (w *WebhookChannel) Send(ctx context.Context, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("webhook: marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if w.Secret != "" {
		mac := hmac.New(sha256.New, []byte(w.Secret))
		mac.Write(body)
		sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Signature-256", sig)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: send: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook: returned %d", resp.StatusCode)
	}
	return nil
}
