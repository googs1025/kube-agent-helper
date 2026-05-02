# Notification / Webhook Setup Guide

KubeDoctor can send real-time notifications when diagnostic runs complete, critical findings are detected, or fixes change status. Four notification channels are supported out of the box.

## Architecture

```
Reconciler Events --> NotificationManager --> Channel Router --> Webhook / Slack / DingTalk / Feishu
                          |
                      Dedup Cache (in-memory, 5 min default)
```

The `NotificationManager` receives typed events from reconcilers and the HTTP API, deduplicates them via a TTL cache keyed by `{eventType}:{resource}`, then fans out to all configured channels.

## Event Types

| Event Type         | When Fired                          | Default Severity |
|--------------------|-------------------------------------|------------------|
| `run.completed`    | Diagnostic run succeeds             | info             |
| `run.failed`       | Diagnostic run fails or times out   | warning          |
| `finding.critical` | A critical-severity finding appears  | critical         |
| `fix.applied`      | A fix patch is successfully applied | info             |
| `fix.failed`       | A fix fails to apply                | warning          |
| `fix.approved`     | A fix is approved via the API       | info             |
| `fix.rejected`     | A fix is rejected via the API       | warning          |

## Supported Channels

### Generic Webhook

Sends the raw `Event` JSON to any HTTP endpoint. Optionally signs the payload with HMAC-SHA256.

**Helm values:**

```yaml
notifications:
  webhook:
    enabled: true
    url: "https://your-endpoint.example.com/webhook"
    secret: "your-hmac-secret"   # optional
```

**CLI flags:**

```
--notify-webhook-url=https://...
--notify-webhook-secret=your-secret
```

**Signature verification (Python example):**

```python
import hmac, hashlib

def verify(payload_bytes, signature_header, secret):
    expected = "sha256=" + hmac.new(
        secret.encode(), payload_bytes, hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(expected, signature_header)
```

**Signature verification (Go example):**

```go
func verify(body []byte, sigHeader, secret string) bool {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(body)
    expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(expected), []byte(sigHeader))
}
```

### Slack

Posts rich messages using Slack Block Kit with color-coded attachments.

```yaml
notifications:
  slack:
    enabled: true
    webhookURL: "https://hooks.slack.com/services/T.../B.../xxx"
```

### DingTalk

Sends markdown messages to a DingTalk robot webhook with optional HMAC-SHA256 signing (timestamp + sign query parameters).

```yaml
notifications:
  dingtalk:
    enabled: true
    webhookURL: "https://oapi.dingtalk.com/robot/send?access_token=..."
    secret: "SEC..."   # optional signing secret
```

### Feishu (Lark)

Sends interactive card messages to a Feishu bot webhook with optional HMAC-SHA256 signing (timestamp + sign in the JSON body).

```yaml
notifications:
  feishu:
    enabled: true
    webhookURL: "https://open.feishu.cn/open-apis/bot/v2/hook/..."
    secret: "..."   # optional signing secret
```

## Helm Configuration Reference

```yaml
notifications:
  dedupTTL: "5m"              # Deduplication window
  webhook:
    enabled: false
    url: ""
    secret: ""
  slack:
    enabled: false
    webhookURL: ""
  dingtalk:
    enabled: false
    webhookURL: ""
    secret: ""
  feishu:
    enabled: false
    webhookURL: ""
    secret: ""
```

## Deduplication

Events with the same `(type, resource)` key are deduplicated within the configured TTL window (default 5 minutes). This prevents notification storms when a reconciler requeues rapidly.

The dedup cache is in-memory and resets on controller restart.

## Troubleshooting

| Symptom | Possible Cause | Fix |
|---------|---------------|-----|
| No notifications received | Channel not enabled in Helm values | Set `notifications.<channel>.enabled: true` |
| Duplicate notifications | TTL too short or controller restarted | Increase `notifications.dedupTTL` |
| Webhook returns 403 | HMAC secret mismatch | Verify `--notify-webhook-secret` matches receiver |
| DingTalk returns error code | Clock skew or wrong secret | Sync server time; verify signing secret |
| Slack shows "invalid_payload" | Malformed webhook URL | Use the full `https://hooks.slack.com/...` URL |
