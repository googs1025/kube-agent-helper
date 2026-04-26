# Notification/Webhook Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task.

**Issue:** #35 - Notification/Webhook

## Goal

Implement a pluggable notification system supporting generic webhooks, Slack, DingTalk, and Feishu (Lark). Notifications fire on diagnostic run completion, critical findings, and fix status changes. Each channel supports HMAC signing where applicable, and a deduplication layer prevents notification storms.

## Architecture

```
Reconciler Events --> NotificationManager --> Channel Router --> Webhook / Slack / DingTalk / Feishu
                          |
                      Dedup Cache (in-memory TTL)
```

The `NotificationManager` receives typed events, deduplicates them via a TTL cache keyed by `{eventType}:{resourceID}`, then fans out to all configured channels. Each channel implements the `Notifier` interface.

## Tech Stack

- Go `net/http` for webhook delivery
- HMAC-SHA256 signing for webhook security
- Slack Block Kit for rich messages
- DingTalk/Feishu bot webhook APIs
- In-memory TTL dedup map with `sync.Map`

## File Map

| File | Status |
|------|--------|
| `internal/notification/types.go` | New |
| `internal/notification/manager.go` | New |
| `internal/notification/webhook.go` | New |
| `internal/notification/slack.go` | New |
| `internal/notification/dingtalk.go` | New |
| `internal/notification/feishu.go` | New |
| `internal/notification/manager_test.go` | New |
| `internal/notification/webhook_test.go` | New |
| `internal/controller/reconciler/diagnosticrun_reconciler.go` | Modified |
| `internal/controller/reconciler/diagnosticfix_reconciler.go` | Modified |
| `internal/controller/httpserver/server.go` | Modified |
| `cmd/controller/main.go` | Modified |
| `deploy/helm/values.yaml` | Modified |
| `docs/notifications.md` | New |

## Tasks

### Task 1: Define Event types, Notifier interface, Manager with deduplication

- [ ] Create `internal/notification/types.go` with Event struct and severity levels
- [ ] Define `Notifier` interface with `Send(ctx, Event) error` and `Name() string`
- [ ] Create `internal/notification/manager.go` with dedup cache

**Files:** `internal/notification/types.go`, `internal/notification/manager.go`

**Steps:**

```go
// types.go
package notification

type EventType string

const (
    EventRunCompleted    EventType = "run.completed"
    EventRunFailed       EventType = "run.failed"
    EventCriticalFinding EventType = "finding.critical"
    EventFixApplied      EventType = "fix.applied"
    EventFixFailed       EventType = "fix.failed"
    EventFixApproved     EventType = "fix.approved"
    EventFixRejected     EventType = "fix.rejected"
)

type Event struct {
    Type      EventType         `json:"type"`
    Severity  string            `json:"severity"`
    Title     string            `json:"title"`
    Message   string            `json:"message"`
    Resource  string            `json:"resource"`
    Namespace string            `json:"namespace"`
    Cluster   string            `json:"cluster"`
    Timestamp time.Time         `json:"timestamp"`
    Metadata  map[string]string `json:"metadata,omitempty"`
}

type Notifier interface {
    Name() string
    Send(ctx context.Context, event Event) error
}
```

```go
// manager.go
type Manager struct {
    channels []Notifier
    dedup    sync.Map
    dedupTTL time.Duration
    logger   *slog.Logger
}

func NewManager(logger *slog.Logger, dedupTTL time.Duration) *Manager { ... }
func (m *Manager) Register(n Notifier) { ... }
func (m *Manager) Notify(ctx context.Context, event Event) error {
    key := fmt.Sprintf("%s:%s", event.Type, event.Resource)
    if _, loaded := m.dedup.LoadOrStore(key, time.Now()); loaded {
        return nil // deduplicated
    }
    time.AfterFunc(m.dedupTTL, func() { m.dedup.Delete(key) })
    var errs []error
    for _, ch := range m.channels {
        if err := ch.Send(ctx, event); err != nil {
            errs = append(errs, fmt.Errorf("%s: %w", ch.Name(), err))
        }
    }
    return errors.Join(errs...)
}
```

**Test:** `go test ./internal/notification/ -run TestManager`

**Commit:** `feat(notification): add event types, notifier interface, and manager with dedup`

### Task 2: Generic webhook channel with HMAC signing

