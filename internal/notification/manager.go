package notification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Manager fans out events to registered channels with deduplication.
type Manager struct {
	channels []Notifier
	dedup    sync.Map
	dedupTTL time.Duration
	logger   *slog.Logger
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

// Register adds a notification channel to the manager.
func (m *Manager) Register(n Notifier) {
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

	var errs []error
	for _, ch := range m.channels {
		if err := ch.Send(ctx, event); err != nil {
			m.logger.Error("notification send failed", "channel", ch.Name(), "error", err)
			errs = append(errs, fmt.Errorf("%s: %w", ch.Name(), err))
		}
	}
	return errors.Join(errs...)
}

// ChannelCount returns the number of registered channels.
func (m *Manager) ChannelCount() int {
	return len(m.channels)
}
