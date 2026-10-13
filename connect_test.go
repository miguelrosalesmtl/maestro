package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"maestro/internal/config"
	"maestro/internal/secrets"
	"maestro/internal/tracker/jira/jiratest"
)

// liveConfig points Maestro at the fake Jira over HTTPS, trusting its
// certificate through jira.tls.caFile, exactly as a user's config would.
func liveConfig(t *testing.T, f *jiratest.Server) *config.Config {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir()) // discovery cache stays in the test
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, f.CertPEM(), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Jira.Site = f.URL()
	cfg.Jira.Deployment = config.Cloud
	cfg.Jira.TLS.CAFile = ca
	cfg.Jira.Auth = config.Auth{Method: config.AuthAPIToken, Email: "me@x.com",
		CredentialRef: secrets.JiraRef("fake", "me@x.com").String()}
	cfg.Projects = []config.Project{{Key: "PLAT"}}
	cfg.Workflow.States = map[string]config.State{"Code Review": config.StateInReview}
	return cfg
}

func TestAppConnectsToJiraEndToEnd(t *testing.T) {
	keyring.MockInit()
	f := jiratest.NewTLS(t)
	cfg := liveConfig(t, f)
	if err := secrets.NewStore().Set(secrets.JiraRef("fake", "me@x.com"), "tok"); err != nil {
		t.Fatal(err)
	}

	app := NewApp(cfg, nil)
	app.connect()
	defer app.shutdown(nil)

	conn := app.GetConnection()
	if conn.Mode != "live" || conn.State != "live" || conn.Error != "" {
		t.Fatalf("connection = %+v", conn)
	}
	snap := app.Snapshot()
	if !snap.Live || snap.Me.ID != "me" || snap.Sprint != "Sprint 42" || snap.StatusStates["Code Review"] != "inReview" {
		t.Fatalf("snapshot meta: live=%v me=%v sprint=%q states=%v", snap.Live, snap.Me, snap.Sprint, snap.StatusStates)
	}
	keys := []string{}
	for _, is := range snap.Issues {
		keys = append(keys, is.Key)
	}
	// MOB-7 is another team's project but assigned to me: My work shows it.
	if strings.Join(keys, ",") != "MOB-7,OPS-1,PLAT-1,PLAT-2,PLAT-3" {
		t.Fatalf("issues = %v", keys)
	}
	if len(snap.Flows) != 5 || len(snap.Activity) != 0 {
		t.Fatalf("live mode must not contain demo flows or events: %d flows, %d events", len(snap.Flows), len(snap.Activity))
	}

	// A status change from the UI goes to Jira.
	is, err := app.TransitionIssue("PLAT-2", "Code Review")
	if err != nil || is.Status != "Code Review" || f.Status("PLAT-2") != "Code Review" {
		t.Fatalf("transition: %+v %v (jira has %q)", is, err, f.Status("PLAT-2"))
	}

	// Typing "git checkout -b" in the ticket terminal fires the "branch
	// created" rule, which moves the Jira issue to In Progress.
	if _, err := app.TransitionIssue("PLAT-1", "To Do"); err == nil {
		t.Fatal("the fake has no transition to To Do; the error must surface")
	}
	if res, ok := app.fl().OnCommand("PLAT-2", "git checkout -b feature/PLAT-2-story"); !ok || res.Flow.Branch != "feature/PLAT-2-story" {
		t.Fatalf("terminal command not handled: %+v", res)
	}
	if got := f.Status("PLAT-2"); got != "In Progress" {
		t.Fatalf("automation must transition the real issue, Jira has %q", got)
	}
}

func TestAppFallsBackToDemoWithReason(t *testing.T) {
	keyring.MockInit() // empty keychain: no token stored
	f := jiratest.NewTLS(t)
	app := NewApp(liveConfig(t, f), nil)
	app.connect()

	conn := app.GetConnection()
	if conn.Mode != "demo" || !conn.Configured || !strings.Contains(conn.Error, "maestro-jira login") {
		t.Fatalf("connection = %+v", conn)
	}
	if snap := app.Snapshot(); snap.Live || len(snap.Issues) == 0 {
		t.Fatal("demo data must still load")
	}
}

func TestAppWithoutLoginIsDemo(t *testing.T) {
	app := NewApp(config.Default(), nil)
	app.connect()
	if c := app.GetConnection(); c.Mode != "demo" || c.Configured || c.Error != "" {
		t.Fatalf("connection = %+v", c)
	}
}
