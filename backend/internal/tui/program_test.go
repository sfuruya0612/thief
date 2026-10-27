package tui

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestDiscardLogsRestoresLogger(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	restore := discardLogs()
	slog.Info("hidden from tui")
	slog.Warn("also hidden")
	if buf.Len() != 0 {
		t.Errorf("logs during TUI = %q, want none", buf.String())
	}

	restore()
	slog.Info("visible after tui")
	if !strings.Contains(buf.String(), "visible after tui") {
		t.Errorf("logs after restore = %q, want the message", buf.String())
	}
	if strings.Contains(buf.String(), "hidden from tui") {
		t.Errorf("logs during TUI leaked: %q", buf.String())
	}
}
