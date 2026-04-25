package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SlackChannel sends notifications to a Slack incoming webhook using Block Kit.
type SlackChannel struct {
	WebhookURL string
	client     *http.Client
}

// NewSlackChannel creates a SlackChannel.
func NewSlackChannel(webhookURL string) *SlackChannel {
	return &SlackChannel{
		WebhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *SlackChannel) Name() string { return "slack" }

// emojiForType returns a Slack emoji prefix for the event type.
func emojiForType(t EventType) string {
	switch t {
	case EventRunCompleted:
		return ":white_check_mark:"
	case EventRunFailed:
		return ":x:"
	case EventCriticalFinding:
		return ":rotating_light:"
	case EventFixApplied:
		return ":wrench:"
	case EventFixFailed:
		return ":warning:"
	case EventFixApproved:
		return ":thumbsup:"
	case EventFixRejected:
		return ":no_entry_sign:"
	default:
		return ":bell:"
	}
}

func colorForSeverity(severity string) string {
	switch severity {
	case "critical":
		return "#FF0000"
	case "warning":
		return "#FFA500"
	default:
		return "#36A64F"
	}
}

func (s *SlackChannel) Send(ctx context.Context, event Event) error {
	emoji := emojiForType(event.Type)
	title := fmt.Sprintf("%s %s", emoji, event.Title)

	payload := map[string]interface{}{
		"attachments": []map[string]interface{}{{
			"color": colorForSeverity(event.Severity),
			"blocks": []map[string]interface{}{
				{
					"type": "header",
					"text": map[string]string{"type": "plain_text", "text": title},
				},
				{
					"type": "section",
					"text": map[string]string{"type": "mrkdwn", "text": event.Message},
				},
				{
					"type": "context",
					"elements": []map[string]string{
						{"type": "mrkdwn", "text": fmt.Sprintf("*Cluster:* %s | *Namespace:* %s | *Resource:* %s", event.Cluster, event.Namespace, event.Resource)},
					},
				},
			},
		}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack: send: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack: returned %d", resp.StatusCode)
	}
	return nil
}
