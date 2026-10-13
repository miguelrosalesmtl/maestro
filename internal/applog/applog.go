// Package applog is Maestro's logging: structured JSON lines (log/slog) in a
// rotating file, crash reports from the Go runtime, and panic recovery.
//
// Files, in Dir():
//
//	maestro.log            current log (rotated at 10 MB, 5 backups, 30 days, gzip)
//	crash.log              written by the Go runtime if the process crashes
//	crash-<time>.log       a crash.log found at the next start, kept for reference
//
// Privacy: log issue keys, IDs, statuses,
// paths and errors, never summaries, descriptions, comments, terminal input or
// credentials. Attributes with sensitive names are redacted as a safety net.
package applog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	FileName      = "maestro.log"
	CrashFileName = "crash.log"
)

// Dir is the OS-conventional log directory.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Logs", "Maestro"), nil
	}
	if state := os.Getenv("XDG_STATE_HOME"); state != "" {
		return filepath.Join(state, "maestro", "logs"), nil
	}
	return filepath.Join(home, ".local", "state", "maestro", "logs"), nil
}

type Options struct {
	Dir     string     // default Dir()
	Level   slog.Level // default Info; MAESTRO_LOG_LEVEL overrides (debug|info|warn|error)
	Version string
	Stderr  bool // also write human-readable lines to stderr (development)
}

// Logs is the running logging setup.
type Logs struct {
	Logger *slog.Logger
	Dir    string
	File   string
	// PreviousCrash is the saved crash report of the last run, if it crashed.
	PreviousCrash string

	file  *lumberjack.Logger
	crash *os.File
	level *slog.LevelVar
}

// Setup creates the log directory, opens the rotating file, installs the
// crash file and sets the result as slog's default logger.
func Setup(o Options) (*Logs, error) {
	dir := o.Dir
	if dir == "" {
		var err error
		if dir, err = Dir(); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("log directory: %w", err)
	}
	l := &Logs{Dir: dir, File: filepath.Join(dir, FileName), level: new(slog.LevelVar)}
	l.level.Set(o.Level)
	if lv, ok := ParseLevel(os.Getenv("MAESTRO_LOG_LEVEL")); ok {
		l.level.Set(lv)
	}

	l.file = &lumberjack.Logger{Filename: l.File, MaxSize: 10, MaxBackups: 5, MaxAge: 30, Compress: true}
	opts := &slog.HandlerOptions{Level: l.level, ReplaceAttr: redact}
	var h slog.Handler = slog.NewJSONHandler(l.file, opts)
	if o.Stderr {
		h = fanout{h, slog.NewTextHandler(os.Stderr, opts)}
	}
	l.Logger = slog.New(h)
	slog.SetDefault(l.Logger)

	l.PreviousCrash = keepPreviousCrash(dir)
	if f, err := os.OpenFile(filepath.Join(dir, CrashFileName), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err == nil {
		if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err == nil {
			l.crash = f
		} else {
			f.Close()
		}
	}

	bi, _ := debug.ReadBuildInfo()
	goVersion := runtime.Version()
	if bi != nil {
		goVersion = bi.GoVersion
	}
	l.Logger.Info("maestro started",
		"version", o.Version, "os", runtime.GOOS, "arch", runtime.GOARCH, "go", goVersion, "pid", os.Getpid(), "level", l.level.Level().String())
	if l.PreviousCrash != "" {
		l.Logger.Error("previous run crashed", "report", l.PreviousCrash)
	}
	return l, nil
}

// SetLevel changes the level at runtime (e.g. after the config is loaded).
func (l *Logs) SetLevel(lv slog.Level) {
	if _, envSet := ParseLevel(os.Getenv("MAESTRO_LOG_LEVEL")); !envSet {
		l.level.Set(lv)
	}
}

// Close logs a clean shutdown and closes the files. An empty crash.log at the
// next start means the last run ended normally.
func (l *Logs) Close() error {
	l.Logger.Info("maestro stopped")
	if l.crash != nil {
		_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
		l.crash.Close()
		os.Remove(l.crash.Name())
	}
	return l.file.Close()
}

// keepPreviousCrash renames a non-empty crash.log to crash-<time>.log.
func keepPreviousCrash(dir string) string {
	p := filepath.Join(dir, CrashFileName)
	st, err := os.Stat(p)
	if err != nil || st.Size() == 0 {
		return ""
	}
	dst := filepath.Join(dir, "crash-"+st.ModTime().UTC().Format("20060102-150405")+".log")
	if os.Rename(p, dst) != nil {
		return ""
	}
	return dst
}

func ParseLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	}
	return slog.LevelInfo, false
}

// ── redaction ───────────────────────────────────────────────────

var sensitiveKey = regexp.MustCompile(`(?i)(token|password|passwd|secret|authorization|cookie|api[-_]?key|credential$)`)

func redact(groups []string, a slog.Attr) slog.Attr {
	if sensitiveKey.MatchString(a.Key) && a.Value.Kind() == slog.KindString && a.Value.String() != "" {
		return slog.String(a.Key, "[redacted]")
	}
	return a
}

// ── panics ──────────────────────────────────────────────────────

// Recover logs a panic with its stack and stops it. Use it at the top of
// goroutines that must not take the app down:
//
//	go func() { defer applog.Recover("terminal reader"); … }()
func Recover(where string) {
	if r := recover(); r != nil {
		slog.Error("panic recovered", "where", where, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
	}
}

// Go runs fn in a goroutine with Recover.
func Go(where string, fn func()) {
	go func() {
		defer Recover(where)
		fn()
	}()
}

// ── frontend errors ─────────────────────────────────────────────

// Frontend records errors reported by the UI, rate limited so a render loop
// cannot flood the file.
type Frontend struct {
	mu      sync.Mutex
	window  time.Time
	count   int
	dropped int
	now     func() time.Time
}

const (
	frontendPerMinute = 30
	frontendMaxLen    = 4000
)

func NewFrontend() *Frontend { return &Frontend{now: time.Now} }

// Log writes one frontend event. level: error | warn | info.
func (f *Frontend) Log(level, message, stack, source string) {
	f.mu.Lock()
	now := f.now()
	if now.Sub(f.window) >= time.Minute {
		if f.dropped > 0 {
			slog.Warn("frontend log rate limited", "dropped", f.dropped)
		}
		f.window, f.count, f.dropped = now, 0, 0
	}
	if f.count >= frontendPerMinute {
		f.dropped++
		f.mu.Unlock()
		return
	}
	f.count++
	f.mu.Unlock()

	lv := slog.LevelError
	if l, ok := ParseLevel(level); ok {
		lv = l
	}
	slog.Log(context.Background(), lv, "frontend: "+truncate(message, 500),
		"component", "frontend", "source", truncate(source, 300), "stack", truncate(stack, frontendMaxLen))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ── handler fan-out ─────────────────────────────────────────────

type fanout []slog.Handler

func (f fanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, r slog.Record) error {
	var first error
	for _, h := range f {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

func (f fanout) WithAttrs(as []slog.Attr) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithAttrs(as)
	}
	return out
}

func (f fanout) WithGroup(name string) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithGroup(name)
	}
	return out
}

var _ io.Closer = (*Logs)(nil)
