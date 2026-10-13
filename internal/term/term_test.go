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
		OnData:    func(_ string, b []byte) { mu.Lock(); out.Write(b); mu.Unlock() },
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
