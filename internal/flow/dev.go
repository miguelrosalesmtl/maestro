package flow

import (
	"fmt"
	"strings"
	"time"
)

// DevState is the real development state of one ticket, put together by the
// dev watcher from GitHub and the local git clone.
type DevState struct {
	Repo    string
	Branch  string       // PR head branch, else a local branch with the key
	Commits []Commit     // PR commits, else local commits not on the base branch
	PR      *PullRequest // nil when there is no pull request
	// Mine is true for the user's own work: a local branch without a pull
	// request, or a pull request the user opened. Others' work fires no rules
	// unless automation.anyAuthor is set.
	Mine bool
}

// ciState sums up the checks that count: the required ones when any are
// required, else all of them. ok is false when there is nothing to judge.
func ciState(checks []Check) (st StageState, detail string, ok bool) {
	var counted []Check
	for _, c := range checks {
		if c.Required {
			counted = append(counted, c)
		}
	}
	if len(counted) == 0 {
		counted = checks
	}
	if len(counted) == 0 {
		return "", "", false
	}
	var done, failed, running []string
	for _, c := range counted {
		switch c.State {
		case StateDone:
			done = append(done, c.Name)
		case StateFailed:
			failed = append(failed, c.Name)
		default:
			running = append(running, c.Name)
		}
	}
	switch {
	case len(failed) > 0:
		return StateFailed, "failed: " + strings.Join(failed, ", "), true
	case len(running) > 0:
		return StateActive, fmt.Sprintf("%d/%d checks passed · %d running", len(done), len(counted), len(running)), true
	}
	return StateDone, fmt.Sprintf("%d/%d checks passed", len(done), len(counted)), true
}

func reviewState(pr *PullRequest) (StageState, string) {
	need := max(pr.RequiredApprovals, 1)
	switch {
	case len(pr.ChangesRequested) > 0:
		return StateFailed, "changes requested by " + strings.Join(pr.ChangesRequested, ", ")
	case pr.Approvals >= need:
		return StateDone, fmt.Sprintf("%d of %d approvals", pr.Approvals, need)
	}
	return StateActive, fmt.Sprintf("%d of %d approvals", pr.Approvals, need)
}

