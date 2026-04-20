package reconciler

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func TestParseCronNext(t *testing.T) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

	tests := []struct {
		expr    string
		wantErr bool
	}{
		{"0 * * * *", false},
		{"*/15 * * * *", false},
		{"0 0 * * *", false},
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		_, err := parser.Parse(tt.expr)
		if (err != nil) != tt.wantErr {
			t.Errorf("Parse(%q) error=%v, wantErr=%v", tt.expr, err, tt.wantErr)
		}
	}
}

func TestNextRunAfterNow(t *testing.T) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, _ := parser.Parse("0 * * * *")

	now := time.Now().Truncate(time.Hour).Add(time.Hour)
	next := sched.Next(now.Add(-2 * time.Hour))

	if !next.Before(now.Add(time.Second)) {
		t.Errorf("expected next run before now, got %v (now=%v)", next, now)
	}
}

func TestChildRunName(t *testing.T) {
	parent := "my-diagnostic-run"
	ts := time.Unix(1745123456, 0)
	name := childRunName(parent, ts)
	if name == "" {
		t.Fatal("childRunName returned empty string")
	}
	if len(name) > 253 {
		t.Errorf("childRunName too long: %d chars", len(name))
	}
}
