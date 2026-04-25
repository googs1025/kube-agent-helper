package notification

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// DingTalkChannel sends notifications to a DingTalk robot webhook.
// If Secret is set, the request URL is signed with HMAC-SHA256 using the
// DingTalk timestamp+sign query parameter convention.
type DingTalkChannel struct {
	WebhookURL string
	Secret     string
	client     *http.Client
}

// NewDingTalkChannel creates a DingTalkChannel.
func NewDingTalkChannel(webhookURL, secret string) *DingTalkChannel {
	return &DingTalkChannel{
		WebhookURL: webhookURL,
		Secret:     secret,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (d *DingTalkChannel) Name() string { return "dingtalk" }

// signURL appends timestamp and sign query parameters to the webhook URL.
func (d *DingTalkChannel) signURL() string {
	if d.Secret == "" {
		return d.WebhookURL
	}
	ts := fmt.Sprintf("%d", time.Now().UnixMilli())
	stringToSign := ts + "\n" + d.Secret
	mac := hmac.New(sha256.New, []byte(d.Secret))
	mac.Write([]byte(stringToSign))
	encoded := url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	return fmt.Sprintf("%s&timestamp=%s&sign=%s", d.WebhookURL, ts, encoded)
}

func (d *DingTalkChannel) Send(ctx context.Context, event Event) error {
	text := fmt.Sprintf("## %s\n\n%s\n\n> Cluster: %s | NS: %s | Resource: %s",
		event.Title, event.Message, event.Cluster, event.Namespace, event.Resource)

	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": event.Title,
			"text":  text,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("dingtalk: marshal: %w", err)
	}

	targetURL := d.signURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("dingtalk: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("dingtalk: send: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("dingtalk: returned %d", resp.StatusCode)
	}
	return nil
}
