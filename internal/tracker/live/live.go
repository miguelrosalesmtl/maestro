// Package live is the Jira-backed tracker.Tracker: it maps Jira issues into
// Maestro's model, keeps them in sync by polling
// and writes changes back through the REST client.
package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"maestro/internal/config"
	"maestro/internal/setup"
	"maestro/internal/tracker"
	"maestro/internal/tracker/jira"
)

// MaxIssues caps the synced set; beyond it Maestro logs a warning.
const MaxIssues = 3000

const callTimeout = 30 * time.Second

// Change is what one sync or write changed, for the UI and the timeline.
type Change struct {
	Issues        []tracker.Issue `json:"issues"`
	Removed       []string        `json:"removed"`
	StatusChanges []StatusChange  `json:"statusChanges"`
}

type StatusChange struct {
	Key  string `json:"key"`
	From string `json:"from"`
	To   string `json:"to"`
}

func (c Change) empty() bool { return len(c.Issues) == 0 && len(c.Removed) == 0 }

// SyncState is the connection state shown in the UI.
type SyncState string

const (
	StateSyncing SyncState = "syncing"
	StateLive    SyncState = "live"
	StateOffline SyncState = "offline" // network errors; retrying
	StateError   SyncState = "error"   // token rejected etc.; stopped
)

type Status struct {
	State    SyncState `json:"state"`
	LastSync string    `json:"lastSync"`
	Issues   int       `json:"issues"`
	Error    string    `json:"error,omitempty"`
}

type Options struct {
	Client   *jira.Client
	Config   *config.Config
	CacheDir string // discovery cache directory; "" disables it
	Logger   *slog.Logger
	OnChange func(Change) // after each sync or write that changed something
	OnStatus func(Status) // when the connection state changes
}

type entry struct {
	issue    tracker.Issue
	raw      jira.Issue
	ancestor bool // fetched only as a parent of an in-scope issue
}

type Tracker struct {
	c   *jira.Client
	cfg *config.Config
	log *slog.Logger
	o   Options
	m   *mapper

	mu         sync.RWMutex
	issues     map[string]*entry
	disc       *jira.Discovery
	keys       []string // projects in scope
	me         tracker.User
	users      []tracker.User
	meta       tracker.Meta
	lastSync   time.Time
	lastFull   time.Time
	burstUntil time.Time
	status     Status
	linkSwap   bool // learned direction of POST /issueLink (see Link)

	cmeta createMetaCache

	syncMu sync.Mutex // one sync at a time (loop or Refresh)
	cancel context.CancelFunc
	done   chan struct{}
	now    func() time.Time
}

func New(o Options) *Tracker {
	log := o.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Tracker{
		c: o.Client, cfg: o.Config, o: o, log: log.With("component", "jira-sync"),
		issues: map[string]*entry{}, now: time.Now,
		status: Status{State: StateSyncing},
	}
}

// Start runs discovery and the first full sync, then keeps syncing in the
// background until Stop. It returns the first sync's error, if any.
func (t *Tracker) Start(ctx context.Context) error {
	t.setStatus(StateSyncing, "")
	if err := t.discover(ctx); err != nil {
		t.fail(err)
		return err
	}
	if err := t.fullSync(ctx); err != nil {
		t.fail(err)
		return err
	}
	loopCtx, cancel := context.WithCancel(context.Background())
	t.cancel, t.done = cancel, make(chan struct{})
	go t.loop(loopCtx)
	return nil
}

func (t *Tracker) Stop() {
	if t.cancel != nil {
		t.cancel()
		<-t.done
	}
}

func (t *Tracker) Status() Status {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.status
}

// ── discovery ───────────────────────────────────────────────────

// discoveryCacheVersion changes when the Discovery shape changes, so an old
// cache file is ignored instead of read with missing data.
const discoveryCacheVersion = 2

type cachedDiscovery struct {
	Version   int             `json:"version"`
	Projects  []string        `json:"projects"`
	Discovery *jira.Discovery `json:"discovery"`
}

