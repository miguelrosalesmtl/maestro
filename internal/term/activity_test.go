package term

import (
	"sync"
	"testing"
	"time"
)

func TestActivityRunningWaitingFinished(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	var mu sync.Mutex
	var events []Activity
	m := NewManager(Hooks{OnActivity: func(a Activity) { mu.Lock(); events = append(events, a); mu.Unlock() }})
	defer m.CloseAll()
	m.StartMonitor(NotifyOptions{MinRun: 300 * time.Millisecond, WaitFor: 400 * time.Millisecond, Every: 50 * time.Millisecond,
		Ignore: []string{"cat"}})

	if _, err := m.Open(Context{Key: "PLAT-1", Session: "PLAT-1#2", Dir: t.TempDir()}, 80, 24); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // shell starts, prints its prompt
	if err := m.Write("PLAT-1#2", "sleep 1\r"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, &mu, &events, func(es []Activity) bool { return len(es) > 0 && es[len(es)-1].Finished })

	mu.Lock()
	got := append([]Activity(nil), events...)
	mu.Unlock()
	states := []ActivityState{}
	for _, e := range got {
		if e.Session != "PLAT-1#2" || e.Ticket != "PLAT-1" {
			t.Errorf("event for wrong session: %+v", e)
		}
		states = append(states, e.State)
	}
	if len(got) != 3 || states[0] != Running || states[1] != Waiting || states[2] != Idle {
		t.Fatalf("want running → waiting → idle, got %v (%+v)", states, got)
	}
	if got[0].Command != "sleep" || !got[1].Notify || !got[2].Finished || !got[2].Notify || got[2].Seconds < 1 {
		t.Fatalf("details: %+v", got)
	}

	// An ignored interactive program never becomes "waiting" or notifies.
	mu.Lock()
	events = nil
	mu.Unlock()
	if err := m.Write("PLAT-1#2", "cat\r"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(800 * time.Millisecond)
	m.Write("PLAT-1#2", "\x04") // ctrl-d ends cat
	waitFor(t, &mu, &events, func(es []Activity) bool { return len(es) > 0 && es[len(es)-1].Finished })
	mu.Lock()
	defer mu.Unlock()
	for _, e := range events {
		if e.State == Waiting || e.Notify {
			t.Fatalf("ignored program notified: %+v", events)
		}
	}
}

func waitFor(t *testing.T, mu *sync.Mutex, events *[]Activity, ok func([]Activity) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := ok(*events)
		mu.Unlock()
		if done {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("timed out; events: %+v", *events)
}

func TestSessionTicket(t *testing.T) {
	for in, want := range map[string]string{"PLAT-1": "PLAT-1", "PLAT-1#2": "PLAT-1", "": ""} {
		if got := SessionTicket(in); got != want {
			t.Errorf("SessionTicket(%q) = %q", in, got)
		}
	}
}
