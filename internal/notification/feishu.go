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
	"time"
)

// FeishuChannel sends notifications to a Feishu (Lark) bot webhook using
// interactive card messages. If Secret is set, the payload includes a
// timestamp+sign for verification.
type FeishuChannel struct {
	WebhookURL string
	Secret     string
	client     *http.Client
}

// NewFeishuChannel creates a FeishuChannel.
func NewFeishuChannel(webhookURL, secret string) *FeishuChannel {
	return &FeishuChannel{
		WebhookURL: webhookURL,
		Secret:     secret,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (f *FeishuChannel) Name() string { return "feishu" }

// sign computes the Feishu HMAC-SHA256 signature.
// The string-to-sign is "{timestamp}\n{secret}" hashed with the secret as key.
func (f *FeishuChannel) sign(timestamp int64) string {
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, f.Secret)
	mac := hmac.New(sha256.New, []byte(stringToSign))
	mac.Write([]byte{})
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func templateForSeverity(severity string) string {
	switch severity {
	case "critical":
		return "red"
	case "warning":
		return "orange"
	default:
		return "green"
	}
}

func (f *FeishuChannel) Send(ctx context.Context, event Event) error {
	content := fmt.Sprintf("**%s**\n\n%s\n\nCluster: %s | Namespace: %s | Resource: %s",
		event.Title, event.Message, event.Cluster, event.Namespace, event.Resource)

	payload := map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"header": map[string]interface{}{
				"title":    map[string]string{"tag": "plain_text", "content": event.Title},
				"template": templateForSeverity(event.Severity),
			},
			"elements": []map[string]interface{}{
				{"tag": "div", "text": map[string]string{"tag": "lark_md", "content": content}},
			},
		},
	}

	if f.Secret != "" {
		ts := time.Now().Unix()
		payload["timestamp"] = fmt.Sprintf("%d", ts)
		payload["sign"] = f.sign(ts)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("feishu: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("feishu: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("feishu: send: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("feishu: returned %d", resp.StatusCode)
	}
	return nil
}
