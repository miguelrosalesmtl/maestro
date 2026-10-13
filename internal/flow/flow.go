// Package flow tracks the developer workflow (branch → PR → merge) for each
// ticket and applies automation rules that move the ticket in the tracker.
//
// With the mock tracker, dev events are simulated (Advance). With a live
// tracker, ApplyDev receives the real state from the GitHub watcher and the
// local git scan.
package flow

import (
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"maestro/internal/tracker"
)

type StageState string

const (
	StatePending StageState = "pending"
	StateActive  StageState = "active"
	StateDone    StageState = "done"
	StateFailed  StageState = "failed"
)

// Trigger names a dev event that automation rules can react to.
type Trigger string

const (
	TriggerBranchCreated    Trigger = "branch_created"
	TriggerPROpened         Trigger = "pr_opened"
	TriggerCIFailed         Trigger = "ci_failed"
	TriggerChangesRequested Trigger = "changes_requested"
	TriggerReviewApproved   Trigger = "review_approved"
	TriggerPRMerged         Trigger = "pr_merged"
)

type Stage struct {
	ID      string     `json:"id"`
	Label   string     `json:"label"`
	State   StageState `json:"state"`
	Detail  string     `json:"detail"`
	At      string     `json:"at"`
	Trigger Trigger    `json:"trigger"`
}

type Commit struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
	Author  string `json:"author"`
	At      string `json:"at"`
	Adds    int    `json:"adds"`
	Dels    int    `json:"dels"`
}

type Check struct {
	Name     string     `json:"name"`
	State    StageState `json:"state"` // active = running or queued
	URL      string     `json:"url,omitempty"`
	Required bool       `json:"required,omitempty"`
}

type PullRequest struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	State     string   `json:"state"` // open | merged | closed
	Checks    []Check  `json:"checks"`
	Approvals int      `json:"approvals"`
	Reviewers []string `json:"reviewers"`

	Repo              string   `json:"repo,omitempty"`
	Draft             bool     `json:"draft,omitempty"`
	Author            string   `json:"author,omitempty"`
	Head              string   `json:"head,omitempty"`
	Base              string   `json:"base,omitempty"`
	RequiredApprovals int      `json:"requiredApprovals,omitempty"`
	ApprovedBy        []string `json:"approvedBy,omitempty"`
	ChangesRequested  []string `json:"changesRequested,omitempty"`
	MergedAt          string   `json:"mergedAt,omitempty"`
	UpdatedAt         string   `json:"updatedAt,omitempty"`
	Live              bool     `json:"live,omitempty"` // from GitHub, not simulated
}

type Flow struct {
	IssueKey string       `json:"issueKey"`
	Repo     string       `json:"repo"`
	Branch   string       `json:"branch"`
	Started  bool         `json:"started"`
	Stages   []Stage      `json:"stages"`
	Commits  []Commit     `json:"commits"`
	PR       *PullRequest `json:"pr"`
	// Live is true when the stages come from GitHub and git, not the demo.
	Live bool `json:"live"`
}

type Event struct {
	ID       string `json:"id"`
	IssueKey string `json:"issueKey"`
	Kind     string `json:"kind"` // dev | jira | automation
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	At       string `json:"at"`
}

type Rule struct {
	ID       string  `json:"id"`
	Trigger  Trigger `json:"trigger"`
	Label    string  `json:"label"`
	ToStatus string  `json:"toStatus"`
	Enabled  bool    `json:"enabled"`
}

// StepResult is what the UI receives after any flow mutation.
type StepResult struct {
	Flow   Flow          `json:"flow"`
	Issue  tracker.Issue `json:"issue"`
	Events []Event       `json:"events"`
}

type Engine struct {
	mu     sync.Mutex
	tr     tracker.Tracker
	flows  map[string]*Flow
	rules  []Rule
	events []Event
	seq    int
	repo   string
	sim    bool
	poke   func(key string)
}

