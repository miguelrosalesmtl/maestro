package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"

	"maestro/internal/config"
	"maestro/internal/flow"
	"maestro/internal/secrets"
	"maestro/internal/tracker/jira/jiratest"
)

func loadLayers(t *testing.T, personal string) *config.Layers {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(personal), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := config.Load(path)
	if l == nil {
		t.Fatal(err)
	}
	return l
}

func TestSaveSettingsWritesOnlyChanges(t *testing.T) {
	dir := t.TempDir()
	team := filepath.Join(dir, "maestro.team.yaml")
	if err := os.WriteFile(team, []byte("version: 1\ndev:\n  branch:\n    template: \"{key}/{slug}\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l := loadLayers(t, "version: 1\nteamFile: "+team+"\n")
	app := NewApp(l.Effective, nil)
	app.UseConfigLayers(l)
	app.connect() // demo

	s := app.GetSettings()
	if !s.Writable || s.Naming.BranchTemplate != "{key}/{slug}" || s.Naming.PRTitle != "[{key}] {summary}" {
		t.Fatalf("settings = %+v", s)
	}
	s.Naming.PRTitle = "{key}: {summary}"
	s.Terminal.Env = map[string]string{"FOO": "bar"}
	s.Terminal.Commands = []config.QuickCommand{{Label: "test", Command: "make test"}, {}}
	s.Terminal.Projects = map[string]config.ProjectTerminal{"PLAT": {Startup: []string{"nvm use", " "}}}
	got, err := app.SaveSettings(s)
	if err != nil || !got.Saved || len(got.Problems) != 0 {
		t.Fatalf("save: %+v %v", got, err)
	}

	raw, _ := os.ReadFile(l.PersonalPath)
	var written config.Config
	if err := yaml.Unmarshal(raw, &written); err != nil {
		t.Fatal(err)
	}
	if written.Dev.PR.TitleTemplate != "{key}: {summary}" || written.Terminal.Env["FOO"] != "bar" ||
		!slices.Equal(written.Terminal.Projects["PLAT"].Startup, []string{"nvm use"}) || len(written.Terminal.Commands) != 1 {
		t.Fatalf("personal file:\n%s", raw)
	}
	// Team and default values are not copied into the personal file.
	if written.Dev.Branch.Template != "" || written.Dev.Branch.SlugWords != 0 || written.Dev.PR.BodyTemplate != "" || written.TeamFile != team {
		t.Fatalf("personal file repeats team/defaults or lost teamFile:\n%s", raw)
	}

	// Applied right away: new branch names, new terminals, one-click commands.
	snap := app.Snapshot()
	if snap.Naming.PRTitle != "{key}: {summary}" || snap.Naming.BranchTemplate != "{key}/{slug}" || len(snap.Commands) != 1 {
		t.Fatalf("snapshot naming %+v commands %+v", snap.Naming, snap.Commands)
	}
	plat, free := app.termSetup("PLAT"), app.termSetup("")
	if !slices.Contains(plat.Env, "FOO=bar") || !slices.Equal(plat.Startup, []string{"nvm use"}) {
		t.Fatalf("PLAT terminal = %+v", plat)
	}
	if !slices.Contains(free.Env, "FOO=bar") || len(free.Startup) != 0 {
		t.Fatalf("freestyle terminal must not get project setup: %+v", free)
	}
}

func TestSaveSettingsRefusesInvalid(t *testing.T) {
	l := loadLayers(t, "version: 1\n")
	app := NewApp(l.Effective, nil)
	app.UseConfigLayers(l)
	app.connect()
	before, _ := os.ReadFile(l.PersonalPath)

	s := app.GetSettings()
	s.Naming.BranchTemplate = "{prefix}/{slug}" // no {key}
	s.Naming.PRTitle = "  "
	s.Terminal.Shell = "/no/such/shell"
	s.Terminal.Env = map[string]string{"MAESTRO_TICKET": "x"}
	got, err := app.SaveSettings(s)
	if err != nil || got.Saved {
		t.Fatalf("must not save: %+v %v", got, err)
	}
	var paths []string
	for _, p := range got.Problems {
		paths = append(paths, p.Path)
	}
	slices.Sort(paths)
	if strings.Join(paths, ",") != "dev.branch.template,dev.pr.titleTemplate,terminal.env.MAESTRO_TICKET,terminal.shell" {
		t.Fatalf("problems = %v", got.Problems)
	}
	if after, _ := os.ReadFile(l.PersonalPath); string(after) != string(before) {
		t.Fatalf("file changed:\n%s", after)
	}
	if got.Naming.BranchTemplate != "{prefix}/{key}-{slug}" || app.Snapshot().Naming.BranchTemplate != "{prefix}/{key}-{slug}" {
		t.Fatal("refused settings must not apply")
	}
}

func TestSaveRulesLive(t *testing.T) {
	keyring.MockInit()
	f := jiratest.NewTLS(t)
	cfg := liveConfig(t, f)
	if err := secrets.NewStore().Set(secrets.JiraRef("fake", "me@x.com"), "tok"); err != nil {
		t.Fatal(err)
	}
	raw, _ := yaml.Marshal(cfg)
	l := loadLayers(t, string(raw))
	app := NewApp(l.Effective, nil)
	app.UseConfigLayers(l)
	app.connect()
	defer app.shutdown(nil)

	rules := app.Snapshot().Rules
	if rules[0].Trigger != "branch_created" || rules[0].ToStatus != "In Progress" {
		t.Fatalf("rules = %+v", rules)
	}
	rules[0].Projects = []string{"MOB"} // branch rule only for MOB
	rules[1].Enabled = false
	saved, err := app.SaveRules(rules)
	if err != nil || !slices.Equal(saved[0].Projects, []string{"MOB"}) || saved[1].Enabled {
		t.Fatalf("saved %+v, %v", saved, err)
	}
	var written config.Config
	file, _ := os.ReadFile(l.PersonalPath)
	if err := yaml.Unmarshal(file, &written); err != nil {
		t.Fatal(err)
	}
	r0 := written.Automation.Rules[0]
	if r0.To != "inProgress" || !slices.Equal(r0.Projects, []string{"MOB"}) || written.Automation.Rules[1].IsEnabled() {
		t.Fatalf("rules in file (a Maestro state must stay a state):\n%s", file)
	}

	// Scoped out: a PLAT branch no longer moves the ticket.
	before := f.Status("PLAT-2")
	app.fl().OnCommand("PLAT-2", "git checkout -b feature/PLAT-2-x")
	if got := f.Status("PLAT-2"); got != before {
		t.Fatalf("rule limited to MOB moved PLAT-2 to %q", got)
	}

	// Two enabled rules for the same event and scope are refused, with a readable reason.
	dup := append(slices.Clone(saved), saved[0])
	if _, err := app.SaveRules(dup); err == nil || !strings.Contains(err.Error(), "Rule ") {
		t.Fatalf("duplicate rule error = %v", err)
	}
	if n := len(app.fl().Rules()); n != len(saved) {
		t.Fatalf("refused rules must not apply: %d rules", n)
	}
	// The automation policy is saved too and applies right away.
	if p, err := app.SavePolicy(flow.Policy{DryRun: true}); err != nil || !p.DryRun || !app.Snapshot().Policy.DryRun {
		t.Fatalf("policy %+v, %v", p, err)
	}
	file, _ = os.ReadFile(l.PersonalPath)
	if !strings.Contains(string(file), "dryRun: true") || strings.Contains(string(file), "anyAuthor") {
		t.Fatalf("policy in file:\n%s", file)
	}
}
