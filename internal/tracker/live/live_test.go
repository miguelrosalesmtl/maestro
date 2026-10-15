package live

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"maestro/internal/config"
	"maestro/internal/secrets"
	"maestro/internal/tracker"
	"maestro/internal/tracker/jira"
	"maestro/internal/tracker/jira/jiratest"
)

func newTracker(t *testing.T, f *jiratest.Server, onChange func(Change)) *Tracker {
	c, err := jira.New(jira.Options{BaseURL: f.URL(), Deployment: jira.Cloud, HTTPClient: f.Client(),
		Auth: jira.BasicToken{Email: "me@x.com", Token: secrets.Secret("tok")}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Projects = []config.Project{{Key: "PLAT"}}
	cfg.Workflow.States = map[string]config.State{"Code Review": config.StateInReview}
	tr := New(Options{Client: c, Config: cfg, CacheDir: t.TempDir(), OnChange: onChange})
	if err := tr.discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := tr.fullSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestInitialSyncMapsHierarchyAndFields(t *testing.T) {
	f := jiratest.New(t)
	tr := newTracker(t, f, nil)

	got := map[string]tracker.Issue{}
	for _, is := range tr.ListIssues() {
		got[is.Key] = is
	}
	if _, ok := got["OPS-1"]; !ok {
		t.Fatal("out-of-scope parent epic must be fetched so the hierarchy is complete")
	}
	s := got["PLAT-2"]
	if s.Type != tracker.TypeStory || s.ParentKey != "PLAT-1" || s.StoryPoints != 5 || s.Sprint != "Sprint 42" || s.Description != "Hello" {
		t.Errorf("story mapped wrong: %+v", s)
	}
	if s.State != "todo" || s.StatusCategory != tracker.CategoryTodo || s.Assignee == nil || s.Assignee.ID != "me" {
		t.Errorf("status/assignee: %+v", s)
	}
	if !strings.HasSuffix(s.Created, "Z") {
		t.Errorf("dates must be RFC 3339: %q", s.Created)
	}
	if strings.Join(got["PLAT-1"].ChildKeys, ",") != "PLAT-2" || strings.Join(got["OPS-1"].ChildKeys, ",") != "PLAT-1" {
		t.Errorf("child keys: PLAT-1=%v OPS-1=%v", got["PLAT-1"].ChildKeys, got["OPS-1"].ChildKeys)
	}
	if got["PLAT-3"].Type != tracker.TypeSubtask || got["PLAT-3"].State != "done" {
		t.Errorf("sub-task: %+v", got["PLAT-3"])
	}

	m := tr.Meta()
	if !m.Live || m.Sprint != "Sprint 42" || strings.Join(m.Statuses, ",") != "To Do,In Progress,Code Review,Done" {
		t.Errorf("meta: %+v", m)
	}
	if m.StatusStates["Code Review"] != "inReview" || m.LinkTypes["blocks"] != "is blocked by" {
		t.Errorf("meta states/links: %+v", m)
	}
	if len(tr.Users()) != 2 || tr.CurrentUser().ID != "me" {
		t.Errorf("users: %+v", tr.Users())
	}
}

func TestIncrementalSyncReportsStatusChangesMadeInJira(t *testing.T) {
	f := jiratest.New(t)
	var changes []Change
	tr := newTracker(t, f, func(c Change) { changes = append(changes, c) })

	f.SetStatus("PLAT-2", "In Progress")
	f.MarkRecent("PLAT-1") // unchanged: the overlap window returns it again

	changes = nil
	if err := tr.incrementalSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || len(changes[0].Issues) != 1 || changes[0].Issues[0].Key != "PLAT-2" {
		t.Fatalf("want only PLAT-2 changed, got %+v", changes)
	}
	sc := changes[0].StatusChanges
	if len(sc) != 1 || sc[0].From != "To Do" || sc[0].To != "In Progress" {
		t.Fatalf("status change = %+v", sc)
	}
}

func TestTransitionByStatusStateAndRequiredResolution(t *testing.T) {
	f := jiratest.New(t)
	tr := newTracker(t, f, nil)

	is, err := tr.Transition("PLAT-2", "In Progress") // by status name
	if err != nil || is.Status != "In Progress" {
		t.Fatalf("by name: %+v %v", is, err)
	}
	is, err = tr.Transition("PLAT-2", "inReview") // by Maestro state (automation rules)
	if err != nil || is.Status != "Code Review" {
		t.Fatalf("by state: %+v %v", is, err)
	}
	is, err = tr.Transition("PLAT-2", "Done") // screen requires a resolution
	if err != nil || is.Status != "Done" || is.State != "done" {
		t.Fatalf("with resolution: %+v %v", is, err)
	}
	if _, err := tr.Transition("PLAT-2", "Released"); err == nil {
		t.Fatal("unknown target must fail with a clear error")
	}
}

func TestCreateIssueResolvesJiraIDs(t *testing.T) {
	f := jiratest.New(t)
	tr := newTracker(t, f, nil)

	is, err := tr.CreateIssue(tracker.NewIssue{Type: tracker.TypeStory, Summary: "Canary", ParentKey: "PLAT-1",
		Priority: "High", AssigneeID: "ana", StoryPoints: 3, Sprint: "Sprint 42"})
	if err != nil {
		t.Fatal(err)
	}
	body := f.Created[0]
	want := map[string]string{
		"project": `{"key":"PLAT"}`, "issuetype": `{"id":"10001"}`, "priority": `{"id":"2"}`,
		"assignee": `{"accountId":"ana"}`, "parent": `{"key":"PLAT-1"}`, "customfield_10016": "3", "customfield_10020": "42",
	}
	for k, v := range want {
		if b, _ := json.Marshal(body[k]); string(b) != v {
			t.Errorf("payload %s = %s, want %s", k, b, v)
		}
	}
	if is.ParentKey != "PLAT-1" {
		t.Errorf("created issue: %+v", is)
	}
	for _, p := range tr.ListIssues() {
		if p.Key == "PLAT-1" && !strings.Contains(strings.Join(p.ChildKeys, ","), is.Key) {
			t.Errorf("parent must list the new child: %v", p.ChildKeys)
		}
	}

	if _, err := tr.CreateIssue(tracker.NewIssue{Type: tracker.TypeBug, Summary: "x", ParentKey: "PLAT-1"}); err == nil ||
		!strings.Contains(err.Error(), "no issue type") {
		t.Errorf("a missing issue type must explain the fix: %v", err)
	}
}

func TestLinkDirectionIsVerifiedAndLearned(t *testing.T) {
	for _, serverSwaps := range []bool{false, true} {
		t.Run(fmt.Sprintf("serverSwaps=%v", serverSwaps), func(t *testing.T) {
			f := jiratest.New(t)
			f.LinkSwaps = serverSwaps
			tr := newTracker(t, f, nil)

			out, err := tr.Link("PLAT-2", "blocks", "PLAT-3")
			if err != nil {
				t.Fatal(err)
			}
			if !hasLink(out[0], "blocks", "PLAT-3") || !hasLink(out[1], "is blocked by", "PLAT-2") {
				t.Fatalf("PLAT-2 must block PLAT-3: %+v / %+v", out[0].Links, out[1].Links)
			}
			if len(f.Links) != 1 {
				t.Fatalf("a reversed link must be replaced, not duplicated: %+v", f.Links)
			}
			// The learned convention is used directly next time.
			if _, err := tr.Link("PLAT-1", "is blocked by", "PLAT-3"); err != nil {
				t.Fatal(err)
			}
			if l := f.Links[len(f.Links)-1]; l.Outward != "PLAT-3" || l.Inward != "PLAT-1" {
				t.Fatalf("second link wrong: %+v", l)
			}
			out, err = tr.Unlink("PLAT-3", "is blocked by", "PLAT-2")
			if err != nil || hasLink(out[0], "is blocked by", "PLAT-2") {
				t.Fatalf("unlink: %+v %v", out, err)
			}
		})
	}
}

func TestGetIssueLoadsComments(t *testing.T) {
	f := jiratest.New(t)
	tr := newTracker(t, f, nil)
	is, err := tr.GetIssue("PLAT-2")
	if err != nil || len(is.Comments) != 1 || is.Comments[0].Body != "plain text" || is.Comments[0].Author.Name != "Ana Torres" {
		t.Fatalf("comments: %+v %v", is.Comments, err)
	}
	f.Delete("PLAT-3")
	if _, err := tr.GetIssue("PLAT-3"); err == nil {
		t.Fatal("deleted issue must error")
	}
	for _, i := range tr.ListIssues() {
		if i.Key == "PLAT-3" {
			t.Fatal("a 404 must drop the issue locally")
		}
	}
}

func hasLink(is tracker.Issue, typ, key string) bool {
	for _, l := range is.Links {
		if l.Type == typ && l.Key == key {
			return true
		}
	}
	return false
}

// A ticket created in Jira shows after Refresh, without waiting for polling;
// one in another team's project counts when it is assigned to me.
func TestRefreshPicksUpNewTickets(t *testing.T) {
	f := jiratest.New(t)
	tr := newTracker(t, f, nil)
	has := func(key string) bool {
		for _, is := range tr.ListIssues() {
			if is.Key == key {
				return true
			}
		}
		return false
	}
	if !has("MOB-7") {
		t.Fatal("an issue assigned to me in another project must be synced")
	}
	if has("OPS-1") && !tr.issues["OPS-1"].ancestor {
		t.Fatal("OPS-1 is assigned to someone else: only kept as a parent")
	}

	f.Add("PLAT-50", "Created in Jira just now", "To Do")
	if has("PLAT-50") {
		t.Fatal("precondition: not synced yet")
	}
	if err := tr.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !has("PLAT-50") {
		t.Fatal("Refresh must pick up a ticket created in Jira")
	}
}

func TestTicketCreatedUnderAnEpicShowsUpInItsTree(t *testing.T) {
	f := jiratest.New(t)
	var changes []Change
	tr := newTracker(t, f, func(c Change) { changes = append(changes, c) })

	f.AddChild("PLAT-60", "Created under the epic in Jira", "PLAT-1")
	changes = nil
	if err := tr.incrementalSync(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := map[string]tracker.Issue{}
	for _, c := range changes {
		for _, is := range c.Issues {
			got[is.Key] = is
		}
	}
	if _, ok := got["PLAT-60"]; !ok {
		t.Fatalf("the new ticket must be sent, got %v", got)
	}
	if epic, ok := got["PLAT-1"]; !ok || !slices.Contains(epic.ChildKeys, "PLAT-60") {
		t.Fatalf("the epic must be sent with its new child, got %+v", got["PLAT-1"])
	}
}
