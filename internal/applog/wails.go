package applog

import (
	"context"
	"log/slog"
	"strings"
)

// WailsLogger routes the Wails runtime's own messages into the same log.
type WailsLogger struct{ L *slog.Logger }

func (w WailsLogger) log(level slog.Level, msg string) {
	w.L.Log(context.Background(), level, strings.TrimSpace(msg), "component", "wails")
}

func (w WailsLogger) Print(m string)   { w.log(slog.LevelInfo, m) }
func (w WailsLogger) Trace(m string)   { w.log(slog.LevelDebug, m) }
func (w WailsLogger) Debug(m string)   { w.log(slog.LevelDebug, m) }
func (w WailsLogger) Info(m string)    { w.log(slog.LevelInfo, m) }
func (w WailsLogger) Warning(m string) { w.log(slog.LevelWarn, m) }
func (w WailsLogger) Error(m string)   { w.log(slog.LevelError, m) }
func (w WailsLogger) Fatal(m string)   { w.log(slog.LevelError, "fatal: "+m) }
