// Package term runs real shell sessions (one per ticket) behind a PTY and
// watches the commands typed into them so the ticket can react.
package term

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creack/pty"

	"maestro/internal/applog"
)

// Context is the ticket a session is bound to; exported to the shell as env vars.
type Context struct {
	Key string `json:"key"` // the ticket
	// Session identifies the shell; several sessions can share one ticket
	// ("PLAT-142", "PLAT-142#2"). Empty means Key.
	Session string `json:"session"`
	Summary string `json:"summary"`
	Branch  string `json:"branch"`
	Dir     string `json:"dir"`
}

// Hooks lets the app react to session output and to commands the user runs.
type Hooks struct {
	OnData    func(key string, data []byte)
	OnExit    func(key string)
	OnCommand func(key, line string)
	// OnActivity reports running / waiting / idle changes (see activity.go).
	OnActivity func(Activity)
}

type session struct {
	cmd    *exec.Cmd
	pty    *os.File
	ticket string
	line   []byte // current input line, for command detection

	lastOutput atomic.Int64 // unix nanos of the last PTY output
	act        actState     // owned by the monitor goroutine
}

type Manager struct {
	mu       sync.Mutex
	sessions map[string]*session
	hooks    Hooks

	notify      NotifyOptions
	monitorStop chan struct{}
}

func NewManager(h Hooks) *Manager {
	return &Manager{sessions: map[string]*session{}, hooks: h}
}

func shell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/zsh"
}

// Open starts (or reattaches to) the shell for a ticket. It returns true when
// a new session was created.
// SessionTicket returns the ticket key of a session ID ("PLAT-142#2" → "PLAT-142").
// FreestylePrefix marks terminals that belong to no ticket ("free:1"):
// no ticket env vars, and their commands never drive a ticket's flow.
const FreestylePrefix = "free:"

func IsFreestyle(session string) bool { return strings.HasPrefix(session, FreestylePrefix) }

func SessionTicket(session string) string {
	if i := strings.IndexByte(session, '#'); i > 0 {
		return session[:i]
	}
	return session
}

func (m *Manager) Open(c Context, cols, rows uint16) (bool, error) {
	if c.Session == "" {
		c.Session = c.Key
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[c.Session]; ok {
		return false, nil
	}
	dir := c.Dir
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	cmd := exec.Command(shell(), "-l")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	if c.Key != "" {
		cmd.Env = append(cmd.Env, "MAESTRO_TICKET="+c.Key, "MAESTRO_SUMMARY="+c.Summary, "MAESTRO_BRANCH="+c.Branch)
	}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return false, fmt.Errorf("start shell: %w", err)
	}
	s := &session{cmd: cmd, pty: f, ticket: c.Key}
	s.lastOutput.Store(time.Now().UnixNano())
	m.sessions[c.Session] = s
	// Log the session, never what is typed in it (it can contain secrets).
	slog.Info("terminal started", "component", "term", "session", c.Session, "key", c.Key, "shell", filepath.Base(shell()), "pid", cmd.Process.Pid)

	go func() {
		defer applog.Recover("terminal " + c.Session)
		buf := make([]byte, 32*1024)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				s.lastOutput.Store(time.Now().UnixNano())
			}
			if n > 0 && m.hooks.OnData != nil {
				m.hooks.OnData(c.Session, append([]byte(nil), buf[:n]...))
			}
			if err != nil {
				break
			}
		}
		werr := cmd.Wait()
		code := 0
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		slog.Info("terminal exited", "component", "term", "session", c.Session, "code", code, "err", errText(werr))
		m.mu.Lock()
		if m.sessions[c.Session] == s {
			delete(m.sessions, c.Session)
		}
		m.mu.Unlock()
		if m.hooks.OnExit != nil {
			m.hooks.OnExit(c.Session)
		}
	}()
	return true, nil
}

// Write sends keystrokes to the session and tracks the line being typed.
func (m *Manager) Write(key, data string) error {
	m.mu.Lock()
	s, ok := m.sessions[key]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("no terminal for %s", key)
	}
	var done []string
	for _, b := range []byte(data) {
		switch {
		case b == '\r' || b == '\n':
			if l := strings.TrimSpace(string(s.line)); l != "" {
				done = append(done, l)
			}
			s.line = s.line[:0]
		case b == 0x7f || b == 0x08: // backspace
			if len(s.line) > 0 {
				s.line = s.line[:len(s.line)-1]
			}
		case b == 0x03 || b == 0x15: // ctrl-c, ctrl-u
			s.line = s.line[:0]
		case b >= 0x20 && b < 0x7f:
			s.line = append(s.line, b)
		}
	}
	if strings.HasPrefix(data, "\x1b") { // arrow keys / history: line is no longer reliable
		s.line = s.line[:0]
	}
	_, err := s.pty.WriteString(data)
	if m.hooks.OnCommand != nil && !IsFreestyle(key) {
		for _, l := range done {
			m.hooks.OnCommand(key, l)
		}
	}
	return err
}

func (m *Manager) Resize(key string, cols, rows uint16) {
	m.mu.Lock()
	s, ok := m.sessions[key]
	m.mu.Unlock()
	if ok {
		_ = pty.Setsize(s.pty, &pty.Winsize{Cols: cols, Rows: rows})
	}
}

func (m *Manager) Close(key string) {
	m.mu.Lock()
	s, ok := m.sessions[key]
	delete(m.sessions, key)
	m.mu.Unlock()
	if ok {
		_ = s.pty.Close()
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
	}
}

func (m *Manager) CloseAll() {
	m.StopMonitor()
	m.mu.Lock()
	keys := make([]string, 0, len(m.sessions))
	for k := range m.sessions {
		keys = append(keys, k)
	}
	m.mu.Unlock()
	for _, k := range keys {
		m.Close(k)
	}
}

// Open sessions, for the UI tab strip.
func (m *Manager) Keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.sessions))
	for k := range m.sessions {
		out = append(out, k)
	}
	return out
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