func (t *Tracker) discover(ctx context.Context) error {
	keys := make([]string, 0, len(t.cfg.Projects))
	for _, p := range t.cfg.Projects {
		keys = append(keys, p.Key)
	}
	if len(keys) == 0 {
		// No projects configured yet: use the ones the user works in.
		sugg, err := setup.SuggestProjects(ctx, t.c)
		if err != nil {
			return err
		}
		for i := 0; i < len(sugg) && i < 5; i++ {
			keys = append(keys, sugg[i].Key)
		}
		t.log.Info("no projects configured; using recent ones", "projects", keys)
	}
	if len(keys) == 0 {
		return errors.New("no Jira projects found; add projects to config.yaml")
	}

	d := t.readCache(keys)
	if d == nil {
		start := time.Now()
		var err error
		if d, err = t.c.Discover(ctx, keys); err != nil {
			return err
		}
		t.log.Info("discovery done", "projects", keys, "took", time.Since(start).Round(time.Millisecond), "warnings", len(d.Warnings))
		for _, w := range d.Warnings {
			t.log.Warn("discovery", "warning", w)
		}
		t.writeCache(keys, d)
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.disc, t.keys = d, keys
	t.m = newMapper(t.cfg, d, t.c)
	if d.Me != nil {
		t.me = user(*d.Me)
	}
	seen := map[string]bool{}
	for _, p := range d.Projects {
		for _, u := range p.Users {
			if u.Active && !seen[u.ID()] {
				seen[u.ID()] = true
				t.users = append(t.users, user(u))
			}
		}
	}
	if !seen[t.me.ID] && t.me.ID != "" {
		t.users = append([]tracker.User{t.me}, t.users...)
	}
	t.meta = t.buildMeta()
	t.meta.Projects = slices.Clone(keys)
	return nil
}

func (t *Tracker) cachePath() string {
	if t.o.CacheDir == "" {
		return ""
	}
	u, err := url.Parse(t.c.BaseURL())
	if err != nil {
		return ""
	}
	return filepath.Join(t.o.CacheDir, u.Host+".json")
}

func (t *Tracker) readCache(keys []string) *jira.Discovery {
	p := t.cachePath()
	if p == "" {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var cd cachedDiscovery
	if json.Unmarshal(b, &cd) != nil || cd.Version != discoveryCacheVersion || cd.Discovery == nil || !slices.Equal(cd.Projects, keys) {
		return nil
	}
	ttl := time.Duration(max(1, t.cfg.Jira.Sync.DiscoveryTTLHours)) * time.Hour
	if time.Since(cd.Discovery.At) > ttl {
		return nil
	}
	t.log.Debug("discovery from cache", "age", time.Since(cd.Discovery.At).Round(time.Second))
	return cd.Discovery
}

func (t *Tracker) writeCache(keys []string, d *jira.Discovery) {
	p := t.cachePath()
	if p == "" {
		return
	}
	b, err := json.Marshal(cachedDiscovery{Version: discoveryCacheVersion, Projects: keys, Discovery: d})
	if err == nil {
		err = os.MkdirAll(filepath.Dir(p), 0o700)
	}
	if err == nil {
		tmp := p + ".tmp"
		if err = os.WriteFile(tmp, b, 0o600); err == nil { // 0600
			err = os.Rename(tmp, p)
		}
	}
	if err != nil {
		t.log.Warn("discovery cache not written", "err", err)
	}
}

// InvalidateDiscovery drops the cache so the next start rediscovers.
func (t *Tracker) InvalidateDiscovery() {
	if p := t.cachePath(); p != "" {
		os.Remove(p)
	}
}

// buildMeta derives statuses, states, sprint, hierarchy and link types.
// Caller holds t.mu.
func (t *Tracker) buildMeta() tracker.Meta {
	meta := tracker.Meta{StatusStates: map[string]string{}, LinkTypes: map[string]string{}, Live: true,
		ChildTypes: map[tracker.IssueType][]tracker.IssueType{}}

	prop := setup.Propose(t.disc)
	order := t.cfg.Workflow.Order
	if len(order) == 0 {
		order = prop.Order
	}
	var all []jira.StatusDef
	for _, p := range t.disc.Projects {
		all = append(all, p.Statuses...)
	}
	seen := map[string]bool{}
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			meta.Statuses = append(meta.Statuses, name)
		}
	}
	for _, n := range order {
		add(n)
	}
	for _, cat := range []string{"TODO", "IN_PROGRESS", "DONE"} { // statuses not on a board
		for _, s := range all {
			if strings.EqualFold(s.StatusCategory, cat) {
				add(s.Name)
			}
		}
	}
	for _, s := range all {
		key := map[string]string{"TODO": "new", "DONE": "done"}[strings.ToUpper(s.StatusCategory)]
		meta.StatusStates[s.Name] = string(t.m.state(s.Name, s.ID, key))
	}

	// Sprints: every active sprint, then future ones, of each project's board.
	for _, key := range t.keys {
		b := t.boardFor(key)
		if b == nil {
			continue
		}
		for _, s := range append(slices.Clone(b.ActiveSprints), b.FutureSprints...) {
			meta.Sprints = append(meta.Sprints, tracker.SprintInfo{ID: s.ID, Name: s.Name, State: s.State, Goal: s.Goal,
				Start: isoTime(s.StartDate), End: isoTime(s.EndDate), Project: key})
		}
	}
	for _, s := range meta.Sprints {
		if s.State == "active" {
			meta.Sprint = s.Name
			break
		}
	}
	// Versions.
	meta.Versions = map[string][]tracker.VersionInfo{}
	for _, p := range t.disc.Projects {
		vs := []tracker.VersionInfo{}
		for _, v := range p.Versions {
			vs = append(vs, tracker.VersionInfo{ID: v.ID, Name: v.Name, Released: v.Released, ReleaseDate: v.ReleaseDate, Overdue: v.Overdue})
		}
		meta.Versions[p.Project.Key] = vs
	}
	d := t.cfg.Defaults.Create
	meta.CreateDefaults = tracker.CreateDefaults{Sprint: d.Sprint, FixVersion: d.FixVersion, AffectsVersion: d.AffectsVersion}
	meta.TimeTracking = t.disc.TimeTracking != nil && t.disc.TimeTracking.Enabled
	meta.CustomColumns = t.m.customColumns()

	for parent, kids := range config.DefaultChildren {
		allowed := kids
		if limit, ok := t.cfg.Hierarchy.Children[parent]; ok {
			allowed = limit
		}
		out := []tracker.IssueType{}
		for _, k := range allowed {
			out = append(out, tracker.IssueType(k))
		}
		meta.ChildTypes[tracker.IssueType(parent)] = out
	}
	for _, lt := range t.disc.LinkTypes {
		meta.LinkTypes[lt.Outward] = lt.Inward
		meta.LinkTypes[lt.Inward] = lt.Outward
	}
	return meta
}

