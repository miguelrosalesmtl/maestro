// Package devsync watches the development work of tickets: pull requests on
// GitHub and branches in the local clones. It turns them into flow.DevState
// values for the flow engine.
//
// Polling is repo-centric: one PR listing per repository and interval (with
// ETags, so an unchanged list costs no rate limit), then details only for
// the PRs that name a ticket Maestro knows. The first look is a baseline: it
// is shown but fires no automation.
package devsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"maestro/internal/applog"
	"maestro/internal/flow"
	"maestro/internal/github"
)

type Repo struct {
	Name string // owner/name on GitHub
	Path string // local clone, may be empty
	Base string // base branch, empty = detect
}

type Options struct {
	Client            *github.Client // nil: local git only
	Host              string         // for messages, e.g. github.com
	Repos             []Repo
	KeyPattern        string
	Known             func(key string) bool // only keys of issues in the tracker
	RequiredChecks    []string
	RequiredApprovals int
	Interval          time.Duration
	Apply             func(key string, ds flow.DevState, fire bool)
	OnStatus          func(Status)
	Logger            *slog.Logger

	git func(ctx context.Context, dir string, args ...string) ([]byte, error)
}

// Status is shown in the sidebar.
type Status struct {
	State    string   `json:"state"` // off | connecting | ok | error
	Host     string   `json:"host"`
	User     string   `json:"user"`
	Repos    []string `json:"repos"`
	Error    string   `json:"error"`
	LastSync string   `json:"lastSync"`
	PRs      int      `json:"prs"` // PRs linked to tickets
}

type Watcher struct {
	o      Options
	log    *slog.Logger
	keyRe  *regexp.Regexp
	git    func(ctx context.Context, dir string, args ...string) ([]byte, error)
	pokeCh chan struct{}

	mu       sync.Mutex
	status   Status
	lists    map[string][]github.PullRequest // repo → last good listing (open + recently closed)
	listed   map[string]bool                 // repo listed once: later changes fire
	closedPR map[string]prDetail             // repo#number → details of a closed PR (they do not change)
	lastPR   map[string]*flow.PullRequest    // key → last PR shown, kept when it leaves the listing
	applied  map[string]bool                 // keys with a state shown
	burst    time.Time
	ran      bool // one Sync done: later changes fire automation

	syncMu sync.Mutex // one Sync at a time

	cancel context.CancelFunc
	done   chan struct{}
}

type prDetail struct {
	updated string
	pr      flow.PullRequest
	commits []flow.Commit
}

func New(o Options) (*Watcher, error) {
	pat := o.KeyPattern
	if pat == "" {
		pat = `[A-Z][A-Z0-9]+-\d+`
	}
	re, err := regexp.Compile(`\b(?:` + pat + `)\b`)
	if err != nil {
		return nil, fmt.Errorf("dev.keyPattern: %w", err)
	}
	if o.Interval <= 0 {
		o.Interval = time.Minute
	}
	if o.Known == nil {
		o.Known = func(string) bool { return true }
	}
	if o.Apply == nil {
		o.Apply = func(string, flow.DevState, bool) {}
	}
	w := &Watcher{o: o, log: o.Logger, keyRe: re, git: o.git, pokeCh: make(chan struct{}, 1),
		lists: map[string][]github.PullRequest{}, listed: map[string]bool{}, closedPR: map[string]prDetail{},
		lastPR: map[string]*flow.PullRequest{}, applied: map[string]bool{}}
	if w.log == nil {
		w.log = slog.Default()
	}
	w.log = w.log.With("component", "devsync")
	if w.git == nil {
		w.git = runGit
	}
	w.status = Status{State: "connecting", Host: o.Host, Repos: w.repoNames()}
	if o.Client == nil {
		w.status.State = "off"
	}
	return w, nil
}

func (w *Watcher) repoNames() []string {
	var out []string
	for _, r := range w.o.Repos {
		out = append(out, r.Name)
	}
	return out
}

func (w *Watcher) Status() Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.status
	s.Repos = slices.Clone(s.Repos)
	return s
}

func (w *Watcher) setStatus(f func(*Status)) {
	w.mu.Lock()
	f(&w.status)
	s := w.status
	w.mu.Unlock()
	if w.o.OnStatus != nil {
		w.o.OnStatus(s)
	}
}

