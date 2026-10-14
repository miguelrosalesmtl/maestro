package flow

import (
	"testing"

	"maestro/internal/tracker"
)

func liveEngine(t *testing.T) (*Engine, tracker.Tracker) {
	t.Helper()
	tr := tracker.NewMock()
	return NewEngineWith(tr, Options{Rules: DefaultRules()}), tr
}

func stageOf(f Flow, id string) Stage {
	for _, s := range f.Stages {
		if s.ID == id {
			return s
		}
	}
	return Stage{}
}

func TestApplyDevBaselineDoesNotFire(t *testing.T) {
	e, tr := liveEngine(t)
	// PLAT-143 is To Do. An old merged PR seen at startup is only shown.
	pr := &PullRequest{Number: 9, State: "merged", Base: "main", Approvals: 1, RequiredApprovals: 1}
	res, changed := e.ApplyDev("PLAT-143", DevState{Repo: "acme/app", Branch: "feature/PLAT-143", PR: pr}, false)
	if !changed || stageOf(res.Flow, "merged").State != StateDone || !res.Flow.Live || len(res.Events) != 0 {
		t.Fatalf("baseline: %+v", res)
	}
	if is, _ := tr.GetIssue("PLAT-143"); is.Status != tracker.StatusTodo {
		t.Fatalf("baseline must not move the ticket, got %q", is.Status)
	}
	if _, changed := e.ApplyDev("PLAT-143", DevState{Repo: "acme/app", Branch: "feature/PLAT-143", PR: pr}, true); changed {
		t.Fatal("same state again must be unchanged")
	}
}

func TestApplyDevFiresOnObservedChanges(t *testing.T) {
	e, tr := liveEngine(t)
	rules := e.Rules()
	rules[3].Enabled = true // "PR merged" → Done, off by default
	e.SetRules(rules)
	key := "PLAT-143"
	status := func() string { is, _ := tr.GetIssue(key); return is.Status }

	e.ApplyDev(key, DevState{}, false) // nothing at startup
	res, _ := e.ApplyDev(key, DevState{Repo: "acme/app", Branch: "feature/PLAT-143-x", Commits: []Commit{{SHA: "a1", Message: "PLAT-143: start"}}, Mine: true}, true)
	if status() != tracker.StatusInProgress || stageOf(res.Flow, "pr").State != StateActive {
		t.Fatalf("branch created: %s %+v", status(), res.Flow.Stages)
	}

	pr := PullRequest{Number: 12, Title: "PLAT-143", State: "open", Draft: true, Base: "main", RequiredApprovals: 2,
		Checks: []Check{{Name: "build", State: StateActive}}}
	e.ApplyDev(key, DevState{Branch: "feature/PLAT-143-x", PR: &pr, Mine: true}, true)
	if status() != tracker.StatusInProgress {
		t.Fatalf("a draft PR is not ready for review: %s", status())
	}
	pr.Draft = false
	res, _ = e.ApplyDev(key, DevState{Branch: "feature/PLAT-143-x", PR: &pr, Mine: true}, true)
	if status() != tracker.StatusInReview || stageOf(res.Flow, "ci").State != StateActive {
		t.Fatalf("ready for review: %s %+v", status(), res.Flow.Stages)
	}

	pr.Checks = []Check{{Name: "build", State: StateDone}}
	pr.ChangesRequested = []string{"ana"}
	res, _ = e.ApplyDev(key, DevState{Branch: "feature/PLAT-143-x", PR: &pr, Mine: true}, true)
	if status() != tracker.StatusInProgress || stageOf(res.Flow, "review").State != StateFailed || stageOf(res.Flow, "ci").State != StateDone {
		t.Fatalf("changes requested: %s %+v", status(), res.Flow.Stages)
	}

	pr.ChangesRequested, pr.Approvals, pr.ApprovedBy = nil, 2, []string{"ana", "bo"}
	res, _ = e.ApplyDev(key, DevState{Branch: "feature/PLAT-143-x", PR: &pr, Mine: true}, true)
	if stageOf(res.Flow, "review").State != StateDone || stageOf(res.Flow, "merged").State != StateActive {
		t.Fatalf("approved: %+v", res.Flow.Stages)
	}

	pr.State = "merged"
	res, _ = e.ApplyDev(key, DevState{Branch: "feature/PLAT-143-x", PR: &pr, Mine: true}, true)
	if status() != tracker.StatusDone || stageOf(res.Flow, "merged").State != StateDone {
		t.Fatalf("merged: %s %+v", status(), res.Flow.Stages)
	}
	if len(res.Events) == 0 || res.Events[0].Title != "PR #12 merged" {
		t.Fatalf("events: %+v", res.Events)
	}
}

func TestRequiredChecksDecide(t *testing.T) {
	st, _, ok := ciState([]Check{{Name: "lint", State: StateFailed}, {Name: "build", State: StateDone, Required: true}})
	if !ok || st != StateDone {
		t.Fatalf("only required checks count: %s", st)
	}
	st, _, _ = ciState([]Check{{Name: "lint", State: StateFailed}, {Name: "build", State: StateDone}})
	if st != StateFailed {
		t.Fatalf("without required checks all count: %s", st)
	}
	if _, _, ok := ciState(nil); ok {
		t.Fatal("no checks: nothing to judge")
	}
}

func TestLiveModeDoesNotSimulate(t *testing.T) {
	e, _ := liveEngine(t)
	if _, err := e.Advance("PLAT-143"); err == nil {
		t.Fatal("Advance must refuse in live mode")
	}
	res, ok := e.OnCommand("PLAT-143", "gh pr create --fill")
	if !ok || res.Flow.PR != nil {
		t.Fatalf("gh pr create must not invent a PR: %+v", res.Flow)
	}
}

// Regression (issue #1): with no events, Activity was nil and reached the UI
// as JSON null, which crashed the first live render.
func TestActivityEmptyIsNotNil(t *testing.T) {
	e, _ := liveEngine(t)
	if a := e.Activity(50); a == nil {
		t.Fatal("Activity() = nil, want an empty slice")
	}
	if r := NewEngineWith(tracker.NewMock(), Options{}).Rules(); r == nil {
		t.Fatal("Rules() = nil, want an empty slice")
	}
}