// DefaultRules are the demo rules (status names of the mock tracker).
func DefaultRules() []Rule {
	return []Rule{
		{ID: "r1", Trigger: TriggerBranchCreated, Label: "Branch created", ToStatus: tracker.StatusInProgress, Enabled: true},
		{ID: "r2", Trigger: TriggerPROpened, Label: "Pull request opened", ToStatus: tracker.StatusInReview, Enabled: true},
		{ID: "r3", Trigger: TriggerChangesRequested, Label: "Changes requested", ToStatus: tracker.StatusInProgress, Enabled: true},
		{ID: "r4", Trigger: TriggerPRMerged, Label: "Pull request merged", ToStatus: tracker.StatusDone, Enabled: true},
		{ID: "r5", Trigger: TriggerCIFailed, Label: "CI failed", ToStatus: tracker.StatusBlocked, Enabled: false},
	}
}

// TriggerLabel is the human name of a trigger.
func TriggerLabel(t Trigger) string {
	return map[Trigger]string{
		TriggerBranchCreated: "Branch created", TriggerPROpened: "Pull request opened",
		TriggerChangesRequested: "Changes requested", TriggerReviewApproved: "Review approved",
		TriggerPRMerged: "Pull request merged", TriggerCIFailed: "CI failed",
	}[t]
}

type Options struct {
	Rules []Rule
	Repo  string // shown in the code panel
	Seed  bool   // demo flows and events (mock tracker only)
	// Simulate lets terminal commands and Advance fake the next dev event
	// (demo). Off with a live tracker: real events come from ApplyDev.
	Simulate bool
	// Poke asks the dev watcher to look again soon (after git push, gh pr …).
	Poke func(key string)
}

// NewEngine is the demo engine used with the mock tracker.
func NewEngine(tr tracker.Tracker) *Engine {
	return NewEngineWith(tr, Options{Rules: DefaultRules(), Repo: "acme/platform", Seed: true, Simulate: true})
}

func NewEngineWith(tr tracker.Tracker, o Options) *Engine {
	e := &Engine{tr: tr, flows: map[string]*Flow{}, repo: o.Repo, rules: append([]Rule(nil), o.Rules...), sim: o.Simulate, poke: o.Poke}
	if o.Seed {
		e.seed()
	}
	return e
}

func blankStages() []Stage {
	return []Stage{
		{ID: "branch", Label: "Branch", State: StatePending, Trigger: TriggerBranchCreated},
		{ID: "commits", Label: "Commits", State: StatePending},
		{ID: "pr", Label: "Pull Request", State: StatePending, Trigger: TriggerPROpened},
		{ID: "ci", Label: "Checks", State: StatePending, Trigger: TriggerCIFailed},
		{ID: "review", Label: "Review", State: StatePending, Trigger: TriggerReviewApproved},
		{ID: "merged", Label: "Merged", State: StatePending, Trigger: TriggerPRMerged},
	}
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// BranchName derives a git branch from the ticket, e.g. feature/PLAT-142-health-gated-traffic.
// A secured issue's summary never goes into a branch name.
func BranchName(is tracker.Issue) string {
	prefix := "feature"
	if is.Type == tracker.TypeBug {
		prefix = "fix"
	}
	if is.Security != "" {
		return prefix + "/" + is.Key
	}
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(is.Summary), "-"), "-")
	if parts := strings.Split(slug, "-"); len(parts) > 5 {
		slug = strings.Join(parts[:5], "-")
	}
	return fmt.Sprintf("%s/%s-%s", prefix, is.Key, slug)
}

func ts(d time.Duration) string { return time.Now().Add(-d).UTC().Format(time.RFC3339) }

func (e *Engine) flowFor(key string) *Flow {
	f, ok := e.flows[key]
	if !ok {
		f = &Flow{IssueKey: key, Repo: e.repo, Stages: blankStages(), Commits: []Commit{}}
		e.flows[key] = f
	}
	return f
}

func (e *Engine) Flow(key string) Flow {
	e.mu.Lock()
	defer e.mu.Unlock()
	return *e.flowFor(key)
}

func (e *Engine) Rules() []Rule {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]Rule(nil), e.rules...)
}

func (e *Engine) ToggleRule(id string) []Rule {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.rules {
		if e.rules[i].ID == id {
			e.rules[i].Enabled = !e.rules[i].Enabled
		}
	}
	return append([]Rule(nil), e.rules...)
}

func (e *Engine) SetRuleStatus(id, status string) []Rule {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.rules {
		if e.rules[i].ID == id {
			e.rules[i].ToStatus = status
		}
	}
	return append([]Rule(nil), e.rules...)
}