- [ ] Create `internal/notification/webhook.go`
- [ ] POST JSON payload to configured URL
- [ ] Sign with HMAC-SHA256 in `X-Signature-256` header if secret is set
- [ ] Configurable timeout and retry (1 retry)

**Files:** `internal/notification/webhook.go`

**Steps:**

```go
type WebhookChannel struct {
    URL     string
    Secret  string
    Timeout time.Duration
    client  *http.Client
}

func (w *WebhookChannel) Send(ctx context.Context, event Event) error {
    body, _ := json.Marshal(event)
    req, _ := http.NewRequestWithContext(ctx, "POST", w.URL, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    if w.Secret != "" {
        mac := hmac.New(sha256.New, []byte(w.Secret))
        mac.Write(body)
        req.Header.Set("X-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
    }
    resp, err := w.client.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode >= 400 { return fmt.Errorf("webhook returned %d", resp.StatusCode) }
    return nil
}
```

**Test:** `go test ./internal/notification/ -run TestWebhook`

**Commit:** `feat(notification): add generic webhook channel with HMAC signing`

### Task 3: Slack incoming webhook channel

- [ ] Create `internal/notification/slack.go`
- [ ] Format using Slack Block Kit (header, section, context blocks)
- [ ] Color-code attachment by severity

**Files:** `internal/notification/slack.go`

**Steps:**

```go
type SlackChannel struct {
    WebhookURL string
    client     *http.Client
}

func (s *SlackChannel) Send(ctx context.Context, event Event) error {
    color := map[string]string{"critical": "#FF0000", "warning": "#FFA500", "info": "#36A64F"}
    payload := map[string]interface{}{
        "attachments": []map[string]interface{}{{
            "color": color[event.Severity],
            "blocks": []map[string]interface{}{
                {"type": "header", "text": map[string]string{"type": "plain_text", "text": event.Title}},
                {"type": "section", "text": map[string]string{"type": "mrkdwn", "text": event.Message}},
                {"type": "context", "elements": []map[string]string{
                    {"type": "mrkdwn", "text": fmt.Sprintf("*Cluster:* %s | *Namespace:* %s", event.Cluster, event.Namespace)},
                }},
            },
        }},
    }
    body, _ := json.Marshal(payload)
    req, _ := http.NewRequestWithContext(ctx, "POST", s.WebhookURL, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    resp, err := s.client.Do(req)
    // handle response...
    return err
}
```

**Test:** `go test ./internal/notification/ -run TestSlack`

**Commit:** `feat(notification): add slack webhook channel`

### Task 4: DingTalk robot webhook channel with signing

- [ ] Create `internal/notification/dingtalk.go`
- [ ] Implement DingTalk timestamp + HMAC-SHA256 signing
- [ ] Use DingTalk markdown message type

**Files:** `internal/notification/dingtalk.go`

**Steps:**

```go
type DingTalkChannel struct {
    WebhookURL string
    Secret     string
    client     *http.Client
}

func (d *DingTalkChannel) signURL() string {
    ts := fmt.Sprintf("%d", time.Now().UnixMilli())
    sign := ts + "\n" + d.Secret
    mac := hmac.New(sha256.New, []byte(d.Secret))
    mac.Write([]byte(sign))
    encoded := url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
    return fmt.Sprintf("%s&timestamp=%s&sign=%s", d.WebhookURL, ts, encoded)
}

func (d *DingTalkChannel) Send(ctx context.Context, event Event) error {
    payload := map[string]interface{}{
        "msgtype": "markdown",
        "markdown": map[string]string{
            "title": event.Title,
            "text":  fmt.Sprintf("## %s\n\n%s\n\n> Cluster: %s | NS: %s", event.Title, event.Message, event.Cluster, event.Namespace),
        },
    }
    // POST to d.signURL() ...
    return nil
}
```

**Test:** `go test ./internal/notification/ -run TestDingTalk`

**Commit:** `feat(notification): add dingtalk webhook channel with signing`

### Task 5: Feishu (Lark) bot webhook channel with signing

- [ ] Create `internal/notification/feishu.go`
- [ ] Implement Feishu timestamp + HMAC-SHA256 signing
- [ ] Use Feishu interactive card message type

**Files:** `internal/notification/feishu.go`

**Steps:**

