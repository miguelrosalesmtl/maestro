package setup

import (
	"strings"
	"testing"

	"maestro/internal/config"
	"maestro/internal/tracker/jira"
)

func TestCheckAgainst(t *testing.T) {
	d := &jira.Discovery{
		Fields: []jira.Field{
			{ID: "customfield_1", Name: "Severity", Custom: true, Schema: jira.FieldSchema{Type: "option"}},
			{ID: "customfield_2", Name: "Team", Custom: true, Schema: jira.FieldSchema{Type: "team", Custom: jira.SchemaTeam}},
			{ID: "customfield_3", Name: "Story point estimate", Custom: true, Schema: jira.FieldSchema{Type: "number", Custom: jira.SchemaStoryPointEst}},
			{ID: "customfield_4", Name: "Story Points", Custom: true, Schema: jira.FieldSchema{Type: "number", Custom: jira.SchemaFloat}},
		},
		Projects: []jira.ProjectDiscovery{{
			Project:  jira.Project{ID: "10", Key: "PLAT"},
			Statuses: []jira.StatusDef{{ID: "1", Name: "To Do"}},
			Versions: []jira.Version{{Name: "2.4"}, {Name: "2.3", Released: true}},
		}, {Project: jira.Project{Key: "GHOST"}}},
	}
	cfg := config.Default()
	cfg.Projects = []config.Project{{Key: "PLAT"}, {Key: "GHOST"}}
	cfg.Workflow.States = map[string]config.State{"To Do": config.StateTodo, "Shipped": config.StateDone}
	cfg.Fields.Custom = []config.CustomField{
		{Name: "Severity", Show: []string{"view", "create"}},
		{Name: "Team", Show: []string{"create"}},
		{Name: "Nope"},
	}
	cfg.Defaults.Create.FixVersion = "2.3"

	ps := CheckAgainst(cfg, d)
	got := map[string]string{}
	for _, p := range ps {
		got[p.Path] += string(p.Severity) + ":" + p.Message + " | "
	}
	expect := map[string]string{
		"projects[1].key":            "error:project GHOST",
		"workflow.states.Shipped":    "warning:no status",
		"fields.custom[1].show":      "error:",
		"fields.custom[2]":           "error:Jira has no field",
		"defaults.create.fixVersion": "error:\"2.3\" is not an unreleased version of PLAT",
		"fields.storyPoints":         "two story point fields",
	}
	for path, frag := range expect {
		if !strings.Contains(got[path], frag) {
			t.Errorf("%s: want %q, got %q", path, frag, got[path])
		}
	}
	if _, bad := got["fields.custom[0]"]; bad {
		t.Errorf("Severity is fine: %v", got["fields.custom[0]"])
	}
}