// Activity returns the global event feed, newest first.
func (e *Engine) Activity(limit int) []Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := append([]Event(nil), e.events...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].At > out[j].At })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// RecordTransition logs a manual status change made from the UI.
func (e *Engine) RecordTransition(key, from, to string) Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.emit(key, "jira", fmt.Sprintf("%s → %s", from, to), "Status changed manually", "")
}

// Record logs an arbitrary event on a ticket's timeline.
func (e *Engine) Record(key, kind, title, detail string) Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.emit(key, kind, title, detail, "")
}

func (e *Engine) emit(key, kind, title, detail, at string) Event {
	e.seq++
	if at == "" {
		at = time.Now().UTC().Format(time.RFC3339Nano)
	}
	ev := Event{ID: fmt.Sprintf("ev-%d", e.seq), IssueKey: key, Kind: kind, Title: title, Detail: detail, At: at}
	e.events = append(e.events, ev)
	return ev
}

// fire applies every enabled rule for trigger and returns the events produced.
func (e *Engine) fire(key string, trig Trigger) []Event {
	var out []Event
	for _, r := range e.rules {
		if !r.Enabled || r.Trigger != trig {
			continue
		}
		is, err := e.tr.GetIssue(key)
		if err != nil || is.Status == r.ToStatus {
			continue
		}
		if _, err := e.tr.Transition(key, r.ToStatus); err != nil {
			slog.Warn("automation failed", "component", "flow", "key", key, "rule", r.ID, "trigger", string(trig), "to", r.ToStatus, "err", err.Error())
			continue
		}
		slog.Info("automation fired", "component", "flow", "key", key, "rule", r.ID, "trigger", string(trig), "from", is.Status, "to", r.ToStatus)
		out = append(out, e.emit(key, "automation",
			fmt.Sprintf("%s → %s", is.Status, r.ToStatus),
			fmt.Sprintf("Rule “%s” fired", r.Label), ""))
	}
	return out
}

func (f *Flow) stage(id string) *Stage {
	for i := range f.Stages {
		if f.Stages[i].ID == id {
			return &f.Stages[i]
		}
	}
	return nil
}

func (f *Flow) complete(id, detail string) {
	s := f.stage(id)
	s.State, s.Detail, s.At = StateDone, detail, time.Now().UTC().Format(time.RFC3339)
	for i := range f.Stages {
		if f.Stages[i].ID == id && i+1 < len(f.Stages) {
			f.Stages[i+1].State = StateActive
		}
	}
}

func (e *Engine) result(f *Flow, evs []Event) (StepResult, error) {
	is, err := e.tr.GetIssue(f.IssueKey)
	if evs == nil {
		evs = []Event{}
	}
	return StepResult{Flow: *f, Issue: is, Events: evs}, err
}

// Start begins work on a ticket: creates the branch and fires branch_created.
func (e *Engine) Start(key string) (StepResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.start(key)
}

func (e *Engine) start(key string) (StepResult, error) {
	is, err := e.tr.GetIssue(key)
	if err != nil {
		return StepResult{}, err
	}
	f := e.flowFor(key)
	if f.Started {
		return e.result(f, nil)
	}
	f.Started = true
	f.Branch = BranchName(is)
	f.complete("branch", f.Branch)
	evs := []Event{e.emit(key, "dev", "Branch created", f.Branch, "")}
	evs = append(evs, e.fire(key, TriggerBranchCreated)...)
	return e.result(f, evs)
}

// Advance simulates the next dev event for the ticket (demo only).
func (e *Engine) Advance(key string) (StepResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.advance(key)
}

