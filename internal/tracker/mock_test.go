package tracker

import "testing"

func TestCreateFollowsHierarchy(t *testing.T) {
	m := NewMock()

	epic, err := m.CreateIssue(NewIssue{Type: TypeEpic, Summary: "Canary releases", ParentKey: "INIT-7", Project: "PLAT"})
	if err != nil {
		t.Fatal(err)
	}
	story, err := m.CreateIssue(NewIssue{Type: TypeStory, Summary: "Canary metrics", ParentKey: epic.Key, Sprint: CurrentSprint})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := m.CreateIssue(NewIssue{Type: TypeSubtask, Summary: "Dashboard", ParentKey: story.Key})
	if err != nil {
		t.Fatal(err)
	}
	if sub.Sprint != CurrentSprint {
		t.Errorf("subtask sprint = %q, want parent's", sub.Sprint)
	}
	if p, _ := m.GetIssue(story.Key); len(p.ChildKeys) != 1 || p.ChildKeys[0] != sub.Key {
		t.Errorf("story children = %v", p.ChildKeys)
	}
	if epic.Key != "PLAT-153" || story.Key != "PLAT-154" {
		t.Errorf("keys = %s, %s; want next numbers in PLAT", epic.Key, story.Key)
	}

	for _, bad := range []NewIssue{
		{Type: TypeStory, Summary: "x", ParentKey: "INIT-7"},   // story under initiative
		{Type: TypeEpic, Summary: "x", ParentKey: "PLAT-100"},  // epic under epic
		{Type: TypeSubtask, Summary: "x"},                      // orphan subtask
		{Type: TypeSubtask, Summary: "x", ParentKey: sub.Key},  // subtask under subtask
		{Type: TypeTask, Summary: "  ", ParentKey: "PLAT-100"}, // empty summary
	} {
		if _, err := m.CreateIssue(bad); err == nil {
			t.Errorf("expected error for %+v", bad)
		}
	}
}

func TestLinksAreReciprocal(t *testing.T) {
	m := NewMock()
	if _, err := m.Link("PLAT-139", "blocks", "PLAT-147"); err != nil {
		t.Fatal(err)
	}
	b, _ := m.GetIssue("PLAT-147")
	if !hasLink(b, Link{"is blocked by", "PLAT-139"}) {
		t.Fatalf("missing reciprocal link: %v", b.Links)
	}
	if _, err := m.Unlink("PLAT-147", "is blocked by", "PLAT-139"); err != nil {
		t.Fatal(err)
	}
	a, _ := m.GetIssue("PLAT-139")
	if hasLink(a, Link{"blocks", "PLAT-147"}) {
		t.Fatalf("unlink should remove both sides: %v", a.Links)
	}
}

func hasLink(is Issue, l Link) bool {
	for _, x := range is.Links {
		if x == l {
			return true
		}
	}
	return false
}