// Start polls until Stop.
func (w *Watcher) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel, w.done = cancel, make(chan struct{})
	go func() {
		defer close(w.done)
		defer applog.Recover("devsync")
		for {
			w.Sync(ctx)
			wait := w.o.Interval
			w.mu.Lock()
			if time.Now().Before(w.burst) {
				wait = min(wait, 15*time.Second)
			}
			w.mu.Unlock()
			t := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-w.pokeCh:
				t.Stop()
				// let git/gh finish what the user typed
				select {
				case <-ctx.Done():
					return
				case <-time.After(3 * time.Second):
				}
			case <-t.C:
			}
		}
	}()
}

func (w *Watcher) Stop() {
	if w.cancel != nil {
		w.cancel()
		<-w.done
	}
}

// Poke asks for a look soon, then every 15 s for two minutes (a push or a PR
// was just made, checks will start).
func (w *Watcher) Poke() {
	w.mu.Lock()
	w.burst = time.Now().Add(2 * time.Minute)
	w.mu.Unlock()
	select {
	case w.pokeCh <- struct{}{}:
	default:
	}
}

// keysIn returns the known ticket keys named in s, in order, without repeats.
func (w *Watcher) keysIn(s string) []string {
	var out []string
	for _, k := range w.keyRe.FindAllString(strings.ToUpper(s), -1) {
		if !slices.Contains(out, k) && w.o.Known(k) {
			out = append(out, k)
		}
	}
	return out
}

// prKeys: the head branch decides; else the title; else the body.
func (w *Watcher) prKeys(pr github.PullRequest) []string {
	for _, s := range []string{pr.Head.Ref, pr.Title, pr.Body} {
		if ks := w.keysIn(s); len(ks) > 0 {
			return ks
		}
	}
	return nil
}

type candidate struct {
	repo Repo
	pr   github.PullRequest
}

// better ranks PRs for one ticket: open first, then merged, then closed;
// inside a group the most recently updated.
func better(a, b github.PullRequest) bool {
	rank := func(p github.PullRequest) int {
		switch {
		case p.State == "open":
			return 0
		case p.IsMerged():
			return 1
		}
		return 2
	}
	if ra, rb := rank(a), rank(b); ra != rb {
		return ra < rb
	}
	return a.UpdatedAt > b.UpdatedAt
}

// Sync does one full look at every repository.
func (w *Watcher) Sync(ctx context.Context) {
	w.syncMu.Lock()
	defer w.syncMu.Unlock()
	var errs []string
	if w.o.Client != nil {
		w.mu.Lock()
		needUser := w.status.User == ""
		w.mu.Unlock()
		if needUser {
			u, err := w.o.Client.Viewer(ctx)
			if err != nil {
				w.fail(err)
				if errors.Is(err, github.ErrUnauthorized) {
					w.syncLocalOnly(ctx)
					return
				}
			} else {
				w.setStatus(func(s *Status) { s.User = u.Login })
				w.log.Info("github connected", "user", u.Login, "repos", len(w.o.Repos))
			}
		}
	}

	prs := map[string][]candidate{}
	for _, r := range w.o.Repos {
		listed, err := w.listRepo(ctx, r)
		if err != nil {
			errs = append(errs, r.Name+": "+short(err))
		}
		for _, pr := range listed {
			for _, k := range w.prKeys(pr) {
				prs[k] = append(prs[k], candidate{r, pr})
			}
		}
	}

	local := map[string]localWork{}
	localRepo := map[string]string{}
	for _, r := range w.o.Repos {
		if r.Path == "" {
			continue
		}
		found, err := w.scanLocal(ctx, r.Path, r.Base)
		if err != nil {
			w.log.Debug("local scan failed", "repo", r.Name, "err", err.Error())
			continue
		}
		for k, lw := range found {
			if cur, ok := local[k]; !ok || lw.At > cur.At {
				local[k], localRepo[k] = lw, r.Name
			}
		}
	}

	keys := map[string]bool{}
	for k := range prs {
		keys[k] = true
	}
	for k := range local {
		keys[k] = true
	}
	w.mu.Lock()
	for k := range w.applied {
		keys[k] = true
	}
	for k := range w.lastPR {
		keys[k] = true
	}
	w.mu.Unlock()

	linked := 0
	for k := range keys {
		if ctx.Err() != nil {
			return
		}
		ds := flow.DevState{}
		w.mu.Lock()
		repoFires := w.ran
		w.mu.Unlock()
		if cs := prs[k]; len(cs) > 0 {
			sort.SliceStable(cs, func(i, j int) bool { return better(cs[i].pr, cs[j].pr) })
			c := cs[0]
			pr, commits, err := w.details(ctx, c.repo, c.pr)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s#%d: %s", c.repo.Name, c.pr.Number, short(err)))
				continue // keep what is shown; try again next time
			}
			ds.Repo, ds.Branch, ds.PR, ds.Commits = c.repo.Name, pr.Head, &pr, commits
			w.mu.Lock()
			repoFires = repoFires && w.listed[c.repo.Name]
			w.lastPR[k] = &pr
			w.mu.Unlock()
		} else {
			w.mu.Lock()
			last := w.lastPR[k]
			w.mu.Unlock()
			if last != nil {
				// The PR left the listing (an old merged PR): keep showing it.
				pr := *last
				ds.Repo, ds.Branch, ds.PR = pr.Repo, pr.Head, &pr
			}
		}
		if lw, ok := local[k]; ok {
			if ds.Branch == "" {
				ds.Branch = lw.Branch
			}
			if ds.Repo == "" {
				ds.Repo = localRepo[k]
			}
			if ds.PR == nil || len(ds.Commits) == 0 {
				ds.Commits = lw.Commits
			}
		}
		if ds.PR != nil {
			linked++
		}
		w.mu.Lock()
		had := w.applied[k]
		empty := ds.Branch == "" && ds.PR == nil
		if empty {
			delete(w.applied, k)
		} else {
			w.applied[k] = true
		}
		w.mu.Unlock()
		if empty && !had {
			continue
		}
		w.o.Apply(k, ds, repoFires)
	}

	w.mu.Lock()
	for _, r := range w.o.Repos {
		if _, ok := w.lists[r.Name]; ok {
			w.listed[r.Name] = true
		}
	}
	w.ran = true
	w.mu.Unlock()
	if w.o.Client == nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	w.setStatus(func(s *Status) {
		s.PRs = linked
		s.LastSync = now
		if len(errs) > 0 {
			s.State, s.Error = "error", strings.Join(errs, "; ")
		} else {
			s.State, s.Error = "ok", ""
		}
	})
	if len(errs) > 0 {
		w.log.Warn("github sync problems", "errors", strings.Join(errs, "; "))
	}
}

