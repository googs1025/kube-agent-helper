// Package notification 实现事件通知扇出与多通道集成。
//
// 架构角色：
//   - Manager 维护一组 Notifier（Webhook / Slack / DingTalk / Feishu），
//     接收来自 Reconciler 的 Event 后并行投递到所有匹配的通道。
//   - 通过 NotifyDispatcher 接口（在 reconciler 包定义）反向暴露给 Reconciler，
//     避免 reconciler ↔ notification 包形成循环依赖。
//
// 关键能力：
//   - 去重：按 (Type, Resource) 组合做 dedup，默认 5 分钟窗口。
//   - 事件过滤：每个通道可配置只接收某些 EventType，未配置 = 全收。
//   - 热重载：ReloadFromConfigs 可被 HTTP API 调用，DB 配置变更后立即生效。
//
// 配置来源（启动时按优先级）：
//  1. SQLite notification_configs 表（HTTP API 增删改）— 推荐
//  2. controller 启动 flag（--notify-slack-url 等）— 备选
package notification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// NotificationConfigProvider abstracts the store methods needed by Manager.
type NotificationConfigProvider interface {
	ListNotificationConfigs(ctx context.Context) ([]*NotificationConfig, error)
}

// NotificationConfig mirrors store.NotificationConfig to avoid import cycles.
type NotificationConfig struct {
	ID         string
	Name       string
	Type       string
	WebhookURL string
	Secret     string
	Events     string
	Enabled    bool
}

// Manager fans out events to registered channels with deduplication.
type Manager struct {
	mu           sync.RWMutex
	channels     []Notifier
	eventFilters map[int]map[EventType]bool // index in channels -> allowed events (nil = all)
	dedup        sync.Map
	dedupTTL     time.Duration
	logger       *slog.Logger
}

// NewManager creates a Manager with the given dedup TTL window.
func NewManager(logger *slog.Logger, dedupTTL time.Duration) *Manager {
	if dedupTTL <= 0 {
		dedupTTL = 5 * time.Minute
	}
	return &Manager{
		logger:   logger,
		dedupTTL: dedupTTL,
	}
}

// Register adds a notification channel to the manager (no event filter — receives all).
func (m *Manager) Register(n Notifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels = append(m.channels, n)
}

// Notify deduplicates the event by (Type, Resource) and fans out to all channels.
func (m *Manager) Notify(ctx context.Context, event Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	key := fmt.Sprintf("%s:%s", event.Type, event.Resource)
	if _, loaded := m.dedup.LoadOrStore(key, time.Now()); loaded {
		m.logger.Debug("notification deduplicated", "key", key)
		return nil
	}
	time.AfterFunc(m.dedupTTL, func() { m.dedup.Delete(key) })

	m.mu.RLock()
	channels := m.channels
	filters := m.eventFilters
	m.mu.RUnlock()

	var errs []error
	for i, ch := range channels {
		// Check event filter if present
		if filters != nil {
			if allowed, ok := filters[i]; ok && len(allowed) > 0 {
				if !allowed[event.Type] {
					continue
				}
			}
		}
		if err := ch.Send(ctx, event); err != nil {
			m.logger.Error("notification send failed", "channel", ch.Name(), "error", err)
			errs = append(errs, fmt.Errorf("%s: %w", ch.Name(), err))
		}
	}
	return errors.Join(errs...)
}

// ChannelCount returns the number of registered channels.
func (m *Manager) ChannelCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.channels)
}

// ReloadFromConfigs replaces internal channels with notifiers built from the
// given notification configs. CLI-flag channels that were registered before are
// replaced. Call this after any config change via the API.
func (m *Manager) ReloadFromConfigs(configs []*NotificationConfig) {
	var channels []Notifier
	filters := map[int]map[EventType]bool{}

	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		var n Notifier
		switch cfg.Type {
		case "webhook":
			n = NewWebhookChannel(cfg.WebhookURL, cfg.Secret)
		case "slack":
			n = NewSlackChannel(cfg.WebhookURL)
		case "dingtalk":
			n = NewDingTalkChannel(cfg.WebhookURL, cfg.Secret)
		case "feishu":
			n = NewFeishuChannel(cfg.WebhookURL, cfg.Secret)
		default:
			m.logger.Warn("unknown notification type", "type", cfg.Type, "name", cfg.Name)
			continue
		}
		idx := len(channels)
		channels = append(channels, n)

		// Parse event filter
		if cfg.Events != "" {
			allowed := map[EventType]bool{}
			for _, e := range strings.Split(cfg.Events, ",") {
				e = strings.TrimSpace(e)
				if e != "" {
					allowed[EventType(e)] = true
				}
			}
			if len(allowed) > 0 {
				filters[idx] = allowed
			}
		}
	}

	m.mu.Lock()
	m.channels = channels
	m.eventFilters = filters
	m.mu.Unlock()

	m.logger.Info("notification channels reloaded from DB", "count", len(channels))
}

// SendTest sends a test notification to a specific notifier built from the
// given config. It bypasses deduplication and event filtering.
func (m *Manager) SendTest(ctx context.Context, cfg *NotificationConfig) error {
	var n Notifier
	switch cfg.Type {
	case "webhook":
		n = NewWebhookChannel(cfg.WebhookURL, cfg.Secret)
	case "slack":
		n = NewSlackChannel(cfg.WebhookURL)
	case "dingtalk":
		n = NewDingTalkChannel(cfg.WebhookURL, cfg.Secret)
	case "feishu":
		n = NewFeishuChannel(cfg.WebhookURL, cfg.Secret)
	default:
		return fmt.Errorf("unknown notification type: %s", cfg.Type)
	}
	return n.Send(ctx, Event{
		Type:      "test",
		Severity:  "info",
		Title:     "Test Notification",
		Message:   fmt.Sprintf("This is a test notification from KubeDoctor (%s channel: %s)", cfg.Type, cfg.Name),
		Timestamp: time.Now(),
	})
}
