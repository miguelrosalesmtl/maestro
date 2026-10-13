package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// Minimal valid personal file for the REST transport.
const validPersonal = `version: 1
jira:
  site: https://acme.atlassian.net
  auth:
    method: apiToken
    email: me@acme.com
    credentialRef: keychain:maestro/jira:acme.atlassian.net:me@acme.com
`

func TestExampleFileIsValid(t *testing.T) {
	l, err := Load("../../docs/config.example.yaml")
	if err != nil {
		t.Fatalf("docs/config.example.yaml must stay valid:\n%v", err)
	}
	// The repo path in the example does not exist here: a warning, not an error.
	for _, p := range l.Problems {
		if p.Path != "projects[0].repos[0].path" {
			t.Errorf("unexpected problem: %s", p)
		}
	}
	if got := l.Effective.Workflow.States["In Review"]; got != StateInReview {
		t.Errorf("states not loaded: %v", l.Effective.Workflow.States)
	}
}

func TestMissingFileGivesDefaultsAndNeedsSetup(t *testing.T) {
	l, err := Load(filepath.Join(t.TempDir(), "config.yaml"))
	var inv *InvalidError
	if !errors.As(err, &inv) {
		t.Fatalf("a fresh install has no site; want InvalidError, got %v", err)
	}
	if !l.NeedsSetup() {
		t.Error("NeedsSetup should be true without a site")
	}
	if l.Effective.Jira.HTTP.TimeoutSeconds != 20 || l.Effective.Dev.PR.MergeMethod != "squash" {
		t.Error("defaults not applied")
	}
}

func TestValidPersonalFile(t *testing.T) {
	l, err := Load(write(t, t.TempDir(), "config.yaml", validPersonal))
	if err != nil {
		t.Fatal(err)
	}
	if l.NeedsSetup() || len(l.Problems) != 0 {
		t.Fatalf("problems: %v", l.Problems)
	}
}

func TestUnknownKeysAreErrorsWithLines(t *testing.T) {
	_, err := Load(write(t, t.TempDir(), "config.yaml", validPersonal+"projcts:\n  - key: PLAT\n"))
	var inv *InvalidError
	if !errors.As(err, &inv) {
		t.Fatalf("want InvalidError, got %v", err)
	}
	p := inv.Problems.Errors()[0]
	if p.Line != 8 || !strings.Contains(p.Message, "projcts") {
		t.Fatalf("want typo reported at line 8, got %s", p)
	}
}

func TestSecretsAreRejected(t *testing.T) {
	body := validPersonal + `terminal:
  env:
    API_TOKEN: hunter2
    NOTE: ATATT3xFfGF0abcdefghijklmnopqrstuvwxyz
`
	_, err := Load(write(t, t.TempDir(), "config.yaml", body))
	var inv *InvalidError
	if !errors.As(err, &inv) {
		t.Fatalf("want InvalidError, got %v", err)
	}
	got := map[string]int{}
	for _, p := range inv.Problems.Errors() {
		got[p.Path] = p.Line
	}
	if got["terminal.env.API_TOKEN"] != 10 || got["terminal.env.NOTE"] != 11 {
		t.Fatalf("secret findings = %v", got)
	}
	if _, ok := got["jira.auth.credentialRef"]; ok {
		t.Fatal("credentialRef is allowed and must not be flagged")
	}
}

func TestAllErrorsAreReported(t *testing.T) {
	body := `version: 1
jira:
  site: http://acme.atlassian.net/browse/PLAT-1
  auth: { method: pat }
  http: { timeoutSeconds: 0 }
projects:
  - key: plat
  - key: plat
automation:
  rules:
    - { trigger: pr_merged, to: done }
    - { trigger: pr_merged, to: inReview }
    - { trigger: deployed, to: done }
`
	_, err := Load(write(t, t.TempDir(), "config.yaml", body))
	var inv *InvalidError
	if !errors.As(err, &inv) {
		t.Fatalf("want InvalidError, got %v", err)
	}
	paths := map[string]bool{}
	for _, p := range inv.Problems.Errors() {
		paths[p.Path] = true
	}
	for _, want := range []string{
		"jira.site", "jira.auth.credentialRef", "jira.http.timeoutSeconds",
		"projects[0].key", "projects[1].key", "automation.rules[1]", "automation.rules[2].trigger",
	} {
		if !paths[want] {
			t.Errorf("missing problem at %s; got %v", want, paths)
		}
	}
}

