package term

import (
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Usage is what one terminal costs: its shell and everything started in it
// (an agent, its sub-agents, a test runner's workers …).
type Usage struct {
	Session string `json:"session"`
	Ticket  string `json:"ticket"`
	// Memory in bytes. On macOS the footprint Activity Monitor shows (with
	// compressed memory), elsewhere the resident size.
	Memory int64     `json:"memory"`
	CPU    float64   `json:"cpu"` // percent of one core, summed
	Procs  int       `json:"procs"`
	Top    []ProcSum `json:"top"` // heaviest programs, grouped by name
}

// ProcSum groups the processes of one program in a terminal.
type ProcSum struct {
	Command string `json:"command"`
	Count   int    `json:"count"`
	Memory  int64  `json:"memory"`
}

// UsageEvery is how often terminal usage is measured.
var UsageEvery = 3 * time.Second

type proc struct {
	pid, ppid int
	rss       int64 // bytes
	cpu       float64
	comm      string
}

// procTable is one look at every process (a single ps call).
func procTable() map[int]proc {
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=,rss=,pcpu=,comm=").Output()
	if err != nil {
		return nil
	}
	t := map[int]proc{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		rss, err3 := strconv.ParseInt(f[2], 10, 64)
		cpu, _ := strconv.ParseFloat(f[3], 64)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		// comm may be a path (macOS) and may contain spaces.
		t[pid] = proc{pid: pid, ppid: ppid, rss: rss * 1024, cpu: cpu, comm: filepath.Base(strings.Join(f[4:], " "))}
	}
	return t
}

// below is pid and every process under it in t.
func below(t map[int]proc, pid int) []int {
	kids := map[int][]int{}
	for _, p := range t {
		if p.pid != p.ppid {
			kids[p.ppid] = append(kids[p.ppid], p.pid)
		}
	}
	all, seen := []int{pid}, map[int]bool{pid: true}
	for i := 0; i < len(all); i++ {
		for _, c := range kids[all[i]] {
			if !seen[c] && c > 1 {
				seen[c] = true
				all = append(all, c)
			}
		}
	}
	return all
}

// usageOf sums a process tree. mem reads a process's memory (footprint on
// macOS); it returns < 0 when it cannot, and the resident size is used.
func usageOf(t map[int]proc, root int, mem func(int) int64) (u Usage) {
	groups := map[string]*ProcSum{}
	for _, pid := range below(t, root) {
		p, ok := t[pid]
		if !ok {
			continue
		}
		m := mem(pid)
		if m < 0 {
			m = p.rss
		}
		u.Memory += m
		u.CPU += p.cpu
		u.Procs++
		g := groups[p.comm]
		if g == nil {
			g = &ProcSum{Command: p.comm}
			groups[p.comm] = g
		}
		g.Count++
		g.Memory += m
	}
	for _, g := range groups {
		u.Top = append(u.Top, *g)
	}
	sort.Slice(u.Top, func(i, j int) bool { return u.Top[i].Memory > u.Top[j].Memory })
	if len(u.Top) > 5 {
		u.Top = u.Top[:5]
	}
	return u
}

// checkUsage measures every open terminal and reports them all (one event,
// so the UI can also show the total).
func (m *Manager) checkUsage() {
	m.mu.Lock()
	type item struct {
		id     string
		ticket string
		pid    int
	}
	items := make([]item, 0, len(m.sessions))
	for id, s := range m.sessions {
		if s.cmd.Process != nil {
			items = append(items, item{id, s.ticket, s.cmd.Process.Pid})
		}
	}
	m.mu.Unlock()
	if len(items) == 0 || m.hooks.OnUsage == nil {
		return
	}
	t := procTable()
	if t == nil {
		return
	}
	out := make([]Usage, 0, len(items))
	for _, it := range items {
		u := usageOf(t, it.pid, footprint)
		u.Session, u.Ticket = it.id, it.ticket
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Session < out[j].Session })
	m.hooks.OnUsage(out)
}