func (w *Watcher) syncLocalOnly(ctx context.Context) {
	for _, r := range w.o.Repos {
		if r.Path == "" {
			continue
		}
		found, err := w.scanLocal(ctx, r.Path, r.Base)
		if err != nil {
			continue
		}
		for k, lw := range found {
			w.mu.Lock()
			w.applied[k] = true
			fire := w.ran
			w.mu.Unlock()
			w.o.Apply(k, flow.DevState{Repo: r.Name, Branch: lw.Branch, Commits: lw.Commits}, fire)
		}
	}
	w.mu.Lock()
	w.ran = true
	w.mu.Unlock()
}

func (w *Watcher) fail(err error) {
	msg := short(err)
	if errors.Is(err, github.ErrUnauthorized) {
		msg = err.Error()
		if !strings.Contains(msg, "gh auth login") && !strings.Contains(msg, "gh)") {
			msg = fmt.Sprintf("GitHub token rejected. Run: gh auth login --hostname %s", firstNonEmpty(w.o.Host, "github.com"))
		}
	}
	w.setStatus(func(s *Status) { s.State, s.Error, s.User = "error", msg, "" })
	w.log.Warn("github unavailable", "err", err.Error())
}

// listRepo returns open and recently closed PRs; on error the last good list.
func (w *Watcher) listRepo(ctx context.Context, r Repo) ([]github.PullRequest, error) {
	if w.o.Client == nil {
		return nil, nil
	}
	open, err := w.o.Client.OpenPullRequests(ctx, r.Name)
	var closed []github.PullRequest
	if err == nil {
		closed, err = w.o.Client.RecentlyClosedPullRequests(ctx, r.Name)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err != nil {
		return w.lists[r.Name], err
	}
	all := append(open, closed...)
	w.lists[r.Name] = all
	return all, nil
}

// details reads reviews, checks and commits of one PR. Closed PRs are read
// once and cached.
func (w *Watcher) details(ctx context.Context, r Repo, p github.PullRequest) (flow.PullRequest, []flow.Commit, error) {
	id := fmt.Sprintf("%s#%d", r.Name, p.Number)
	w.mu.Lock()
	d, ok := w.closedPR[id]
	w.mu.Unlock()
	if ok && p.State != "open" && d.updated == p.UpdatedAt {
		return d.pr, d.commits, nil
	}

	c := w.o.Client
	pr := flow.PullRequest{Number: p.Number, Title: p.Title, URL: p.HTMLURL, Repo: r.Name, Draft: p.Draft,
		Author: p.User.Login, Head: p.Head.Ref, Base: p.Base.Ref, MergedAt: p.MergedAt, UpdatedAt: p.UpdatedAt,
		RequiredApprovals: max(w.o.RequiredApprovals, 1), Live: true, Checks: []flow.Check{}, Reviewers: []string{}}
	switch {
	case p.State == "open":
		pr.State = "open"
	case p.IsMerged():
		pr.State = "merged"
	default:
		pr.State = "closed"
	}

	reviews, err := c.Reviews(ctx, r.Name, p.Number)
	if err != nil {
		return pr, nil, err
	}
	for login, state := range github.LatestReviews(reviews) {
		if state == "APPROVED" {
			pr.ApprovedBy = append(pr.ApprovedBy, login)
		} else {
			pr.ChangesRequested = append(pr.ChangesRequested, login)
		}
	}
	sort.Strings(pr.ApprovedBy)
	sort.Strings(pr.ChangesRequested)
	pr.Approvals = len(pr.ApprovedBy)
	for _, u := range p.RequestedReviewers {
		pr.Reviewers = append(pr.Reviewers, u.Login)
	}
	for _, rv := range reviews {
		if rv.User.Login != "" && rv.User.Login != p.User.Login && !slices.Contains(pr.Reviewers, rv.User.Login) {
			pr.Reviewers = append(pr.Reviewers, rv.User.Login)
		}
	}

	if p.Head.SHA != "" {
		runs, err := c.CheckRuns(ctx, r.Name, p.Head.SHA)
		if err != nil && !errors.Is(err, github.ErrForbidden) && !errors.Is(err, github.ErrNotFound) {
			return pr, nil, err
		}
		statuses, err := c.Statuses(ctx, r.Name, p.Head.SHA)
		if err != nil && !errors.Is(err, github.ErrForbidden) && !errors.Is(err, github.ErrNotFound) {
			return pr, nil, err
		}
		pr.Checks = w.checks(runs, statuses)
	}

	gc, err := c.PullRequestCommits(ctx, r.Name, p.Number)
	if err != nil {
		return pr, nil, err
	}
	commits := make([]flow.Commit, 0, len(gc))
	for _, g := range gc {
		msg, _, _ := strings.Cut(g.Commit.Message, "\n")
		commits = append(commits, flow.Commit{SHA: shortSHA(g.SHA), Message: msg, Author: g.Commit.Author.Name, At: g.Commit.Author.Date})
	}

	if p.State != "open" {
		w.mu.Lock()
		w.closedPR[id] = prDetail{updated: p.UpdatedAt, pr: pr, commits: commits}
		w.mu.Unlock()
	}
	return pr, commits, nil
}

// checks merges check runs and commit statuses, one entry per name.
func (w *Watcher) checks(runs []github.CheckRun, statuses []github.CommitStatus) []flow.Check {
	byName := map[string]flow.Check{}
	var order []string
	add := func(c flow.Check) {
		if _, ok := byName[c.Name]; !ok {
			order = append(order, c.Name)
		}
		c.Required = slices.Contains(w.o.RequiredChecks, c.Name)
		byName[c.Name] = c
	}
	for _, r := range runs {
		st := flow.StateActive
		if r.Status == "completed" {
			switch r.Conclusion {
			case "success", "neutral", "skipped":
				st = flow.StateDone
			default:
				st = flow.StateFailed
			}
		}
		add(flow.Check{Name: r.Name, State: st, URL: r.HTMLURL})
	}
	for _, s := range statuses {
		st := flow.StateActive
		switch s.State {
		case "success":
			st = flow.StateDone
		case "failure", "error":
			st = flow.StateFailed
		}
		add(flow.Check{Name: s.Context, State: st})
	}
	// A required check that has not reported yet is waiting.
	for _, name := range w.o.RequiredChecks {
		if _, ok := byName[name]; !ok {
			add(flow.Check{Name: name, State: flow.StateActive})
		}
	}
	out := make([]flow.Check, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

func short(err error) string {
	var api *github.APIError
	if errors.As(err, &api) {
		return fmt.Sprintf("%d %s", api.Status, firstNonEmpty(api.Message, "error"))
	}
	return err.Error()
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