// boardFor is the configured board of a project, else the proposed one.
func (t *Tracker) boardFor(projectKey string) *jira.BoardDiscovery {
	var pd *jira.ProjectDiscovery
	for i := range t.disc.Projects {
		if t.disc.Projects[i].Project.Key == projectKey {
			pd = &t.disc.Projects[i]
		}
	}
	if pd == nil {
		return nil
	}
	for _, p := range t.cfg.Projects {
		if p.Key == projectKey && p.Board != 0 {
			for i := range pd.Boards {
				if pd.Boards[i].Board.ID == p.Board {
					return &pd.Boards[i]
				}
			}
		}
	}
	for i := range pd.Boards {
		if pd.Boards[i].Board.Type == "scrum" {
			return &pd.Boards[i]
		}
	}
	if len(pd.Boards) > 0 {
		return &pd.Boards[0]
	}
	return nil
}

// ── reads (tracker.Tracker) ─────────────────────────────────────

func (t *Tracker) Meta() tracker.Meta {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.meta
}

func (t *Tracker) CurrentUser() tracker.User {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.me
}

func (t *Tracker) Users() []tracker.User {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return slices.Clone(t.users)
}

func (t *Tracker) ListIssues() []tracker.Issue {
	t.mu.RLock()
	defer t.mu.RUnlock()
	keys := make([]string, 0, len(t.issues))
	for k := range t.issues {
		keys = append(keys, k)
	}
	sortKeys(keys)
	out := make([]tracker.Issue, len(keys))
	for i, k := range keys {
		out[i] = t.issues[k].issue
	}
	return out
}

// GetIssue reads the issue fresh from Jira, with comments (Jira view). If
// Jira cannot be reached, the cached copy is returned.
func (t *Tracker) GetIssue(key string) (tracker.Issue, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	is, err := t.refresh(ctx, key, true)
	if err == nil {
		return is, nil
	}
	t.mu.RLock()
	e, ok := t.issues[key]
	t.mu.RUnlock()
	if ok && !errors.Is(err, jira.ErrNotFound) {
		t.log.Warn("using cached issue", "key", key, "err", err.Error())
		return e.issue, nil
	}
	return tracker.Issue{}, err
}