func (e *Engine) advance(key string) (StepResult, error) {
	if !e.sim {
		return StepResult{}, fmt.Errorf("simulation is only available in demo mode")
	}
	is, err := e.tr.GetIssue(key)
	if err != nil {
		return StepResult{}, err
	}
	f := e.flowFor(key)
	if !f.Started {
		return e.start(key)
	}
	var evs []Event
	switch {
	case f.stage("commits").State == StateActive:
		n := len(f.Commits)
		msgs := []string{"scaffold implementation", "add tests", "address edge cases"}
		f.Commits = append(f.Commits, Commit{SHA: fmt.Sprintf("%07x", 0xa1b2c3d+n*7919), Message: fmt.Sprintf("%s: %s", is.Key, msgs[n%len(msgs)]), Author: e.tr.CurrentUser().Name, At: time.Now().UTC().Format(time.RFC3339), Adds: 40 + n*23, Dels: 3 + n*5})
		f.stage("commits").Detail = fmt.Sprintf("%d commits", len(f.Commits))
		evs = append(evs, e.emit(key, "dev", "Pushed commit", f.Commits[len(f.Commits)-1].Message, ""))
		if len(f.Commits) >= 2 {
			f.complete("commits", fmt.Sprintf("%d commits", len(f.Commits)))
		}
	case f.stage("pr").State == StateActive:
		f.PR = &PullRequest{Number: 480 + len(e.flows), Title: fmt.Sprintf("[%s] %s", is.Key, is.Summary),
			URL: fmt.Sprintf("https://github.com/%s/pull/%d", f.Repo, 480+len(e.flows)), State: "open",
			Checks: []Check{{Name: "build", State: StateActive}, {Name: "unit-tests", State: StateActive}, {Name: "lint", State: StateActive}}, Reviewers: []string{"Ana Torres", "Devon Park"}}
		f.complete("pr", fmt.Sprintf("#%d opened", f.PR.Number))
		evs = append(evs, e.emit(key, "dev", fmt.Sprintf("PR #%d opened", f.PR.Number), f.PR.Title, ""))
		evs = append(evs, e.fire(key, TriggerPROpened)...)
	case f.stage("ci").State == StateActive:
		for i := range f.PR.Checks {
			f.PR.Checks[i].State = StateDone
		}
		f.complete("ci", "3/3 checks passed")
		evs = append(evs, e.emit(key, "dev", "Checks passed", "build · unit-tests · lint", ""))
	case f.stage("review").State == StateActive:
		f.PR.Approvals = 2
		f.complete("review", "2 approvals")
		evs = append(evs, e.emit(key, "dev", "PR approved", "Ana Torres, Devon Park approved", ""))
		evs = append(evs, e.fire(key, TriggerReviewApproved)...)
	case f.stage("merged").State == StateActive:
		f.PR.State = "merged"
		f.complete("merged", "squash-merged into main")
		evs = append(evs, e.emit(key, "dev", fmt.Sprintf("PR #%d merged", f.PR.Number), "squash-merged into main", ""))
		evs = append(evs, e.fire(key, TriggerPRMerged)...)
	}
	return e.result(f, evs)
}

// Reset clears the dev flow for a ticket (demo helper).
func (e *Engine) Reset(key string) (StepResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.flows, key)
	return e.result(e.flowFor(key), nil)
}

func (e *Engine) seed() {
	h := time.Hour
	// PLAT-142: in progress, commits pushed, ready to open PR.
	f := e.flowFor("PLAT-142")
	f.Started = true
	f.Branch = "feature/PLAT-142-health-gated-traffic-shifting"
	f.Stages[0] = Stage{ID: "branch", Label: "Branch", State: StateDone, Detail: f.Branch, At: ts(26 * h), Trigger: TriggerBranchCreated}
	f.Stages[1] = Stage{ID: "commits", Label: "Commits", State: StateDone, Detail: "3 commits", At: ts(40 * time.Minute)}
	f.Stages[2].State = StateActive
	f.Commits = []Commit{
		{SHA: "9f3e1ac", Message: "PLAT-142: add weighted shift controller", Author: "Miguel Rosales", At: ts(25 * h), Adds: 212, Dels: 14},
		{SHA: "4b07d2e", Message: "PLAT-142: abortThreshold in manifest", Author: "Miguel Rosales", At: ts(5 * h), Adds: 58, Dels: 6},
		{SHA: "c81aa90", Message: "PLAT-142: rollback on error budget breach", Author: "Miguel Rosales", At: ts(40 * time.Minute), Adds: 97, Dels: 21},
	}
	e.emit("PLAT-142", "dev", "Branch created", f.Branch, ts(26*h))
	e.emit("PLAT-142", "automation", "To Do → In Progress", "Rule “Branch created” fired", ts(26*h-time.Minute))
	e.emit("PLAT-142", "dev", "Pushed commit", "PLAT-142: rollback on error budget breach", ts(40*time.Minute))

	// PLAT-139: in review, waiting on approvals.
	g := e.flowFor("PLAT-139")
	g.Started = true
	g.Branch = "feature/PLAT-139-pre-warm-connection-pools"
	for i, d := range []string{g.Branch, "2 commits", "#478 opened", "3/3 checks passed"} {
		g.Stages[i].State, g.Stages[i].Detail, g.Stages[i].At = StateDone, d, ts(time.Duration(30-i*6)*h)
	}
	g.Stages[4].State = StateActive
	g.Commits = []Commit{
		{SHA: "1d4c9b0", Message: "PLAT-139: warm pools before traffic shift", Author: "Miguel Rosales", At: ts(30 * h), Adds: 88, Dels: 9},
		{SHA: "e7720fa", Message: "PLAT-139: make pool size configurable", Author: "Miguel Rosales", At: ts(24 * h), Adds: 31, Dels: 4},
	}
	g.PR = &PullRequest{Number: 478, Title: "[PLAT-139] Pre-warm connection pools on green stack", URL: "https://github.com/acme/platform/pull/478",
		State: "open", Approvals: 1, Reviewers: []string{"Ana Torres", "Devon Park"},
		Checks: []Check{{Name: "build", State: StateDone}, {Name: "unit-tests", State: StateDone}, {Name: "lint", State: StateDone}}}
	e.emit("PLAT-139", "dev", "PR #478 opened", g.PR.Title, ts(18*h))
	e.emit("PLAT-139", "automation", "In Progress → In Review", "Rule “Pull request opened” fired", ts(18*h-time.Minute))
	e.emit("PLAT-139", "dev", "Ana Torres approved", "1 of 2 approvals", ts(3*h))
	e.emit("PLAT-147", "jira", "Bug reported", "Deploy dashboard shows stale release", ts(9*h))
}

