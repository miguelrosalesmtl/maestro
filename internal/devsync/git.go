package devsync

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"

	"maestro/internal/flow"
)

// runGit runs git in dir. Output is stdout only.
func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && stderr.Len() > 0 {
		return out, &gitError{args: args, msg: strings.TrimSpace(stderr.String()), err: err}
	}
	return out, err
}

type gitError struct {
	args []string
	msg  string
	err  error
}

func (e *gitError) Error() string { return "git " + strings.Join(e.args, " ") + ": " + e.msg }
func (e *gitError) Unwrap() error { return e.err }

// localWork is what the clone knows about a ticket before (or besides) a PR.
type localWork struct {
	Branch  string
	Commits []flow.Commit // oldest first
	At      string        // last commit time, to choose between branches
}

// baseRef finds the ref that work branches are compared to.
func (w *Watcher) baseRef(ctx context.Context, dir, base string) string {
	var tries []string
	if base != "" {
		tries = append(tries, "origin/"+base, base)
	} else {
		if out, err := w.git(ctx, dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
			tries = append(tries, strings.TrimSpace(string(out)))
		}
		tries = append(tries, "origin/main", "main", "origin/master", "master")
	}
	for _, r := range tries {
		if r == "" {
			continue
		}
		if _, err := w.git(ctx, dir, "rev-parse", "--verify", "--quiet", r+"^{commit}"); err == nil {
			return r
		}
	}
	return ""
}

// scanLocal finds local branches that name a ticket and their commits that
// are not on the base branch.
func (w *Watcher) scanLocal(ctx context.Context, dir, base string) (map[string]localWork, error) {
	out, err := w.git(ctx, dir, "for-each-ref", "--sort=-committerdate", "--format=%(refname:short)%09%(committerdate:iso-strict)", "refs/heads")
	if err != nil {
		return nil, err
	}
	baseRef := w.baseRef(ctx, dir, base)
	baseName := strings.TrimPrefix(baseRef, "origin/")
	found := map[string]localWork{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		branch, at, _ := strings.Cut(line, "\t")
		if branch == "" || branch == baseName {
			continue
		}
		for _, key := range w.keysIn(branch) {
			if _, seen := found[key]; seen {
				continue // newest branch for the key wins (sorted by date)
			}
			lw := localWork{Branch: branch, At: at}
			if baseRef != "" {
				lw.Commits, err = w.commits(ctx, dir, baseRef, branch)
				if err != nil {
					w.log.Debug("git log failed", "dir", dir, "branch", branch, "err", err.Error())
				}
			}
			found[key] = lw
		}
	}
	return found, nil
}

const (
	recSep  = "\x1e"
	unitSep = "\x1f"
)

// commits lists base..branch, oldest first, with line counts (≤ 50).
func (w *Watcher) commits(ctx context.Context, dir, base, branch string) ([]flow.Commit, error) {
	out, err := w.git(ctx, dir, "log", "--no-merges", "-n", "50", "--numstat",
		"--format="+recSep+"%h"+unitSep+"%an"+unitSep+"%aI"+unitSep+"%s", base+".."+branch, "--")
	if err != nil {
		return nil, err
	}
	var cs []flow.Commit
	for _, rec := range strings.Split(string(out), recSep) {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		head, stats, _ := strings.Cut(rec, "\n")
		p := strings.SplitN(head, unitSep, 4)
		if len(p) < 4 {
			continue
		}
		c := flow.Commit{SHA: p[0], Author: p[1], At: p[2], Message: p[3]}
		for _, l := range strings.Split(stats, "\n") {
			f := strings.Fields(l)
			if len(f) < 3 {
				continue
			}
			a, _ := strconv.Atoi(f[0]) // "-" for binary files
			d, _ := strconv.Atoi(f[1])
			c.Adds += a
			c.Dels += d
		}
		cs = append(cs, c)
	}
	for i, j := 0, len(cs)-1; i < j; i, j = i+1, j-1 {
		cs[i], cs[j] = cs[j], cs[i]
	}
	return cs, nil
}
