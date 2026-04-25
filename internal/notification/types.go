package notification

import (
	"context"
	"time"
)

// EventType identifies the kind of notification event.
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

// Event is the payload sent to notification channels.
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

// Notifier is the interface implemented by each notification channel.
type Notifier interface {
	// Name returns a human-readable channel name (e.g. "webhook", "slack").
	Name() string
	// Send delivers an event to the channel.
	Send(ctx context.Context, event Event) error
}
