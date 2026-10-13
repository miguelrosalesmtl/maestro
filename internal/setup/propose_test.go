package setup

import (
	"testing"

	"maestro/internal/config"
	"maestro/internal/tracker/jira"
)

func TestProposeRole(t *testing.T) {
	cases := []struct {
		t    jira.IssueType
		want config.Role
		conf Confidence
	}{
		{jira.IssueType{Name: "Sub-task", Subtask: true, HierarchyLevel: -1}, config.RoleSubtask, Certain},
		{jira.IssueType{Name: "Epic", HierarchyLevel: 1}, config.RoleEpic, Certain},
		{jira.IssueType{Name: "Initiative", HierarchyLevel: 2}, config.RoleInitiative, Certain},
		{jira.IssueType{Name: "Bug"}, config.RoleBug, Certain},
		{jira.IssueType{Name: "Spike"}, config.RoleTask, Certain},
		{jira.IssueType{Name: "Story"}, config.RoleStory, Certain},
		{jira.IssueType{Name: "Experiment"}, config.RoleStory, Fallback},
	}
	for _, c := range cases {
		got := ProposeRole(c.t)
		if got.Role != c.want || got.Confidence != c.conf {
			t.Errorf("%s: got %s/%s, want %s/%s", c.t.Name, got.Role, got.Confidence, c.want, c.conf)
		}
	}
}

func TestProposeState(t *testing.T) {
	cases := []struct {
		name, cat string
		want      config.State
		conf      Confidence
	}{
		{"Backlog", "TODO", config.StateTodo, Certain},
		{"Closed", "DONE", config.StateDone, Certain},
		{"In Development", "IN_PROGRESS", config.StateInProgress, Guess},
		{"Code Review", "IN_PROGRESS", config.StateInReview, Guess},
		{"QA", "IN_PROGRESS", config.StateInReview, Guess},
		{"On Hold", "IN_PROGRESS", config.StateBlocked, Guess},
		{"Waiting for customer", "IN_PROGRESS", config.StateBlocked, Guess},
		{"Blocked", "IN_PROGRESS", config.StateBlocked, Guess},
	}
	for _, c := range cases {
		got := ProposeState(jira.StatusDef{Name: c.name, StatusCategory: c.cat})
		if got.State != c.want || got.Confidence != c.conf {
			t.Errorf("%s: got %s/%s, want %s/%s", c.name, got.State, got.Confidence, c.want, c.conf)
		}
	}
}

func TestProposeAndApply(t *testing.T) {
	d := &jira.Discovery{
		Fields: []jira.Field{{ID: "customfield_10020", Schema: jira.FieldSchema{Custom: "com.pyxis.greenhopper.jira:gh-sprint"}}},
		Projects: []jira.ProjectDiscovery{{
			Project:    jira.Project{Key: "PLAT"},
			IssueTypes: []jira.IssueType{{ID: "1", Name: "Story"}, {ID: "2", Name: "Epic", HierarchyLevel: 1}},
			Statuses: []jira.StatusDef{
				{ID: "10", Name: "To Do", StatusCategory: "TODO"},
				{ID: "11", Name: "In Review", StatusCategory: "IN_PROGRESS"},
				{ID: "12", Name: "Done", StatusCategory: "DONE"},
			},
			Boards: []jira.BoardDiscovery{
				{Board: jira.Board{ID: 7, Type: "kanban"}},
				{Board: jira.Board{ID: 42, Type: "scrum"}, Config: boardWithColumns("10", "11", "12")},
			},
		}},
	}
	p := Propose(d)
	if p.Boards["PLAT"] != 42 {
		t.Errorf("must prefer the scrum board: %v", p.Boards)
	}
	if len(p.NeedsConfirmation) != 1 || p.NeedsConfirmation[0] != "In Review" {
		t.Errorf("needs confirmation = %v", p.NeedsConfirmation)
	}

	cfg := &config.Config{Projects: []config.Project{{Key: "PLAT"}}}
	p.Apply(cfg)
	if cfg.Projects[0].Board != 42 || cfg.Fields.Sprint != "customfield_10020" ||
		cfg.Workflow.States["In Review"] != config.StateInReview || len(cfg.Workflow.Order) != 3 || len(cfg.Hierarchy.Types) != 2 {
		t.Fatalf("applied config: %+v", cfg)
	}
}

func boardWithColumns(statusIDs ...string) *jira.BoardConfig {
	bc := &jira.BoardConfig{}
	for _, id := range statusIDs {
		col := jira.BoardColumn{Name: id}
		col.Statuses = append(col.Statuses, struct {
			ID string `json:"id"`
		}{ID: id})
		bc.ColumnConfig.Columns = append(bc.ColumnConfig.Columns, col)
	}
	return bc
}
