package term

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Terminal activity: is a command running in a ticket's terminal, is it
// waiting for input, did a long one just finish?
//
// Detection uses two signals per session, checked twice a second:
//   - the PTY's foreground process group (tcgetpgrp): if it is not the shell
//     itself, a command is running (standard Unix job control, no shell hooks);
//   - the time of the last output: a running command that has been silent for
//     a while is probably waiting for input (a prompt, an AI agent's question).

type ActivityState string

const (
	Idle    ActivityState = "idle"    // at the shell prompt
	Running ActivityState = "running" // a command is running and printing
	Waiting ActivityState = "waiting" // a command is running but silent
)

// Activity is one state change of one terminal session.
type Activity struct {
	Session  string        `json:"session"`
	Ticket   string        `json:"ticket"`
	State    ActivityState `json:"state"`
	Command  string        `json:"command,omitempty"`  // foreground program, e.g. "npm", "claude"
	Since    string        `json:"since,omitempty"`    // when the command started (RFC 3339)
	Finished bool          `json:"finished,omitempty"` // a command ended (State is Idle)
	Seconds  int           `json:"seconds,omitempty"`  // how long it ran, when Finished
	// Notify asks the UI to chirp: a long command finished, or a command
	// started waiting for input. Interactive programs (Ignore) never notify.
	Notify bool `json:"notify,omitempty"`
}

type NotifyOptions struct {
	MinRun  time.Duration // only notify "finished" for commands that ran at least this long
	WaitFor time.Duration // silence after which a running command counts as waiting
	Ignore  []string      // interactive programs: never "waiting", never notify (vim, less, ssh…)
	Every   time.Duration // poll interval
}

func DefaultNotifyOptions() NotifyOptions {
	return NotifyOptions{
		MinRun:  10 * time.Second,
		WaitFor: 8 * time.Second,
		Every:   500 * time.Millisecond,
		Ignore: []string{"vim", "nvim", "vi", "nano", "emacs", "less", "more", "man", "ssh", "mosh", "top", "htop", "btop",
			"tmux", "screen", "watch", "tail", "k9s", "lazygit", "lazydocker", "fzf", "psql", "mysql", "sqlite3", "redis-cli"},
	}
}

type actState struct {
	state   ActivityState
	pgid    int
	command string
	since   time.Time
	quiet   bool // already reported Waiting for this silence
}

// StartMonitor begins activity detection. Safe to call once.
func (m *Manager) StartMonitor(o NotifyOptions) {
	if o.Every <= 0 {
		o.Every = 500 * time.Millisecond
	}
	m.mu.Lock()
	if m.monitorStop != nil {
		m.mu.Unlock()
		return
	}
	m.notify = o
	m.monitorStop = make(chan struct{})
	stop := m.monitorStop
	m.mu.Unlock()

	go func() {
		t := time.NewTicker(o.Every)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				m.checkActivity()
			}
		}
	}()
	go func() {
		t := time.NewTicker(UsageEvery)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				m.checkUsage()
			}
		}
	}()
}

func (m *Manager) StopMonitor() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.monitorStop != nil {
		close(m.monitorStop)
		m.monitorStop = nil
	}
}

func (m *Manager) checkActivity() {
	m.mu.Lock()
	type item struct {
		id string
		s  *session
	}
	items := make([]item, 0, len(m.sessions))
	for id, s := range m.sessions {
		items = append(items, item{id, s})
	}
	o := m.notify
	m.mu.Unlock()

	now := time.Now()
	for _, it := range items {
		if ev, changed := it.s.step(it.id, now, o); changed && m.hooks.OnActivity != nil {
			m.hooks.OnActivity(ev)
		}
	}
}

// step computes the session's state; only the monitor goroutine calls it.
func (s *session) step(id string, now time.Time, o NotifyOptions) (Activity, bool) {
	a := &s.act
	shellPid := 0
	if s.cmd.Process != nil {
		shellPid = s.cmd.Process.Pid
	}
	fg := foregroundPgid(s.pty)
	busy := fg > 0 && fg != shellPid

	ev := Activity{Session: id, Ticket: s.ticket}
	switch {
	case !busy && a.state != Idle && a.state != "":
		ran := now.Sub(a.since)
		ignored := slices.Contains(o.Ignore, a.command)
		ev.State, ev.Finished, ev.Seconds = Idle, true, int(ran.Round(time.Second).Seconds())
		ev.Command = a.command
		ev.Notify = !ignored && ran >= o.MinRun
		*a = actState{state: Idle}
		return ev, true
	case !busy:
		if a.state == "" {
			a.state = Idle
		}
		return ev, false
	}

	if fg != a.pgid { // a new foreground job
		a.pgid, a.command, a.since, a.quiet = fg, procName(fg), now, false
		a.state = Running
		ev.State, ev.Command, ev.Since = Running, a.command, a.since.UTC().Format(time.RFC3339)
		return ev, true
	}
	silent := now.Sub(time.Unix(0, s.lastOutput.Load()))
	ignored := slices.Contains(o.Ignore, a.command)
	switch {
	case !ignored && silent >= o.WaitFor && a.state == Running && !a.quiet:
		a.state, a.quiet = Waiting, true
		ev.State, ev.Command, ev.Since, ev.Notify = Waiting, a.command, a.since.UTC().Format(time.RFC3339), true
		return ev, true
	case silent < o.WaitFor && a.state == Waiting:
		a.state, a.quiet = Running, false
		ev.State, ev.Command, ev.Since = Running, a.command, a.since.UTC().Format(time.RFC3339)
		return ev, true
	}
	return ev, false
}

// foregroundPgid reads the PTY's foreground process group without switching
// the file to blocking mode (which Fd() would do).
func foregroundPgid(f *os.File) int {
	rc, err := f.SyscallConn()
	if err != nil {
		return 0
	}
	pgid := 0
	_ = rc.Control(func(fd uintptr) {
		if v, err := unix.IoctlGetInt(int(fd), unix.TIOCGPGRP); err == nil {
			pgid = v
		}
	})
	return pgid
}

// procName is the program name of a process (the group leader).
func procName(pid int) string {
	if runtime.GOOS == "linux" {
		if b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "comm")); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return filepath.Base(strings.TrimSpace(string(out)))
}