func (t *Tracker) Transitions(key string) ([]tracker.Transition, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	trs, err := t.c.Transitions(ctx, key)
	if err != nil {
		return nil, err
	}
	out := make([]tracker.Transition, 0, len(trs))
	for _, tr := range trs {
		out = append(out, tracker.Transition{ID: tr.ID, Name: tr.Name, To: tr.To.Name, Category: trackerCategory(tr.To.StatusCategory.Key)})
	}
	return out, nil
}

// ── store helpers ───────────────────────────────────────────────

// fields is the field list for syncs and refreshes.
func (t *Tracker) fields() []string { return t.m.listFields() }

// refresh fetches one issue and stores it. withComments also loads the last
// 20 comments (they are not part of syncs).
func (t *Tracker) refresh(ctx context.Context, key string, withComments bool) (tracker.Issue, error) {
	fields := t.fields()
	if withComments {
		fields = []string{"*all"} // one issue in the Jira view: every field
	}
	raw, err := t.c.Issue(ctx, key, fields)
	if err != nil {
		if errors.Is(err, jira.ErrNotFound) {
			t.removeLocal(key)
		}
		return tracker.Issue{}, err
	}
	is := t.m.issueWith(raw, withComments)
	if withComments {
		for c, err := range t.c.Comments(ctx, key) {
			if err != nil {
				t.log.Warn("comments not loaded", "key", key, "err", err.Error())
				break
			}
			is.Comments = append(is.Comments, t.m.comment(c))
			if len(is.Comments) == 20 {
				break
			}
		}
		slices.Reverse(is.Comments) // oldest first, like Jira
	}
	t.mu.Lock()
	prev, had := t.issues[key]
	if !withComments && had {
		is.Comments = prev.issue.Comments
	}
	t.issues[key] = &entry{issue: is, raw: *raw, ancestor: had && prev.ancestor}
	t.relink()
	is = t.issues[key].issue
	t.mu.Unlock()
	return is, nil
}

func (t *Tracker) removeLocal(key string) {
	t.mu.Lock()
	delete(t.issues, key)
	t.relink()
	t.mu.Unlock()
	t.emit(Change{Removed: []string{key}})
}

// relink rebuilds ChildKeys from ParentKey. Caller holds t.mu.
func (t *Tracker) relink() {
	kids := map[string][]string{}
	for k, e := range t.issues {
		if p := e.issue.ParentKey; p != "" {
			kids[p] = append(kids[p], k)
		}
	}
	for k, e := range t.issues {
		ks := kids[k]
		sortKeys(ks)
		if ks == nil {
			ks = []string{}
		}
		e.issue.ChildKeys = ks
	}
}

func (t *Tracker) snapshotOf(keys ...string) []tracker.Issue {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var out []tracker.Issue
	for _, k := range keys {
		if e, ok := t.issues[k]; ok {
			out = append(out, e.issue)
		}
	}
	return out
}

func (t *Tracker) emit(c Change) {
	if t.o.OnChange != nil && !c.empty() {
		t.o.OnChange(c)
	}
}

func (t *Tracker) setStatus(s SyncState, errText string) {
	t.mu.Lock()
	changed := t.status.State != s || t.status.Error != errText
	t.status.State, t.status.Error = s, errText
	if !t.lastSync.IsZero() {
		t.status.LastSync = t.lastSync.UTC().Format(time.RFC3339)
	}
	t.status.Issues = len(t.issues)
	st := t.status
	t.mu.Unlock()
	if changed && t.o.OnStatus != nil {
		t.o.OnStatus(st)
	}
}

// fail records a sync failure: auth problems stop, network problems retry.
func (t *Tracker) fail(err error) {
	switch {
	case errors.Is(err, jira.ErrUnauthorized):
		t.setStatus(StateError, "Jira token is not valid or expired. Run: maestro-jira login")
	case errors.Is(err, jira.ErrOffline):
		t.setStatus(StateOffline, "Cannot reach Jira; retrying")
	default:
		t.setStatus(StateError, err.Error())
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// errf adds the issue key to write errors so the UI message is useful.
func errf(key, op string, err error) error {
	var api *jira.APIError
	if errors.As(err, &api) && api.Detail() != "" {
		return fmt.Errorf("%s %s: %s", op, key, api.Detail())
	}
	return fmt.Errorf("%s %s: %w", op, key, err)
}

// Discovery is the current discovery result (read only).
func (t *Tracker) Discovery() *jira.Discovery {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.disc
}

// Has reports whether the issue is in the synced set (no network call).
func (t *Tracker) Has(key string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, ok := t.issues[key]
	return ok
}