// stagesFor derives every stage from the real state. Times of stages that do
// not change are kept from prev.
func stagesFor(ds DevState, prev []Stage, started bool, startedBranch string) []Stage {
	st := blankStages()
	now := time.Now().UTC().Format(time.RFC3339)
	set := func(id string, s StageState, detail string) {
		for i := range st {
			if st[i].ID != id {
				continue
			}
			st[i].State, st[i].Detail = s, detail
			for _, p := range prev {
				if p.ID == id && p.State == s {
					st[i].At = p.At
				}
			}
			if st[i].At == "" && s != StatePending {
				st[i].At = now
			}
		}
	}

	branch := ds.Branch
	if branch == "" && started {
		branch = startedBranch
	}
	if branch == "" && ds.PR == nil {
		return st
	}
	set("branch", StateDone, branch)

	pr := ds.PR
	switch {
	case pr != nil:
		set("commits", StateDone, plural(len(ds.Commits), "commit"))
	case len(ds.Commits) > 0:
		set("commits", StateDone, plural(len(ds.Commits), "commit"))
		set("pr", StateActive, "no pull request yet")
		return st
	default:
		set("commits", StateActive, "no commits yet")
		return st
	}

	switch {
	case pr.State == "closed":
		set("pr", StateFailed, fmt.Sprintf("#%d closed without merging", pr.Number))
		return st
	case pr.Draft:
		set("pr", StateActive, fmt.Sprintf("#%d draft", pr.Number))
	default:
		set("pr", StateDone, fmt.Sprintf("#%d opened", pr.Number))
	}

	ci, ciDetail, hasCI := ciState(pr.Checks)
	if !hasCI {
		ci, ciDetail = StateDone, "no checks"
	}
	set("ci", ci, ciDetail)
	rv, rvDetail := reviewState(pr)
	set("review", rv, rvDetail)

	if pr.State == "merged" {
		set("merged", StateDone, "merged into "+firstNonEmpty(pr.Base, "base branch"))
	} else if ci == StateDone && rv == StateDone && !pr.Draft {
		set("merged", StateActive, "ready to merge")
	}
	return st
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ApplyDev replaces a ticket's flow with the real dev state. When fire is
// true, the changes since the previous state are dev events: they go on the
// timeline and run the automation rules. With fire false (the first look at
// startup) the state is only shown, so an old merged PR never moves a
// reopened ticket. changed is false when nothing visible changed.
func (e *Engine) ApplyDev(key string, ds DevState, fire bool) (res StepResult, changed bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	f := e.flowFor(key)
	before := *f
	beforeStages := append([]Stage(nil), f.Stages...)

	f.Live = true
	if ds.Repo != "" {
		f.Repo = ds.Repo
	}
	if ds.Branch != "" {
		f.Branch = ds.Branch
	}
	f.Started = f.Started || ds.Branch != "" || ds.PR != nil
	f.Commits = append([]Commit{}, ds.Commits...)
	if ds.PR != nil {
		pr := *ds.PR
		pr.Live = true
		f.PR = &pr
	} else {
		f.PR = nil
	}
	f.Stages = stagesFor(ds, beforeStages, before.Started, before.Branch)

	changed = !sameFlow(&before, f)
	if !changed {
		return StepResult{Flow: *f, Events: []Event{}}, false // no tracker read
	}
	if !fire {
		r, _ := e.result(f, nil)
		return r, changed
	}

	var evs []Event
	stage := func(fl *Flow, id string) StageState {
		for _, s := range fl.Stages {
			if s.ID == id {
				return s.State
			}
		}
		return StatePending
	}
	beforeFlow := before
	beforeFlow.Stages = beforeStages

	if before.Branch == "" && ds.Branch != "" {
		evs = append(evs, e.emit(key, "dev", "Branch created", ds.Branch, ""))
		evs = append(evs, e.fire(key, TriggerBranchCreated, ds.Mine)...)
	}
	if n, m := len(before.Commits), len(f.Commits); m > n {
		last := f.Commits[len(f.Commits)-1]
		evs = append(evs, e.emit(key, "dev", fmt.Sprintf("Pushed %s", plural(m-n, "commit")), last.Message, ""))
	}
	if pr := f.PR; pr != nil {
		prevPR := before.PR
		samePR := prevPR != nil && prevPR.Number == pr.Number && prevPR.Repo == pr.Repo
		wasReady := samePR && prevPR.State == "open" && !prevPR.Draft
		if pr.State == "open" && !pr.Draft && !wasReady && !(samePR && prevPR.State == "merged") {
			title := fmt.Sprintf("PR #%d opened", pr.Number)
			if samePR && prevPR.Draft {
				title = fmt.Sprintf("PR #%d ready for review", pr.Number)
			}
			evs = append(evs, e.emit(key, "dev", title, pr.Title, ""))
			evs = append(evs, e.fire(key, TriggerPROpened, ds.Mine)...)
		}
		if ci := stage(f, "ci"); ci != stage(&beforeFlow, "ci") && pr.State == "open" {
			switch ci {
			case StateFailed:
				evs = append(evs, e.emit(key, "dev", "Checks failed", f.stage("ci").Detail, ""))
				evs = append(evs, e.fire(key, TriggerCIFailed, ds.Mine)...)
			case StateDone:
				if samePR {
					evs = append(evs, e.emit(key, "dev", "Checks passed", f.stage("ci").Detail, ""))
				}
			}
		}
		if rv := stage(f, "review"); rv != stage(&beforeFlow, "review") && pr.State == "open" {
			switch rv {
			case StateFailed:
				evs = append(evs, e.emit(key, "dev", "Changes requested", strings.Join(pr.ChangesRequested, ", "), ""))
				evs = append(evs, e.fire(key, TriggerChangesRequested, ds.Mine)...)
			case StateDone:
				evs = append(evs, e.emit(key, "dev", fmt.Sprintf("PR #%d approved", pr.Number), strings.Join(pr.ApprovedBy, ", ")+" approved", ""))
				evs = append(evs, e.fire(key, TriggerReviewApproved, ds.Mine)...)
			}
		}
		if pr.State == "merged" && !(samePR && prevPR.State == "merged") {
			evs = append(evs, e.emit(key, "dev", fmt.Sprintf("PR #%d merged", pr.Number), "merged into "+firstNonEmpty(pr.Base, "base branch"), ""))
			evs = append(evs, e.fire(key, TriggerPRMerged, ds.Mine)...)
		}
		if pr.State == "closed" && !(samePR && prevPR.State == "closed") {
			evs = append(evs, e.emit(key, "dev", fmt.Sprintf("PR #%d closed", pr.Number), "closed without merging", ""))
		}
	}
	r, _ := e.result(f, evs)
	return r, true
}

// sameFlow compares what the UI shows (stage times aside).
func sameFlow(a, b *Flow) bool {
	if a.Branch != b.Branch || a.Live != b.Live || len(a.Commits) != len(b.Commits) || len(a.Stages) != len(b.Stages) {
		return false
	}
	for i := range a.Commits {
		if a.Commits[i] != b.Commits[i] {
			return false
		}
	}
	for i := range a.Stages {
		if a.Stages[i].State != b.Stages[i].State || a.Stages[i].Detail != b.Stages[i].Detail {
			return false
		}
	}
	return samePR(a.PR, b.PR)
}

func samePR(a, b *PullRequest) bool {
	if a == nil || b == nil {
		return a == b
	}
	return fmt.Sprintf("%+v", *a) == fmt.Sprintf("%+v", *b)
}