```go
type FeishuChannel struct {
    WebhookURL string
    Secret     string
    client     *http.Client
}

func (f *FeishuChannel) sign(timestamp int64) string {
    data := fmt.Sprintf("%d\n%s", timestamp, f.Secret)
    mac := hmac.New(sha256.New, []byte(data))
    mac.Write([]byte{})
    return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (f *FeishuChannel) Send(ctx context.Context, event Event) error {
    ts := time.Now().Unix()
    payload := map[string]interface{}{
        "timestamp": fmt.Sprintf("%d", ts),
        "sign":      f.sign(ts),
        "msg_type":  "interactive",
        "card": map[string]interface{}{
            "header": map[string]interface{}{
                "title":    map[string]string{"tag": "plain_text", "content": event.Title},
                "template": "red",
            },
            "elements": []map[string]interface{}{
                {"tag": "div", "text": map[string]string{"tag": "lark_md", "content": event.Message}},
            },
        },
    }
    // POST to f.WebhookURL ...
    return nil
}
```

**Test:** `go test ./internal/notification/ -run TestFeishu`

**Commit:** `feat(notification): add feishu webhook channel with signing`

### Task 6: Integrate into DiagnosticRunReconciler

- [ ] On run completion: emit `EventRunCompleted` or `EventRunFailed`
- [ ] On critical finding detected: emit `EventCriticalFinding`
- [ ] Pass `NotificationManager` to reconciler constructor

**Files:** `internal/controller/reconciler/diagnosticrun_reconciler.go`

**Steps:**

- Add `notifier *notification.Manager` field to reconciler struct
- After run reaches `Completed` phase: `r.notifier.Notify(ctx, notification.Event{Type: EventRunCompleted, ...})`
- After run reaches `Failed` phase: `r.notifier.Notify(ctx, notification.Event{Type: EventRunFailed, ...})`

**Test:** `go test ./internal/controller/reconciler/ -run TestRunNotification`

**Commit:** `feat(reconciler): emit notifications on run completion`

### Task 7: Integrate into DiagnosticFixReconciler

- [ ] On fix applied: emit `EventFixApplied`
- [ ] On fix failed: emit `EventFixFailed`

**Files:** `internal/controller/reconciler/diagnosticfix_reconciler.go`

**Steps:**

- Add `notifier *notification.Manager` field
- On fix status change to Applied/Failed: call `r.notifier.Notify(...)`

**Test:** `go test ./internal/controller/reconciler/ -run TestFixNotification`

**Commit:** `feat(reconciler): emit notifications on fix status changes`

### Task 8: Wire into main.go + Helm values

- [ ] Create NotificationManager in main.go
- [ ] Read channel config from environment/flags
- [ ] Add Helm values for notification channels
- [ ] Pass manager to reconcilers

**Files:** `cmd/controller/main.go`, `deploy/helm/values.yaml`

**Steps:**

Helm values:
```yaml
notifications:
  dedupTTL: "5m"
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

**Test:** `helm template ./deploy/helm --set notifications.slack.enabled=true --set notifications.slack.webhookURL=https://hooks.slack.com/test`

**Commit:** `feat(main): wire notification manager with helm configuration`

### Task 9: Notify on fix approval via HTTP API

- [ ] In the fix approve/reject HTTP handler, emit notification events
- [ ] `EventFixApproved` on approval, `EventFixRejected` on rejection

**Files:** `internal/controller/httpserver/server.go`

**Steps:**

- After successful approve: `s.notifier.Notify(ctx, notification.Event{Type: EventFixApproved, ...})`
- After successful reject: `s.notifier.Notify(ctx, notification.Event{Type: EventFixRejected, ...})`

**Test:** `go test ./internal/controller/httpserver/ -run TestFixApproveNotification`

**Commit:** `feat(server): emit notifications on fix approval/rejection`

### Task 10: Documentation

- [ ] Create `docs/notifications.md`
- [ ] Cover: architecture, channel setup (webhook, Slack, DingTalk, Feishu)
- [ ] Include Helm values reference, event types table, HMAC verification example
- [ ] Troubleshooting section

**Files:** `docs/notifications.md`

**Steps:**

Sections:
1. Overview and architecture diagram
2. Supported channels with setup instructions
3. Event types table (type, when fired, severity)
4. Helm configuration reference
5. HMAC signature verification example (Python/Go)
6. Deduplication behavior
7. Troubleshooting

**Test:** Review documentation for accuracy and completeness.

**Commit:** `docs: add notification webhook setup guide`
