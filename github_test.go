package main

import (
	"context"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"maestro/internal/config"
	"maestro/internal/github"
	"maestro/internal/github/ghtest"
	"maestro/internal/secrets"
	"maestro/internal/tracker/jira/jiratest"
)

// A pull request on GitHub moves the real Jira issue: opened → review,
// merged → done (once opted in), and the PR is linked on the issue.
func TestGitHubDrivesJiraEndToEnd(t *testing.T) {
	keyring.MockInit()
	f := jiratest.NewTLS(t)
	gh := ghtest.New(t)
	cfg := liveConfig(t, f)
	cfg.Projects[0].Repos = []config.Repo{{Name: "acme/app"}}
	cfg.Dev.GitHub.APIURL = gh.URL
	cfg.Dev.GitHub.Auth = "token"
	store := secrets.NewStore()
	if err := store.Set(secrets.JiraRef("fake", "me@x.com"), "tok"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(secrets.GitHubRef(github.WebHost(gh.URL)), ghtest.Token); err != nil {
		t.Fatal(err)
	}
	f.SetStatus("PLAT-2", "In Progress")
	cfg.Automation.Rules[3].Enabled = nil // opt in to "PR merged" → done (off by default)

	app := NewApp(cfg, nil)
	app.connect()
	defer app.shutdown(nil)
	if c := app.GetConnection(); c.Mode != "live" {
		t.Fatalf("connection = %+v", c)
	}

	// Wait for the first (baseline) look.
	deadline := time.Now().Add(5 * time.Second)
	for app.GetGitHubStatus().LastSync == "" {
		if time.Now().After(deadline) {
			t.Fatalf("github never synced: %+v", app.GetGitHubStatus())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st := app.GetGitHubStatus(); st.State != "ok" || st.User != "octo" {
		t.Fatalf("status = %+v", st)
	}
	sync := func() { app.dev.w.Sync(context.Background()) }

	gh.Update("acme/app", 21, func(p *ghtest.PR) {
		p.Title, p.Head = "Story work", "feature/PLAT-2-story"
		p.Checks = []ghtest.Check{{Name: "build", Status: "in_progress"}}
	})
	sync()
	if got := f.Status("PLAT-2"); got != "Code Review" {
		t.Fatalf("PR opened: Jira has %q", got)
	}
	links := f.RemoteLinks("PLAT-2")
	if l, ok := links["https://github.com/x/pull/21"]; !ok || l.Resolved || l.Title != "acme/app#21: Story work" {
		t.Fatalf("remote link: %+v", links)
	}
	if fl := app.fl().Flow("PLAT-2"); !fl.Live || fl.PR == nil || fl.PR.Number != 21 {
		t.Fatalf("flow: %+v", fl)
	}

	sync() // nothing changed: no second write
	if l := f.RemoteLinks("PLAT-2")["https://github.com/x/pull/21"]; l.Posts != 1 {
		t.Fatalf("link written %d times", l.Posts)
	}

	gh.Update("acme/app", 21, func(p *ghtest.PR) {
		p.Checks = []ghtest.Check{{Name: "build", Status: "completed", Conclusion: "success"}}
		p.Reviews = []ghtest.Review{{User: "ana", State: "APPROVED", At: "2026-10-01T00:00:00Z"}}
		p.State, p.MergedAt = "closed", "2026-10-02T00:00:00Z"
	})
	sync()
	if got := f.Status("PLAT-2"); got != "Done" {
		t.Fatalf("PR merged: Jira has %q", got)
	}
	if l := f.RemoteLinks("PLAT-2")["https://github.com/x/pull/21"]; !l.Resolved || l.Posts != 2 {
		t.Fatalf("merged link: %+v", l)
	}
}
