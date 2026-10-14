package devsync

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"maestro/internal/flow"
	"maestro/internal/github"
	"maestro/internal/github/ghtest"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Dev", "GIT_AUTHOR_EMAIL=d@x", "GIT_COMMITTER_NAME=Dev", "GIT_COMMITTER_EMAIL=d@x", "GIT_CONFIG_GLOBAL=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	write("a.txt", "one\n")
	run("add", ".")
	run("commit", "-q", "-m", "init")
	run("checkout", "-q", "-b", "feature/PLAT-3-local-work")
	write("b.txt", "1\n2\n3\n")
	run("add", ".")
	run("commit", "-q", "-m", "PLAT-3: first")
	write("b.txt", "1\n")
	run("commit", "-q", "-am", "PLAT-3: second")
	run("checkout", "-q", "-b", "chore/NOPE-1-unknown", "main")
	run("checkout", "-q", "main")
	return dir
}

type applied struct {
	ds   flow.DevState
	fire bool
}

func TestSyncMatchesPRsAndLocalBranches(t *testing.T) {
	dir := gitRepo(t)
	gh := ghtest.New(t)
	gh.Update("acme/app", 5, func(p *ghtest.PR) {
		p.Title, p.Head = "Add retries", "feature/PLAT-2-retries"
		p.Checks = []ghtest.Check{{Name: "build", Status: "completed", Conclusion: "success"}, {Name: "lint", Status: "in_progress"}}
		p.Statuses = []ghtest.Status{{Context: "ci/legacy", State: "success"}}
		p.Reviews = []ghtest.Review{{User: "ana", State: "APPROVED", At: "2026-01-01T00:00:00Z"}}
		p.Commits = []ghtest.Commit{{SHA: "abcdef1234", Message: "PLAT-2: retries\n\nbody", Author: "Dev", At: "2026-01-01T00:00:00Z"}}
	})
	// Key only in the title; an unrelated PR names an unknown key.
	gh.Update("acme/app", 6, func(p *ghtest.PR) { p.Title, p.Head = "[PLAT-1] epic plumbing", "misc" })
	gh.Update("acme/app", 8, func(p *ghtest.PR) { p.Title, p.Head = "NOPE-1", "nope" })

	known := map[string]bool{"PLAT-1": true, "PLAT-2": true, "PLAT-3": true}
	got := map[string]applied{}
	client, _ := github.New(github.Options{APIURL: gh.URL, Tokens: github.Static(ghtest.Token)})
	w, err := New(Options{Client: client, Repos: []Repo{{Name: "acme/app", Path: dir}}, Known: func(k string) bool { return known[k] },
		RequiredChecks: []string{"build", "deploy-preview"}, RequiredApprovals: 1,
		Apply: func(k string, ds flow.DevState, fire bool) { got[k] = applied{ds, fire} }})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	w.Sync(ctx)

	if len(got) != 3 {
		t.Fatalf("applied %v", got)
	}
	p2 := got["PLAT-2"]
	if p2.fire || p2.ds.PR == nil || p2.ds.PR.Approvals != 1 || p2.ds.Branch != "feature/PLAT-2-retries" || len(p2.ds.Commits) != 1 || p2.ds.Commits[0].Message != "PLAT-2: retries" {
		t.Fatalf("PLAT-2: %+v", p2)
	}
	checks := map[string]flow.Check{}
	for _, c := range p2.ds.PR.Checks {
		checks[c.Name] = c
	}
	if checks["build"].State != flow.StateDone || !checks["build"].Required || checks["lint"].State != flow.StateActive ||
		checks["ci/legacy"].State != flow.StateDone || checks["deploy-preview"].State != flow.StateActive || !checks["deploy-preview"].Required {
		t.Fatalf("checks: %+v", checks)
	}
	if got["PLAT-1"].ds.PR == nil || got["PLAT-1"].ds.PR.Number != 6 {
		t.Fatalf("title match: %+v", got["PLAT-1"])
	}
	p3 := got["PLAT-3"]
	if p3.ds.PR != nil || p3.ds.Branch != "feature/PLAT-3-local-work" || len(p3.ds.Commits) != 2 || p3.ds.Commits[0].Message != "PLAT-3: first" || p3.ds.Commits[0].Adds != 3 {
		t.Fatalf("local branch: %+v", p3)
	}
	if st := w.Status(); st.State != "ok" || st.User != "octo" || st.PRs != 2 {
		t.Fatalf("status: %+v", st)
	}

	// Second look: the merge is an observed change and fires.
	gh.Update("acme/app", 5, func(p *ghtest.PR) { p.State, p.MergedAt = "closed", "2026-01-02T00:00:00Z" })
	w.Sync(ctx)
	if p2 := got["PLAT-2"]; !p2.fire || p2.ds.PR.State != "merged" {
		t.Fatalf("merged: %+v", p2)
	}
	req, _ := gh.Counts()
	w.Sync(ctx)
	w.Sync(ctx)
	if after, nm := gh.Counts(); nm == 0 || after-req > 16 {
		t.Fatalf("unchanged repos must be cheap: %d requests, %d not modified", after-req, nm)
	}
}

