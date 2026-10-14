package flow

import (
	"strings"
	"testing"

	"maestro/internal/tracker"
)

// Regression (issue #4): rules moved teammates' tickets and re-closed Done ones.

func mergedPR(author string) *PullRequest {
	return &PullRequest{Number: 31, State: "merged", Author: author, Base: "main", Approvals: 1, RequiredApprovals: 1}
}

// observe shows a ticket with nothing, then a branch, then a merged PR.
func observe(e *Engine, key string, mine bool) []Event {
	e.ApplyDev(key, DevState{}, false)
	r1, _ := e.ApplyDev(key, DevState{Repo: "acme/app", Branch: "feature/" + key, Mine: mine}, true)
	r2, _ := e.ApplyDev(key, DevState{Repo: "acme/app", Branch: "feature/" + key, PR: mergedPR("someone"), Mine: mine}, true)
	return append(r1.Events, r2.Events...)
}

func allRulesOn(e *Engine) {
	rules := e.Rules()
	for i := range rules {
		rules[i].Enabled = true
	}
	e.SetRules(rules)
}

func automations(evs []Event) []string {
	var out []string
	for _, ev := range evs {
		if ev.Kind == "automation" {
			out = append(out, ev.Title)
		}
	}
	return out
}

func TestTeammatesWorkMovesNothing(t *testing.T) {
	e, tr := liveEngine(t)
	allRulesOn(e)
	// PLAT-121 is To Do and assigned to a teammate; they branch and merge.
	if evs := observe(e, "PLAT-121", false); len(automations(evs)) != 0 {
		t.Fatalf("teammate's ticket moved: %v", automations(evs))
	}
	if is, _ := tr.GetIssue("PLAT-121"); is.Status != tracker.StatusTodo {
		t.Fatalf("PLAT-121 = %q", is.Status)
	}
	// Even with any author allowed, the ticket is not the user's.
	e2, tr2 := liveEngine(t)
	allRulesOn(e2)
	e2.SetPolicy(Policy{AnyAuthor: true})
	if evs := observe(e2, "PLAT-121", false); len(automations(evs)) != 0 {
		t.Fatalf("anyAuthor reached a ticket assigned to someone else: %v", automations(evs))
	}
	if is, _ := tr2.GetIssue("PLAT-121"); is.Status != tracker.StatusTodo {
		t.Fatalf("PLAT-121 = %q", is.Status)
	}
}

func TestSomeoneElsesPRDoesNotMoveMyTicket(t *testing.T) {
	e, tr := liveEngine(t)
	allRulesOn(e)
	key := "PLAT-143" // To Do, assigned to the user
	e.ApplyDev(key, DevState{}, false)
	e.ApplyDev(key, DevState{Repo: "acme/app", Branch: "fix/PLAT-143", PR: mergedPR("teammate")}, true)
	if is, _ := tr.GetIssue(key); is.Status != tracker.StatusTodo {
		t.Fatalf("someone else's merge moved the user's ticket to %q", is.Status)
	}

	e2, tr2 := liveEngine(t)
	allRulesOn(e2)
	e2.SetPolicy(Policy{AnyAuthor: true})
	e2.ApplyDev(key, DevState{}, false)
	e2.ApplyDev(key, DevState{Repo: "acme/app", Branch: "fix/PLAT-143", PR: mergedPR("teammate")}, true)
	if is, _ := tr2.GetIssue(key); is.Status != tracker.StatusDone {
		t.Fatalf("anyAuthor: status = %q, want Done", is.Status)
	}
}

func TestDoneTicketIsNeverReopened(t *testing.T) {
	e, tr := liveEngine(t)
	allRulesOn(e)
	key := "PLAT-150" // Done, assigned to the user
	if evs := observe(e, key, true); len(automations(evs)) != 0 {
		t.Fatalf("rules fired on a Done ticket: %v", automations(evs))
	}
	if is, _ := tr.GetIssue(key); is.Status != tracker.StatusDone {
		t.Fatalf("status = %q", is.Status)
	}
	// Starting from the terminal does not reopen it either.
	if res, _ := e.Start(key); res.Issue.Status != tracker.StatusDone {
		t.Fatalf("Start reopened a Done ticket: %q", res.Issue.Status)
	}
}

func TestDryRunLeavesJiraAlone(t *testing.T) {
	e, tr := liveEngine(t)
	e.SetPolicy(Policy{DryRun: true})
	e.ApplyDev("PLAT-143", DevState{}, false)
	res, _ := e.ApplyDev("PLAT-143", DevState{Repo: "acme/app", Branch: "feature/PLAT-143", Mine: true}, true)
	if got := automations(res.Events); len(got) != 1 || got[0] != "Would move To Do → In Progress" {
		t.Fatalf("dry run events: %v", got)
	}
	if is, _ := tr.GetIssue("PLAT-143"); is.Status != tracker.StatusTodo {
		t.Fatalf("dry run moved the ticket to %q", is.Status)
	}
}

func TestOwnWorkOnUnassignedTicketSaysWhy(t *testing.T) {
	e, _ := liveEngine(t)
	res, _ := e.Start("PLAT-121") // the user branches off a teammate's ticket
	var said bool
	for _, ev := range res.Events {
		said = said || (ev.Kind == "automation" && strings.Contains(ev.Detail, "not assigned to you"))
	}
	if !said || res.Issue.Status != tracker.StatusTodo {
		t.Fatalf("events %+v, status %q", res.Events, res.Issue.Status)
	}
}
