package term

import (
	"strings"
	"sync"
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
