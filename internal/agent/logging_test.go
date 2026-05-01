package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// captureStdout redirects os.Stdout into a pipe, runs fn, restores Stdout, and
// returns whatever fn wrote. Used because EmitEvent writes directly to stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	os.Stdout = origStdout
	return <-done
}

func TestEmitEvent_HappyPathWithData(t *testing.T) {
	type payload struct {
		Step   int    `json:"step"`
		Reason string `json:"reason"`
	}
	out := captureStdout(t, func() {
		EmitEvent("run-123", LogTypeStep, "checking pod", payload{Step: 2, Reason: "pending"})
	})

	var got LogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatalf("unmarshal emitted line: %v\nraw=%q", err, out)
	}

	if got.RunID != "run-123" {
		t.Errorf("RunID = %q, want run-123", got.RunID)
	}
	if got.Type != LogTypeStep {
		t.Errorf("Type = %q, want %q", got.Type, LogTypeStep)
	}
	if got.Message != "checking pod" {
		t.Errorf("Message = %q, want %q", got.Message, "checking pod")
	}
	if got.Data == nil {
		t.Errorf("Data = nil, want non-nil")
	}
	if _, err := time.Parse(time.RFC3339Nano, got.Timestamp); err != nil {
		t.Errorf("Timestamp %q does not parse as RFC3339Nano: %v", got.Timestamp, err)
	}
}

func TestEmitEvent_NilDataOmitsField(t *testing.T) {
	out := captureStdout(t, func() {
		EmitEvent("run-7", LogTypeInfo, "starting", nil)
	})

	// `data,omitempty` means nil should not produce the key at all.
	if strings.Contains(out, `"data"`) {
		t.Errorf("expected no \"data\" field for nil data, got: %s", out)
	}

	var got LogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw=%q", err, out)
	}
	if got.Data != nil {
		t.Errorf("Data = %v, want nil", got.Data)
	}
}

func TestEmitEvent_AllLogTypes(t *testing.T) {
	cases := []struct {
		name    string
		logType string
	}{
		{"step", LogTypeStep},
		{"finding", LogTypeFinding},
		{"fix", LogTypeFix},
		{"error", LogTypeError},
		{"info", LogTypeInfo},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				EmitEvent("r1", tc.logType, "msg", nil)
			})
			var got LogEntry
			if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
				t.Fatalf("unmarshal: %v\nraw=%q", err, out)
			}
			if got.Type != tc.logType {
				t.Errorf("Type = %q, want %q", got.Type, tc.logType)
			}
		})
	}
}

func TestLogTypeConstantsAreStable(t *testing.T) {
	// These constant values are read by the controller-side log tailer
	// and must not silently drift. Lock them down.
	cases := map[string]string{
		"step":    LogTypeStep,
		"finding": LogTypeFinding,
		"fix":     LogTypeFix,
		"error":   LogTypeError,
		"info":    LogTypeInfo,
	}
	for want, got := range cases {
		if got != want {
			t.Errorf("LogType for %q = %q, want %q", want, got, want)
		}
	}
}

func TestEmitEvent_TimestampMonotonic(t *testing.T) {
	// Two consecutive calls should produce non-decreasing timestamps.
	out := captureStdout(t, func() {
		EmitEvent("r", LogTypeInfo, "first", nil)
		EmitEvent("r", LogTypeInfo, "second", nil)
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), out)
	}
	var a, b LogEntry
	if err := json.Unmarshal([]byte(lines[0]), &a); err != nil {
		t.Fatalf("line0 unmarshal: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &b); err != nil {
		t.Fatalf("line1 unmarshal: %v", err)
	}
	ta, _ := time.Parse(time.RFC3339Nano, a.Timestamp)
	tb, _ := time.Parse(time.RFC3339Nano, b.Timestamp)
	if tb.Before(ta) {
		t.Errorf("second timestamp %v is before first %v", tb, ta)
	}
}
