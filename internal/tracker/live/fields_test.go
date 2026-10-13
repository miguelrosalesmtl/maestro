package live

import (
	"encoding/json"
	"strings"
	"testing"

	"context"
	"maestro/internal/config"
	"maestro/internal/secrets"
	"maestro/internal/tracker"
	"maestro/internal/tracker/jira"
	"maestro/internal/tracker/jira/jiratest"
)

// trackerWith builds a tracker with extra config applied.
func trackerWith(t *testing.T, f *jiratest.Server, tweak func(*config.Config)) *Tracker {
	t.Helper()
	c, err := jira.New(jira.Options{BaseURL: f.URL(), Deployment: jira.Cloud, HTTPClient: f.Client(),
		Auth: jira.BasicToken{Email: "me@x.com", Token: secrets.Secret("tok")}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Projects = []config.Project{{Key: "PLAT"}}
	cfg.Workflow.States = map[string]config.State{"Code Review": config.StateInReview}
	if tweak != nil {
		tweak(cfg)
	}
	tr := New(Options{Client: c, Config: cfg})
	if err := tr.discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := tr.fullSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	return tr
}

func issueByKey(tr *Tracker, key string) tracker.Issue {
	for _, i := range tr.ListIssues() {
		if i.Key == key {
			return i
		}
	}
	return tracker.Issue{}
}

func withSeverity(c *config.Config) {
	c.Fields.Custom = []config.CustomField{
		{Name: "Severity", Role: "severity", Show: []string{"view", "create", "card"}, Default: "Major", DefaultFor: []string{"bug"}},
		{Name: "Acceptance Criteria", Role: "acceptanceCriteria", Show: []string{"view", "create"}, RequiredOn: []string{"inReview"}},
	}
}

func TestSystemAndStandardFieldsAreMapped(t *testing.T) {
	f := jiratest.New(t)
	tr := trackerWith(t, f, withSeverity)
	is := issueByKey(tr, "PLAT-2")

	if strings.Join(is.FixVersions, ",") != "2.4.0" || is.DueDate != "2026-11-01" || is.StoryPoints != 5 {
		t.Errorf("system fields: fix=%v due=%q points=%d", is.FixVersions, is.DueDate, is.StoryPoints)
	}
	if !is.Flagged || is.CarriedOver != 1 {
		t.Errorf("flagged=%v carriedOver=%d", is.Flagged, is.CarriedOver)
	}
	if len(is.Custom) != 1 || is.Custom[0].Label != "Severity" || is.Custom[0].Text != "Major" || is.Custom[0].Role != "severity" {
		t.Errorf("custom = %+v", is.Custom)
	}
	if sec := issueByKey(tr, "PLAT-3").Security; sec != "Internal" {
		t.Errorf("security = %q", sec)
	}

	m := tr.Meta()
	var sprints []string
	for _, s := range m.Sprints {
		sprints = append(sprints, s.Name+":"+s.State)
	}
	if strings.Join(sprints, ",") != "Sprint 42:active,Hotfix sprint:active,Sprint 43:future" || m.Sprint != "Sprint 42" {
		t.Errorf("sprints = %v (current %q)", sprints, m.Sprint)
	}
	var versions []string
	for _, v := range m.Versions["PLAT"] {
		versions = append(versions, v.Name)
	}
	if strings.Join(versions, ",") != "2.4.0,2.5.0,2.3.0" {
		t.Errorf("versions = %v (archived and old must be hidden)", versions)
	}
	if !m.TimeTracking || len(m.CustomColumns) != 2 || m.CreateDefaults.Sprint != "active" {
		t.Errorf("meta: tt=%v cols=%+v defaults=%+v", m.TimeTracking, m.CustomColumns, m.CreateDefaults)
	}
}

func TestJiraViewShowsUnconfiguredCustomFields(t *testing.T) {
	f := jiratest.New(t)
	tr := trackerWith(t, f, nil) // no fields.custom: default
	is, err := tr.GetIssue("PLAT-2")
	if err != nil {
		t.Fatal(err)
	}
	labels := []string{}
	for _, c := range is.Custom {
		labels = append(labels, c.Label+"="+c.Text+strings.Join(c.Items, "|"))
	}
	// Flagged and Sprint are standard fields shown elsewhere, not repeated here.
	if strings.Join(labels, ",") != "Severity=Major" {
		t.Errorf("custom in Jira view = %v", labels)
	}
	if is.TimeTracking == nil || is.TimeTracking.Remaining != "1d" {
		t.Errorf("time tracking = %+v", is.TimeTracking)
	}
}

func TestFieldSpecsAndUpdate(t *testing.T) {
	f := jiratest.New(t)
	tr := trackerWith(t, f, withSeverity)

	specs, err := tr.FieldSpecs("PLAT-2")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]tracker.FieldSpec{}
	for _, s := range specs {
		got[s.ID] = s
	}
	if got["fixVersions"].Kind != "versions" || strings.Join(got["fixVersions"].Options, ",") != "2.4.0,2.5.0" ||
		got["customfield_10050"].Label != "Severity" || len(got["customfield_10050"].Options) != 3 || got["duedate"].Kind != "date" {
		t.Fatalf("specs = %+v", specs)
	}
	if _, ok := got["versions"]; ok {
		t.Error("fields not on the edit screen must not be offered")
	}

	fix := []string{"2.5.0"}
	due := ""
	is, err := tr.UpdateFields("PLAT-2", tracker.FieldPatch{FixVersions: &fix, DueDate: &due,
		Custom: map[string][]string{"customfield_10050": {"Critical"}}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(f.Edits[len(f.Edits)-1])
	if string(b) != `{"customfield_10050":{"value":"Critical"},"duedate":null,"fixVersions":[{"name":"2.5.0"}]}` {
		t.Errorf("edit payload = %s", b)
	}
	if strings.Join(is.FixVersions, ",") != "2.5.0" || is.DueDate != "" || is.Custom[0].Text != "Critical" {
		t.Errorf("after update: %+v", is)
	}
}

func TestCreateSpecsAndPayload(t *testing.T) {
	f := jiratest.New(t)
	tr := trackerWith(t, f, withSeverity)

	specs, err := tr.CreateFieldSpecs("PLAT", tracker.TypeStory)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range specs {
		ids = append(ids, s.ID+map[bool]string{true: "*", false: ""}[s.Required])
	}
	// Customer is required by Jira; Severity and Acceptance Criteria are configured for create.
	if strings.Join(ids, ",") != "customfield_10050,customfield_10052*,customfield_10051" {
		t.Errorf("create specs = %v", ids)
	}
	for _, s := range specs {
		if s.ID == "customfield_10050" && s.Default != "" {
			t.Errorf("Severity's default is only for bugs: %+v", s)
		}
	}

	_, err = tr.CreateIssue(tracker.NewIssue{Type: tracker.TypeStory, Summary: "With fields", ParentKey: "PLAT-1",
		Sprint: "next", FixVersions: []string{"2.4.0"},
		Custom: map[string][]string{"customfield_10050": {"Minor"}, "customfield_10052": {"ACME"}}})
	if err != nil {
		t.Fatal(err)
	}
	body := f.Created[len(f.Created)-1]
	want := map[string]string{"customfield_10020": "43", "fixVersions": `[{"name":"2.4.0"}]`,
		"customfield_10050": `{"value":"Minor"}`, "customfield_10052": `"ACME"`}
	for k, v := range want {
		if b, _ := json.Marshal(body[k]); string(b) != v {
			t.Errorf("payload %s = %s, want %s", k, b, v)
		}
	}
	if _, err := tr.CreateIssue(tracker.NewIssue{Type: tracker.TypeStory, Summary: "x", Sprint: "Sprint 99"}); err == nil {
		t.Error("an unknown sprint must fail clearly")
	}
}

func TestTeamRequiredFieldsBlockTransitions(t *testing.T) {
	f := jiratest.New(t)
	tr := trackerWith(t, f, func(c *config.Config) {
		withSeverity(c)
		c.Versions.RequireFixVersionOnDone = true
	})
	_, err := tr.Transition("PLAT-2", "Code Review")
	if err == nil || !strings.Contains(err.Error(), "Acceptance Criteria") || !strings.Contains(err.Error(), "team rule") {
		t.Fatalf("inReview without acceptance criteria: %v", err)
	}
	if f.Status("PLAT-2") != "To Do" {
		t.Fatal("the issue must not move")
	}
	if _, err := tr.UpdateFields("PLAT-2", tracker.FieldPatch{Custom: map[string][]string{"customfield_10051": {"Given a deploy…"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Transition("PLAT-2", "Code Review"); err != nil {
		t.Fatalf("with acceptance criteria: %v", err)
	}
	empty := []string{}
	if _, err := tr.UpdateFields("PLAT-2", tracker.FieldPatch{FixVersions: &empty}); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Transition("PLAT-2", "Done"); err == nil || !strings.Contains(err.Error(), "Fix version") {
		t.Fatalf("done without fix version: %v", err)
	}
}