// OnCommand reacts to a command typed in the ticket's terminal. Known git/gh
// commands drive the flow; other dev commands are logged on the timeline.
// ok is false when the command was not interesting.
func (e *Engine) OnCommand(key, line string) (res StepResult, ok bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	f := e.flowFor(key)
	fields := strings.Fields(line)
	has := func(prefix ...string) bool {
		if len(fields) < len(prefix) {
			return false
		}
		for i, p := range prefix {
			if fields[i] != p {
				return false
			}
		}
		return true
	}
	logged := []Event{e.emit(key, "term", "$ "+line, "ran in ticket terminal", "")}
	collect := func(r StepResult, err error) bool {
		if err != nil {
			return false
		}
		logged = append(logged, r.Events...)
		return true
	}
	// until advances the simulated flow until stage is done.
	until := func(stage string) {
		for i := 0; i < len(f.Stages) && f.stage(stage).State != StateDone; i++ {
			if !collect(e.advance(key)) {
				return
			}
		}
	}

	if !e.sim {
		switch {
		case has("git", "checkout", "-b") || has("git", "switch", "-c"):
			// A real branch: start work now, the git scan confirms it later.
			if !f.Started && len(fields) > 3 {
				collect(e.start(key))
				f.Branch = fields[3]
				f.stage("branch").Detail = f.Branch
			}
			if e.poke != nil {
				e.poke(key)
			}
		case has("git") || has("gh"):
			if e.poke != nil {
				e.poke(key)
			}
		case has("make") || has("go", "test") || has("npm", "test") || has("npm", "run"):
		default:
			e.events = e.events[:len(e.events)-1]
			return StepResult{}, false
		}
		r, _ := e.result(f, logged)
		return r, true
	}
	switch {
	case has("git", "checkout", "-b") || has("git", "switch", "-c"):
		if !f.Started && len(fields) > 3 {
			collect(e.start(key))
			f.Branch = fields[3]
			f.stage("branch").Detail = f.Branch
		}
	case has("git", "push"):
		if f.Started && f.stage("commits").State == StateActive {
			collect(e.advance(key))
		}
	case has("gh", "pr", "create"):
		if f.Started {
			until("pr")
		}
	case has("gh", "pr", "merge"):
		if f.Started {
			until("merged")
		}
	case has("git") || has("gh") || has("make") || has("go", "test") || has("npm", "test") || has("npm", "run"):
	default:
		// Not a dev command: drop the log entry we just made.
		e.events = e.events[:len(e.events)-1]
		return StepResult{}, false
	}
	r, _ := e.result(f, logged)
	return r, true
}
