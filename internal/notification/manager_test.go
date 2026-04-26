package notification

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeChannel records calls for testing.
type fakeChannel struct {
	name   string
	calls  atomic.Int32
	failOn int32 // fail when calls reaches this value (0 = never fail)
}

func (f *fakeChannel) Name() string { return f.name }
func (f *fakeChannel) Send(_ context.Context, _ Event) error {
	n := f.calls.Add(1)
	if f.failOn > 0 && n == f.failOn {
		return fmt.Errorf("fake error on call %d", n)
	}
	return nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestManagerNotify(t *testing.T) {
	m := NewManager(testLogger(), 5*time.Minute)
	ch := &fakeChannel{name: "test"}
	m.Register(ch)

	evt := Event{
		Type:     EventRunCompleted,
		Severity: "info",
		Title:    "Run done",
		Resource: "run-abc",
	}

	err := m.Notify(context.Background(), evt)
	require.NoError(t, err)
	assert.Equal(t, int32(1), ch.calls.Load())
}

func TestManagerDedup(t *testing.T) {
	m := NewManager(testLogger(), 5*time.Minute)
	ch := &fakeChannel{name: "test"}
	m.Register(ch)

	evt := Event{
		Type:     EventRunCompleted,
		Resource: "run-abc",
	}

	// First call goes through
	require.NoError(t, m.Notify(context.Background(), evt))
	assert.Equal(t, int32(1), ch.calls.Load())

	// Second call with same key is deduplicated
	require.NoError(t, m.Notify(context.Background(), evt))
	assert.Equal(t, int32(1), ch.calls.Load())

	// Different resource goes through
	evt2 := Event{
		Type:     EventRunCompleted,
		Resource: "run-xyz",
	}
	require.NoError(t, m.Notify(context.Background(), evt2))
	assert.Equal(t, int32(2), ch.calls.Load())
}

func TestManagerDedupExpiry(t *testing.T) {
	m := NewManager(testLogger(), 50*time.Millisecond)
	ch := &fakeChannel{name: "test"}
	m.Register(ch)

	evt := Event{Type: EventRunFailed, Resource: "run-1"}

	require.NoError(t, m.Notify(context.Background(), evt))
	assert.Equal(t, int32(1), ch.calls.Load())

	// Wait for dedup to expire
	time.Sleep(100 * time.Millisecond)

	require.NoError(t, m.Notify(context.Background(), evt))
	assert.Equal(t, int32(2), ch.calls.Load())
}

func TestManagerMultipleChannels(t *testing.T) {
	m := NewManager(testLogger(), 5*time.Minute)
	ch1 := &fakeChannel{name: "ch1"}
	ch2 := &fakeChannel{name: "ch2"}
	m.Register(ch1)
	m.Register(ch2)

	evt := Event{Type: EventFixApplied, Resource: "fix-1"}
	require.NoError(t, m.Notify(context.Background(), evt))

	assert.Equal(t, int32(1), ch1.calls.Load())
	assert.Equal(t, int32(1), ch2.calls.Load())
}

func TestManagerChannelError(t *testing.T) {
	m := NewManager(testLogger(), 5*time.Minute)
	ch1 := &fakeChannel{name: "good"}
	ch2 := &fakeChannel{name: "bad", failOn: 1}
	m.Register(ch1)
	m.Register(ch2)

	evt := Event{Type: EventFixFailed, Resource: "fix-2"}
	err := m.Notify(context.Background(), evt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad")

	// Both channels were still called
	assert.Equal(t, int32(1), ch1.calls.Load())
	assert.Equal(t, int32(1), ch2.calls.Load())
}

func TestManagerTimestampAutoFill(t *testing.T) {
	m := NewManager(testLogger(), 5*time.Minute)
	var captured Event
	ch := &capturingChannel{name: "cap", captured: &captured}
	m.Register(ch)

	evt := Event{Type: EventRunCompleted, Resource: "run-ts"}
	require.NoError(t, m.Notify(context.Background(), evt))
	assert.False(t, captured.Timestamp.IsZero())
}

type capturingChannel struct {
	name     string
	captured *Event
}

func (c *capturingChannel) Name() string { return c.name }
func (c *capturingChannel) Send(_ context.Context, e Event) error {
	*c.captured = e
	return nil
}