func TestTeamAndPersonalMerge(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "team.yaml", `version: 1
projects:
  - key: PLAT
workflow:
  states: { "To Do": todo, "Code Review": inReview }
dev:
  branch:
    prefix: { story: feat }
automation:
  rules:
    - { trigger: pr_merged, to: "Ready to Deploy" }
`)
	personal := write(t, dir, "config.yaml", validPersonal+`teamFile: team.yaml
workflow:
  states: { "QA": inReview }
ui:
  defaultView: board
`)
	l, err := Load(personal)
	if err != nil {
		t.Fatal(err)
	}
	e := l.Effective
	if len(e.Projects) != 1 || e.Projects[0].Key != "PLAT" {
		t.Errorf("projects from team file missing: %+v", e.Projects)
	}
	if len(e.Workflow.States) != 3 {
		t.Errorf("maps must merge across layers: %v", e.Workflow.States)
	}
	if p := e.Dev.Branch.Prefix; p["story"] != "feat" || p["bug"] != "fix" || p["default"] != "feature" {
		t.Errorf("prefix map must merge with defaults: %v", p)
	}
	if len(e.Automation.Rules) != 1 || e.Automation.Rules[0].To != "Ready to Deploy" {
		t.Errorf("lists replace defaults as a whole: %+v", e.Automation.Rules)
	}
	if e.UI.DefaultView != "board" {
		t.Errorf("personal must win: %q", e.UI.DefaultView)
	}
}

func TestTeamFileCannotHoldCredentials(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "team.yaml", "version: 1\njira:\n  auth:\n    email: shared@acme.com\n")
	_, err := Load(write(t, dir, "config.yaml", validPersonal+"teamFile: team.yaml\n"))
	if err == nil || !strings.Contains(err.Error(), "personal credentials") {
		t.Fatalf("want team-file credential error, got %v", err)
	}
}

func TestImportIsAllOrNothing(t *testing.T) {
	dir := t.TempDir()
	personal := write(t, dir, "config.yaml", validPersonal)
	before, _ := os.ReadFile(personal)

	bad := write(t, dir, "bad.yaml", "version: 1\njira:\n  site: nope\n")
	if probs, err := Import(bad, personal); err == nil || !probs.HasErrors() {
		t.Fatalf("bad import must fail, got %v %v", probs, err)
	}
	if after, _ := os.ReadFile(personal); string(after) != string(before) {
		t.Fatal("a failed import must not change the personal file")
	}

	good := write(t, dir, "good.yaml", validPersonal+"ui:\n  defaultView: roadmap\n")
	if _, err := Import(good, personal); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(personal)
	if st.Mode().Perm() != 0o600 {
		t.Errorf("personal file mode = %v, want 0600", st.Mode().Perm())
	}
	l, err := Load(personal)
	if err != nil || l.Effective.UI.DefaultView != "roadmap" {
		t.Fatalf("imported file did not load back: %v", err)
	}
}

func TestSaveWritesOnlyThePersonalLayer(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "team.yaml", "version: 1\nprojects:\n  - key: PLAT\n")
	l, err := Load(write(t, dir, "config.yaml", validPersonal+"teamFile: team.yaml\n"))
	if err != nil {
		t.Fatal(err)
	}
	edited := *l.Personal
	edited.UI.DefaultView = "focus"
	if err := l.SetPersonal(&edited); err != nil || l.Problems.HasErrors() {
		t.Fatalf("remerge: %v %v", err, l.Problems)
	}
	if err := l.SavePersonal(); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(l.PersonalPath)
	s := string(saved)
	if !strings.Contains(s, "defaultView: focus") {
		t.Errorf("edit not saved:\n%s", s)
	}
	for _, leaked := range []string{"PLAT", "timeoutSeconds", "squash"} {
		if strings.Contains(s, leaked) {
			t.Errorf("team/default value %q leaked into personal file:\n%s", leaked, s)
		}
	}
}

func TestFieldsSprintsVersionsValidation(t *testing.T) {
	body := validPersonal + `fields:
  custom:
    - { name: Severity, role: severity, show: [view, card] }
    - { name: Severity }
    - { id: summary, show: [sidebar] }
    - { name: Other, role: severity, requiredOn: [merged] }
    - {}
defaults:
  create: { sprint: tomorrow, fixVersion: "" }
sprints: { nameTemplate: "Sprint" }
versions: { pattern: "(", fromBranch: { enabled: true, template: "release" } }
`
	_, err := Load(write(t, t.TempDir(), "config.yaml", body))
	var inv *InvalidError
	if !errors.As(err, &inv) {
		t.Fatalf("want InvalidError, got %v", err)
	}
	got := map[string]bool{}
	for _, p := range inv.Problems.Errors() {
		got[p.Path] = true
	}
	for _, want := range []string{
		"fields.custom[1]", "fields.custom[2].id", "fields.custom[2].show", "fields.custom[3].role", "fields.custom[3].requiredOn",
		"fields.custom[4]", "defaults.create.sprint", "defaults.create.fixVersion", "sprints.nameTemplate",
		"versions.pattern", "versions.fromBranch.template",
	} {
		if !got[want] {
			t.Errorf("missing problem at %s; got %v", want, got)
		}
	}
}

func TestCustomFieldShows(t *testing.T) {
	if !(CustomField{Name: "x"}).Shows("view") || (CustomField{Name: "x"}).Shows("card") {
		t.Error("default place is the Jira view only")
	}
	if f := (CustomField{Name: "x", Show: []string{"card"}}); f.Shows("view") || !f.Shows("card") {
		t.Error("explicit places replace the default")
	}
}
