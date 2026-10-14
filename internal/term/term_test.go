package term

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestSessionRunsCommandsWithTicketEnv(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	var mu sync.Mutex
	var out strings.Builder
	var cmds []string
	m := NewManager(Hooks{
		OnData:    func(_ string, b []byte, _ int64) { mu.Lock(); out.Write(b); mu.Unlock() },
		OnCommand: func(_, line string) { mu.Lock(); cmds = append(cmds, line); mu.Unlock() },
	})
	defer m.CloseAll()

	if _, err := m.Open(Context{Key: "PLAT-1", Branch: "feature/PLAT-1-x", Dir: t.TempDir()}, 80, 24); err != nil {
		t.Fatal(err)
	}
	if err := m.Write("PLAT-1", "echo ticket=$MAESTRO_TICKET\r"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := strings.Contains(out.String(), "ticket=PLAT-1")
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(out.String(), "ticket=PLAT-1") {
		t.Fatalf("env not visible in shell, output: %q", out.String())
	}
	if len(cmds) != 1 || cmds[0] != "echo ticket=$MAESTRO_TICKET" {
		t.Fatalf("commands = %q", cmds)
	}
}

func TestFreestyleSessionHasNoTicket(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("MAESTRO_TICKET", "")
	var mu sync.Mutex
	var out strings.Builder
	var cmds []string
	m := NewManager(Hooks{
		OnData:    func(_ string, b []byte, _ int64) { mu.Lock(); out.Write(b); mu.Unlock() },
		OnCommand: func(_, line string) { mu.Lock(); cmds = append(cmds, line); mu.Unlock() },
	})
	defer m.CloseAll()

	if _, err := m.Open(Context{Session: "free:1", Dir: t.TempDir()}, 80, 24); err != nil {
		t.Fatal(err)
	}
	// The quotes split the marker, so the echoed command line cannot match it.
	if err := m.Write("free:1", "echo ticket\"\"=[${MAESTRO_TICKET-unset}]\r"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := strings.Contains(out.String(), "ticket=[") // output only
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(out.String(), "ticket=[]") && !strings.Contains(out.String(), "ticket=[unset]") {
		t.Fatalf("freestyle shell must not get a ticket, output: %q", out.String())
	}
	if len(cmds) != 0 {
		t.Fatalf("freestyle commands must not reach the flow: %q", cmds)
	}
}

func TestStartupCommandsAndEnv(t *testing.T) {
	t.Setenv("SHELL", "/bin/false") // Context.Shell wins over $SHELL
	var mu sync.Mutex
	var out strings.Builder
	var cmds []string
	m := NewManager(Hooks{
		OnData:    func(_ string, b []byte, _ int64) { mu.Lock(); out.Write(b); mu.Unlock() },
		OnCommand: func(_, line string) { mu.Lock(); cmds = append(cmds, line); mu.Unlock() },
	})
	defer m.CloseAll()

	c := Context{Key: "PLAT-1", Dir: t.TempDir(), Shell: "/bin/sh",
		Env:     []string{"GREETING=hi", "MAESTRO_TICKET=not-this"},
		Startup: []string{`echo start""=$GREETING/$MAESTRO_TICKET`}}
	if _, err := m.Open(c, 80, 24); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := strings.Contains(out.String(), "start=")
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(out.String(), "start=hi/PLAT-1") {
		t.Fatalf("startup command should see config env and the real ticket, output: %q", out.String())
	}
	if len(cmds) != 0 {
		t.Fatalf("startup commands are not the user's and must not drive the flow: %q", cmds)
	}
}

func TestHistoryRefillsAReopenedView(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLAUDE_CODE_CHILD_SESSION", "1")
	var mu sync.Mutex
	var out strings.Builder
	var lastEnd int64
	m := NewManager(Hooks{OnData: func(_ string, b []byte, end int64) {
		mu.Lock()
		out.Write(b)
		lastEnd = end
		mu.Unlock()
	}})
	defer m.CloseAll()
	if _, err := m.Open(Context{Session: "free:1", Dir: t.TempDir()}, 80, 24); err != nil {
		t.Fatal(err)
	}
	// The quotes split the marker, so the echoed command line cannot match it.
	if err := m.Write("free:1", "echo agent\"\"=[${CLAUDECODE-}${CLAUDE_CODE_CHILD_SESSION-}]\r"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := strings.Contains(out.String(), "agent=[")
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)

	data, end, ok := m.History("free:1")
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(out.String(), "agent=[]") {
		t.Fatalf("a terminal must not inherit the launching agent's session markers, output: %q", out.String())
	}
	if !ok || string(data) != out.String() || end != lastEnd || end != int64(len(data)) {
		t.Fatalf("history = %q (end %d, ok %v), streamed %q (end %d)", data, end, ok, out.String(), lastEnd)
	}
	if _, _, ok := m.History("free:9"); ok {
		t.Fatal("unknown session must report !ok")
	}
}

func TestHistoryKeepsTheLatestOutput(t *testing.T) {
	s := &session{}
	line := strings.Repeat("x", 99) + "\n"
	var end int64
	for range 2*HistoryLimit/100 + 50 {
		end = s.record([]byte(line))
	}
	if len(s.hist) > 2*HistoryLimit || len(s.hist) < HistoryLimit || len(s.hist)%100 != 0 || end != int64((2*HistoryLimit/100+50)*100) {
		t.Fatalf("kept %d bytes (must be whole lines, at most %d), end %d", len(s.hist), HistoryLimit, end)
	}
}

// Closing a terminal ends what was started in it, also processes that
// ignore the hangup or run in the background. Before, only the shell was
// killed and agents / dev servers lived on under launchd.
func TestCloseEndsEverythingStartedInTheTerminal(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	old := KillGrace
	KillGrace = 300 * time.Millisecond
	defer func() { KillGrace = old }()
	pidFile := filepath.Join(t.TempDir(), "pids")
	m := NewManager(Hooks{})
	c := Context{Key: "PLAT-1", Dir: t.TempDir(),
		// A background job, and a foreground child that ignores HUP and TERM.
		Startup: []string{`sleep 300 & echo $! > ` + pidFile + `; sh -c 'trap "" HUP TERM; echo $$ >> ` + pidFile + `; while :; do sleep 1; done'`}}
	if _, err := m.Open(c, 80, 24); err != nil {
		t.Fatal(err)
	}
	var pids []int
	deadline := time.Now().Add(3 * time.Second)
	for len(pids) < 2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		raw, _ := os.ReadFile(pidFile)
		pids = pids[:0]
		for _, f := range strings.Fields(string(raw)) {
			if p, err := strconv.Atoi(f); err == nil {
				pids = append(pids, p)
			}
		}
	}
	if len(pids) < 2 {
		t.Fatalf("children never started: %v", pids)
	}
	m.CloseAll()
	for _, p := range pids {
		if syscall.Kill(p, 0) == nil && !zombie(p) {
			_ = syscall.Kill(p, syscall.SIGKILL)
			t.Fatalf("process %d outlived its terminal", p)
		}
	}
}

// Each terminal reports the memory of everything started in it.
func TestUsageCountsTheWholeTerminal(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	old := UsageEvery
	UsageEvery = 100 * time.Millisecond
	defer func() { UsageEvery = old }()
	var mu sync.Mutex
	var last []Usage
	m := NewManager(Hooks{OnUsage: func(u []Usage) { mu.Lock(); last = u; mu.Unlock() }})
	defer m.CloseAll()
	m.StartMonitor(DefaultNotifyOptions())
	c := Context{Key: "PLAT-1", Dir: t.TempDir(), Startup: []string{"sleep 300 & sleep 300 & sleep 300"}}
	if _, err := m.Open(c, 80, 24); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		u := slices.Clone(last)
		mu.Unlock()
		if len(u) == 1 && u[0].Procs >= 4 {
			if u[0].Session != "PLAT-1" || u[0].Ticket != "PLAT-1" || u[0].Memory <= 0 {
				t.Fatalf("usage = %+v", u[0])
			}
			for _, g := range u[0].Top {
				if g.Command == "sleep" && g.Count == 3 && g.Memory > 0 {
					return
				}
			}
			t.Fatalf("sleep processes not grouped: %+v", u[0].Top)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no usage with the shell and 3 sleeps: %+v", last)
}
