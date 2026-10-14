// Package term runs real shell sessions (one per ticket) behind a PTY and
// watches the commands typed into them so the ticket can react.
package term

import (
	"bytes"
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

	// From config terminal: the shell ("" = $SHELL), extra environment
	// ("NAME=value", before the MAESTRO_ variables so those win) and commands
	// typed into the shell once it starts.
	Shell   string   `json:"-"`
	Env     []string `json:"-"`
	Startup []string `json:"-"`
}

// Hooks lets the app react to session output and to commands the user runs.
type Hooks struct {
	// OnData gets each chunk of output; end is the session's output length
	// after it, so a reattached view can line it up with History.
	OnData    func(key string, data []byte, end int64)
	OnExit    func(key string)
	OnCommand func(key, line string)
	// OnActivity reports running / waiting / idle changes (see activity.go).
	OnActivity func(Activity)
	// OnUsage reports the memory and CPU of every open terminal (usage.go),
	// every UsageEvery while the monitor runs.
	OnUsage func([]Usage)
}

type session struct {
	cmd    *exec.Cmd
	pty    *os.File
	ticket string
	line   []byte // current input line, for command detection

	lastOutput atomic.Int64 // unix nanos of the last PTY output

	histMu sync.Mutex
	hist   []byte   // the latest output, so a reopened view is not blank
	total  int64    // bytes of output so far
	act    actState // owned by the monitor goroutine
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
	sh := c.Shell
	if sh == "" {
		sh = shell()
	}
	cmd := exec.Command(sh, "-l")
	cmd.Dir = dir
	cmd.Env = append(inheritedEnv(), "TERM=xterm-256color", "COLORTERM=truecolor")
	cmd.Env = append(cmd.Env, c.Env...)
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
	slog.Info("terminal started", "component", "term", "session", c.Session, "key", c.Key, "shell", filepath.Base(sh), "startup", len(c.Startup), "pid", cmd.Process.Pid)

	if len(c.Startup) > 0 {
		// Typed like the user would: the shell runs them after its own startup files.
		go func() {
			defer applog.Recover("terminal startup " + c.Session)
			for _, line := range c.Startup {
				if _, err := f.Write([]byte(line + "\r")); err != nil {
					return
				}
			}
		}()
	}

	// Output is batched for a few milliseconds before it goes to the UI:
	// programs that redraw their screen (AI agents, progress bars) write in
	// many tiny pieces, and one message per piece costs CPU on both sides.
	var (
		pendMu  sync.Mutex
		pending []byte
	)
	wake := make(chan struct{}, 1)
	emitted := make(chan struct{})
	go func() {
		defer close(emitted)
		defer applog.Recover("terminal output " + c.Session)
		flush := func() {
			pendMu.Lock()
			chunk := pending
			pending = nil
			pendMu.Unlock()
			if len(chunk) == 0 {
				return
			}
			end := s.record(chunk)
			if m.hooks.OnData != nil {
				m.hooks.OnData(c.Session, chunk, end)
			}
		}
		for range wake {
			pendMu.Lock()
			small := len(pending) < 64<<10
			pendMu.Unlock()
			if small {
				time.Sleep(OutputBatch)
			}
			flush()
		}
		flush()
	}()

	go func() {
		defer applog.Recover("terminal " + c.Session)
		buf := make([]byte, 32*1024)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				s.lastOutput.Store(time.Now().UnixNano())
				pendMu.Lock()
				pending = append(pending, buf[:n]...)
				pendMu.Unlock()
				select {
				case wake <- struct{}{}:
				default: // a flush is already due
				}
			}
			if err != nil {
				break
			}
		}
		close(wake)
		<-emitted // all output reaches the UI before the exit does
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

// OutputBatch is how long output is collected before it is sent to the UI.
const OutputBatch = 4 * time.Millisecond

// HistoryLimit is how much recent output each session keeps for History
// (between one and two times this much, so trimming is rare and allocates nothing).
const HistoryLimit = 512 << 10

// record keeps chunk in the session's history and returns the output length after it.
func (s *session) record(chunk []byte) int64 {
	s.histMu.Lock()
	defer s.histMu.Unlock()
	if s.hist == nil {
		s.hist = make([]byte, 0, 2*HistoryLimit+32<<10)
	}
	s.hist = append(s.hist, chunk...)
	// Trim once twice the limit is reached, in place: a busy program (an AI
	// agent's screen redraws) writes many small chunks, and copying the whole
	// history for each of them would burn CPU and memory.
	if len(s.hist) > 2*HistoryLimit {
		cut := len(s.hist) - HistoryLimit
		// Start on a line, not in the middle of an escape sequence.
		if i := bytes.IndexByte(s.hist[cut:], '\n'); i >= 0 && i < 4096 {
			cut += i + 1
		}
		s.hist = s.hist[:copy(s.hist, s.hist[cut:])]
	}
	s.total += int64(len(chunk))
	return s.total
}

// History is the session's recent output and the output length it ends at
// (ok is false for an unknown session).
func (m *Manager) History(session string) (data []byte, end int64, ok bool) {
	m.mu.Lock()
	s := m.sessions[session]
	m.mu.Unlock()
	if s == nil {
		return nil, 0, false
	}
	s.histMu.Lock()
	defer s.histMu.Unlock()
	return append([]byte(nil), s.hist...), s.total, true
}

// inheritedEnv is Maestro's environment minus the markers of an AI agent
// session that launched it (e.g. Maestro started from Claude Code): a shell
// here is the user's, and tools in it must not think they run inside that session.
func inheritedEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name == "CLAUDECODE" || name == "AI_AGENT" || name == "CLAUDE_PID" || name == "CLAUDE_EFFORT" || strings.HasPrefix(name, "CLAUDE_CODE_") {
			continue
		}
		out = append(out, kv)
	}
	return out
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

// Close ends a terminal and everything started in it (an agent, a dev
// server, their helpers), not only the shell.
func (m *Manager) Close(key string) {
	if pids := m.detach(key); pids != nil {
		go reap(pids, KillGrace)
	}
}

// detach removes a session and closes its PTY. It returns the processes to
// end, listed while the shell still holds them.
func (m *Manager) detach(key string) []int {
	m.mu.Lock()
	s, ok := m.sessions[key]
	delete(m.sessions, key)
	m.mu.Unlock()
	if !ok {
		return nil
	}
	var pids []int
	if s.cmd.Process != nil {
		pids = tree(s.cmd.Process.Pid)
	}
	_ = s.pty.Close()
	return pids
}

// CloseAll ends every terminal (Maestro is quitting) and waits for them.
func (m *Manager) CloseAll() {
	m.StopMonitor()
	m.mu.Lock()
	keys := make([]string, 0, len(m.sessions))
	for k := range m.sessions {
		keys = append(keys, k)
	}
	m.mu.Unlock()
	var groups [][]int
	for _, k := range keys {
		if pids := m.detach(k); pids != nil {
			groups = append(groups, pids)
		}
	}
	reapAll(groups, KillGrace)
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