func TestBadTokenKeepsLocalWork(t *testing.T) {
	dir := gitRepo(t)
	gh := ghtest.New(t)
	client, _ := github.New(github.Options{APIURL: gh.URL, Tokens: github.Static("nope")})
	var keys []string
	w, _ := New(Options{Client: client, Host: "github.com", Repos: []Repo{{Name: "acme/app", Path: dir}},
		Known: func(k string) bool { return k == "PLAT-3" },
		Apply: func(k string, ds flow.DevState, fire bool) { keys = append(keys, k) }})
	w.Sync(context.Background())
	st := w.Status()
	if st.State != "error" || st.Error == "" || len(keys) != 1 {
		t.Fatalf("status %+v, applied %v", st, keys)
	}
}

// Regression (issue #4): a ticket that joins the synced set after the first
// look (reassigned, edited back into the window) had its old merged PR
// replayed as new: "branch created" then "PR merged" fired back to back.
func TestTicketJoiningLaterIsABaseline(t *testing.T) {
	gh := ghtest.New(t)
	gh.Update("acme/app", 5, func(p *ghtest.PR) {
		p.Title, p.Head, p.State, p.MergedAt = "Old work", "feature/PLAT-9-old", "closed", "2026-01-02T00:00:00Z"
	})
	known := map[string]bool{"PLAT-1": true}
	got := map[string]applied{}
	client, _ := github.New(github.Options{APIURL: gh.URL, Tokens: github.Static(ghtest.Token)})
	w, _ := New(Options{Client: client, Repos: []Repo{{Name: "acme/app"}}, Known: func(k string) bool { return known[k] },
		Apply: func(k string, ds flow.DevState, fire bool) { got[k] = applied{ds, fire} }})
	ctx := context.Background()
	w.Sync(ctx)
	if _, ok := got["PLAT-9"]; ok {
		t.Fatal("PLAT-9 is not synced yet")
	}

	known["PLAT-9"] = true // enters the synced set
	w.Sync(ctx)
	if p9, ok := got["PLAT-9"]; !ok || p9.fire || p9.ds.PR == nil {
		t.Fatalf("PLAT-9 first look must be shown and not fire: %+v", p9)
	}

	// New work for a synced ticket after that is news and fires, and the
	// user's own pull request is marked as theirs.
	gh.Update("acme/app", 7, func(p *ghtest.PR) { p.Title, p.Head = "Start", "feature/PLAT-1-new" })
	w.Sync(ctx)
	if p1 := got["PLAT-1"]; !p1.fire || p1.ds.PR == nil || p1.ds.PR.Number != 7 || !p1.ds.Mine {
		t.Fatalf("PLAT-1: %+v", p1)
	}
	gh.Update("acme/app", 7, func(p *ghtest.PR) { p.State = "closed" })
	gh.Update("acme/app", 8, func(p *ghtest.PR) { p.Title, p.Head, p.Author = "Takeover", "fix/PLAT-1-other", "teammate" })
	w.Sync(ctx)
	if p1 := got["PLAT-1"]; p1.ds.PR == nil || p1.ds.PR.Number != 8 || p1.ds.Mine {
		t.Fatalf("a teammate's PR is not the user's: %+v", p1)
	}
}
