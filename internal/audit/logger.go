package audit

import (
	"log/slog"
	"os"
)

// New returns a JSON slog.Logger writing to stderr at the given level.
// level must be "info" or "debug"; anything else defaults to info.
func New(level string) *slog.Logger {
	lvl := slog.LevelInfo
	if level == "debug" {
		lvl = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
