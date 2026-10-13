package live

import (
	"slices"
	"testing"

	"maestro/internal/tracker"
	"maestro/internal/tracker/jira/jiratest"
)

func TestBrowseAnyBoardAndSprint(t *testing.T) {
	f := jiratest.New(t)
	tr := newTracker(t, f, nil)

	all, err := tr.SearchBoards("")
	if err != nil || len(all) != 2 {
		t.Fatalf("boards = %+v, %v", all, err)
	}
	found, _ := tr.SearchBoards("mobile")
	if len(found) != 1 || found[0].ID != 12 || found[0].Type != "kanban" || found[0].Project != "MOB" {
		t.Fatalf("search by name: %+v", found)
	}

	sprints, err := tr.BoardSprints(7)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range sprints {
		names = append(names, s.Name+":"+s.State)
	}
	// Active sprints first, then future, then recent closed.
	if len(names) != 4 || names[0] != "Sprint 42:active" && names[0] != "Hotfix sprint:active" || names[2] != "Sprint 43:future" || names[3] != "Sprint 41:closed" {
		t.Fatalf("sprints = %v", names)
	}
	if ks, err := tr.BoardSprints(12); err != nil || len(ks) != 0 {
		t.Fatalf("a kanban board has no sprints: %v %v", ks, err)
	}

	// Sprint 41 is closed; PLAT-2 was carried over from it, so it shows there.
	v, err := tr.BoardIssues(7, 41)
	if err != nil {
		t.Fatal(err)
	}
	if v.Sprint == nil || v.Sprint.Name != "Sprint 41" || v.Board.Name != "PLAT board" || len(v.Columns) != 3 {
		t.Fatalf("view = %+v", v)
	}
	if len(v.Issues) != 1 || v.Issues[0].Key != "PLAT-2" || !slices.Contains(v.Columns[0].Keys, "PLAT-2") {
		t.Fatalf("PLAT-2 (To Do) must be in the To Do column: %+v", v.Columns)
	}
	if !slices.Contains(v.Columns[0].Statuses, "To Do") {
		t.Fatalf("column statuses (drop target) = %v", v.Columns[0].Statuses)
	}

	// Showing a board does not add its issues to the synced set.
	before := len(tr.ListIssues())
	if _, err := tr.BoardIssues(12, 0); err != nil {
		t.Fatal(err)
	}
	if len(tr.ListIssues()) != before {
		t.Fatal("board issues must not be added to the synced set")
	}
	var _ tracker.BoardBrowser = tr
	var _ tracker.BoardBrowser = tracker.NewMock()
}
