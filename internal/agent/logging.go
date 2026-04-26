package agent

import (
	"encoding/json"
	"os"
	"time"
)

// LogEntry represents a structured log line emitted by the agent runtime.
type LogEntry struct {
	Timestamp string      `json:"timestamp"`
	RunID     string      `json:"run_id"`
	Type      string      `json:"type"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
}

// Log types emitted by the agent.
const (
	LogTypeStep    = "step"
	LogTypeFinding = "finding"
	LogTypeFix     = "fix"
	LogTypeError   = "error"
	LogTypeInfo    = "info"
)

// EmitEvent writes a structured JSON log entry to stdout.
// Agent pods produce these lines; the controller tails and persists them.
func EmitEvent(runID, logType, message string, data interface{}) {
	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		RunID:     runID,
		Type:      logType,
		Message:   message,
		Data:      data,
	}
	_ = json.NewEncoder(os.Stdout).Encode(entry)
}
