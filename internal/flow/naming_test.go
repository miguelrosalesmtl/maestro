package flow

import (
	"encoding/json"
	"os"
	"testing"

	"maestro/internal/tracker"
)

// The same cases run against frontend/src/lib/naming.ts (frontend/tests/naming.test.ts).
func TestNamingCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/naming.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string
		Naming *Naming
		Issue  tracker.Issue
		Branch string
		Title  string
		Body   string
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		n := DefaultNaming()
		if c.Naming != nil {
			n = *c.Naming
		}
		branch := n.Branch(c.Issue)
		if branch != c.Branch {
			t.Errorf("%s: branch = %q, want %q", c.Name, branch, c.Branch)
		}
		if got := n.Title(c.Issue, branch); got != c.Title {
			t.Errorf("%s: title = %q, want %q", c.Name, got, c.Title)
		}
		if got := n.Body(c.Issue, branch); got != c.Body {
			t.Errorf("%s: body = %q, want %q", c.Name, got, c.Body)
		}
	}
}

func TestRuleScope(t *testing.T) {
	bug := tracker.Issue{Key: "PLAT-1", Type: tracker.TypeBug}
	story := tracker.Issue{Key: "MOB-2", Type: tracker.TypeStory}
	for _, c := range []struct {
		rule       Rule
		bug, story bool
	}{
		{Rule{}, true, true},
		{Rule{Projects: []string{"PLAT"}}, true, false},
		{Rule{IssueTypes: []string{"Story"}}, false, true},
		{Rule{Projects: []string{"PLAT", "MOB"}, IssueTypes: []string{"bug"}}, true, false},
	} {
		if c.rule.Applies(bug) != c.bug || c.rule.Applies(story) != c.story {
			t.Errorf("%+v: bug %v story %v, want %v %v", c.rule, c.rule.Applies(bug), c.rule.Applies(story), c.bug, c.story)
		}
	}
}
