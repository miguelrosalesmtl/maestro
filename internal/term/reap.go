package term

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// KillGrace is how long a closed terminal's processes get to exit after
// SIGHUP/SIGTERM before they are killed.
var KillGrace = 2 * time.Second

// tree is pid and every process below it (children, grandchildren …). It must
// be taken before the shell dies: orphans move under launchd/init and leave
// the tree, which is how an agent or dev server outlived its terminal.
func tree(pid int) []int {
	t := procTable()
	if t == nil {
		return []int{pid}
	}
	return below(t, pid)
}

// reap asks every process to exit (SIGHUP, as a closed terminal does, and
// SIGTERM), waits up to grace, then kills what is left.
func reap(pids []int, grace time.Duration) {
	for _, p := range pids {
		_ = syscall.Kill(p, syscall.SIGHUP)
		_ = syscall.Kill(p, syscall.SIGTERM)
	}
	if pids = waitGone(pids, grace); len(pids) == 0 {
		return
	}
	for _, p := range pids {
		_ = syscall.Kill(p, syscall.SIGKILL)
	}
	waitGone(pids, time.Second)
}

// waitGone polls until the processes have exited or d passes; it returns
// the ones still running.
func waitGone(pids []int, d time.Duration) []int {
	deadline := time.Now().Add(d)
	for {
		alive := pids[:0:0]
		for _, p := range pids {
			if syscall.Kill(p, 0) == nil && !zombie(p) {
				alive = append(alive, p)
			}
		}
		if len(alive) == 0 || time.Now().After(deadline) {
			return alive
		}
		pids = alive
		time.Sleep(50 * time.Millisecond)
	}
}

// zombie: exited, waiting for its parent to collect it (no longer running).
func zombie(pid int) bool {
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	return err == nil && strings.HasPrefix(strings.TrimSpace(string(out)), "Z")
}

// reapAll ends several terminals at once with one shared grace period.
func reapAll(groups [][]int, grace time.Duration) {
	var wg sync.WaitGroup
	for _, g := range groups {
		wg.Add(1)
		go func() { defer wg.Done(); reap(g, grace) }()
	}
	wg.Wait()
}
