package main

import (
	"context"
	"encoding/base64"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"maestro/internal/applog"
	"maestro/internal/config"
	"maestro/internal/flow"
	"maestro/internal/term"
	"maestro/internal/tracker"
	"maestro/internal/tracker/live"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the bridge between the Go backend and the frontend. Every exported
// method is callable from TypeScript via the generated wailsjs bindings.
//
// The tracker is chosen at startup (see connect.go): the live Jira tracker
// when Maestro is logged in, else the demo mock. Methods that need it call
// a.tr()/a.fl(), which wait until that choice is made.
type App struct {
	ctx  context.Context
	term *term.Manager
	// cfg is the effective configuration; Settings can replace it (read it with conf()).
	cfgMu  sync.RWMutex
	cfg    *config.Config
	layers *config.Layers // personal + team files; nil when config.yaml could not be read
	logs   *applog.Logs   // nil if the log file could not be opened
	ui     *applog.Frontend
	log    *slog.Logger
	ready  chan struct{} // closed once tracker and flow are set

	mu      sync.RWMutex
	tracker tracker.Tracker
	flow    *flow.Engine
	live    *live.Tracker // nil in demo mode
	conn    Connection

	dev devState // GitHub pull requests and local branches (github.go)
}

func NewApp(cfg *config.Config, logs *applog.Logs) *App {
	a := &App{cfg: cfg, logs: logs, ui: applog.NewFrontend(), log: slog.Default().With("component", "app"), ready: make(chan struct{})}
	a.term = term.NewManager(term.Hooks{
		OnData: func(key string, data []byte, end int64) {
			wruntime.EventsEmit(a.ctx, "term:data", key, base64.StdEncoding.EncodeToString(data), end)
		},
		OnExit:     func(key string) { wruntime.EventsEmit(a.ctx, "term:exit", key) },
		OnActivity: func(ev term.Activity) { a.onTermActivity(ev) },
		OnCommand: func(session, line string) {
			if res, ok := a.fl().OnCommand(term.SessionTicket(session), line); ok {
				wruntime.EventsEmit(a.ctx, "flow:step", res)
			}
		},
	})
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if n := a.conf().Terminal.Notify; n.IsEnabled() {
		o := term.DefaultNotifyOptions()
		o.MinRun = time.Duration(n.MinSeconds) * time.Second
		o.WaitFor = time.Duration(max(2, n.WaitSeconds)) * time.Second
		if len(n.Ignore) > 0 {
			o.Ignore = n.Ignore
		}
		a.term.StartMonitor(o)
	}
	go a.connect() // never block the window from appearing
}

func (a *App) shutdown(ctx context.Context) {
	a.log.Info("shutting down", "terminals", len(a.term.Keys()))
	a.term.CloseAll()
	a.mu.RLock()
	lt := a.live
	a.mu.RUnlock()
	a.stopDevSync()
	if lt != nil {
		lt.Stop()
	}
}

func (a *App) conf() *config.Config {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return a.cfg
}

// tr and fl wait for startup to pick the tracker (bounded by connect's timeout).
func (a *App) tr() tracker.Tracker {
	<-a.ready
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.tracker
}

func (a *App) fl() *flow.Engine {
	<-a.ready
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.flow
}

// fail logs a failed user operation with its ticket key and returns err.
// Only keys and error text are logged, never ticket content.
func (a *App) fail(op string, err error, attrs ...any) error {
	if err != nil {
		a.log.Warn(op+" failed", append(attrs, "err", err.Error())...)
	}
	return err
}

// Snapshot is everything the UI needs on boot.
type Snapshot struct {
	Me       tracker.User    `json:"me"`
	Issues   []tracker.Issue `json:"issues"`
	Rules    []flow.Rule     `json:"rules"`
	Activity []flow.Event    `json:"activity"`
	Flows    []flow.Flow     `json:"flows"`
	Users    []tracker.User  `json:"users"`

	Statuses     []string                                  `json:"statuses"`
	StatusStates map[string]string                         `json:"statusStates"`
	Sprint       string                                    `json:"sprint"`
	ChildTypes   map[tracker.IssueType][]tracker.IssueType `json:"childTypes"`
	LinkTypes    map[string]string                         `json:"linkTypes"`
	Projects     []string                                  `json:"projects"`
	Live         bool                                      `json:"live"`

	Sprints        []tracker.SprintInfo             `json:"sprints"`
	Versions       map[string][]tracker.VersionInfo `json:"versions"`
	CreateDefaults tracker.CreateDefaults           `json:"createDefaults"`
	TimeTracking   bool                             `json:"timeTracking"`
	CustomColumns  []tracker.CustomColumn           `json:"customColumns"`

	// Naming builds branch names and pull request text (Settings).
	Naming   flow.Naming           `json:"naming"`
	Commands []config.QuickCommand `json:"commands"` // extra one-click terminal commands
}

func (a *App) Snapshot() Snapshot {
	tr, fl := a.tr(), a.fl()
	issues := tr.ListIssues()
	flows := make([]flow.Flow, 0, len(issues))
	for _, is := range issues {
		flows = append(flows, fl.Flow(is.Key))
	}
	m := tr.Meta()
	// Lists and maps are never null in JSON: a live site can leave any of
	// them empty (no activity yet, a 403 on statuses…), and the UI indexes them.
	return Snapshot{
		Me: tr.CurrentUser(), Issues: orEmpty(issues), Rules: orEmpty(fl.Rules()), Activity: orEmpty(fl.Activity(50)), Flows: flows, Users: orEmpty(tr.Users()),
		Statuses: orEmpty(m.Statuses), StatusStates: orEmptyMap(m.StatusStates), Sprint: m.Sprint, ChildTypes: orEmptyMap(m.ChildTypes),
		LinkTypes: orEmptyMap(m.LinkTypes), Projects: orEmpty(m.Projects), Live: m.Live,
		Sprints: orEmpty(m.Sprints), Versions: orEmptyMap(m.Versions), CreateDefaults: m.CreateDefaults, TimeTracking: m.TimeTracking,
		CustomColumns: orEmpty(m.CustomColumns),
		Naming:        fl.Naming(), Commands: orEmpty(a.conf().Terminal.Commands),
	}
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func orEmptyMap[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return map[K]V{}
	}
	return m
}

// cached finds an issue in the synced set without calling Jira.
func (a *App) cached(key string) (tracker.Issue, bool) {
	for _, is := range a.tr().ListIssues() {
		if is.Key == key {
			return is, true
		}
	}
	return tracker.Issue{}, false
}

func (a *App) GetIssue(key string) (tracker.Issue, error) { return a.tr().GetIssue(key) }

func (a *App) GetTransitions(key string) ([]tracker.Transition, error) {
	return a.tr().Transitions(key)
}

func (a *App) TransitionIssue(key, toStatus string) (tracker.Issue, error) {
	before, _ := a.cached(key)
	is, err := a.tr().Transition(key, toStatus)
	if err == nil && before.Status != is.Status {
		a.fl().RecordTransition(key, before.Status, is.Status)
		a.log.Info("status changed", "key", key, "from", before.Status, "to", is.Status, "by", "user")
	}
	return is, a.fail("transition", err, "key", key, "to", toStatus)
}

func (a *App) AddComment(key, body string) (tracker.Issue, error) {
	is, err := a.tr().AddComment(key, body)
	return is, a.fail("comment", err, "key", key)
}

// CreateIssue creates a ticket (optionally under a parent) and logs it on the timeline.
func (a *App) CreateIssue(in tracker.NewIssue) (tracker.Issue, error) {
	is, err := a.tr().CreateIssue(in)
	if err != nil {
		return is, a.fail("create issue", err, "type", string(in.Type), "parent", in.ParentKey)
	}
	a.log.Info("issue created", "key", is.Key, "type", string(is.Type), "parent", is.ParentKey)
	detail := "Created in Maestro"
	if is.ParentKey != "" {
		detail = "Created under " + is.ParentKey
	}
	a.fl().Record(is.Key, "jira", "Ticket created", detail)
	return is, nil
}

// UpdateFields edits versions, due date, labels and custom fields.
func (a *App) UpdateFields(key string, p tracker.FieldPatch) (tracker.Issue, error) {
	is, err := a.tr().UpdateFields(key, p)
	return is, a.fail("update fields", err, "key", key)
}

// GetFieldSpecs lists the editable fields of an issue (for the Jira view).
func (a *App) GetFieldSpecs(key string) ([]tracker.FieldSpec, error) {
	specs, err := a.tr().FieldSpecs(key)
	return specs, a.fail("field specs", err, "key", key)
}

// GetCreateFieldSpecs lists the extra fields of the create dialog.
func (a *App) GetCreateFieldSpecs(project string, issueType tracker.IssueType) ([]tracker.FieldSpec, error) {
	specs, err := a.tr().CreateFieldSpecs(project, issueType)
	return specs, a.fail("create field specs", err, "project", project, "type", string(issueType))
}

// LinkIssues links two tickets; both updated issues are returned.
func (a *App) LinkIssues(from, linkType, to string) ([]tracker.Issue, error) {
	out, err := a.tr().Link(from, linkType, to)
	if err == nil {
		a.fl().Record(from, "jira", "Linked: "+linkType+" "+to, "")
	}
	return out, a.fail("link", err, "from", from, "type", linkType, "to", to)
}

func (a *App) UnlinkIssues(from, linkType, to string) ([]tracker.Issue, error) {
	out, err := a.tr().Unlink(from, linkType, to)
	return out, a.fail("unlink", err, "from", from, "type", linkType, "to", to)
}

func (a *App) GetFlow(key string) flow.Flow                          { return a.fl().Flow(key) }
func (a *App) StartWork(key string) (flow.StepResult, error)         { return a.fl().Start(key) }
func (a *App) SimulateNextEvent(key string) (flow.StepResult, error) { return a.fl().Advance(key) }
func (a *App) ResetFlow(key string) (flow.StepResult, error)         { return a.fl().Reset(key) }
func (a *App) GetActivity() []flow.Event                             { return a.fl().Activity(50) }

// OpenURL opens a link in the system browser.
func (a *App) OpenURL(url string) {
	wruntime.BrowserOpenURL(a.ctx, url)
}

// CopyToClipboard puts text on the system clipboard.
func (a *App) CopyToClipboard(text string) bool {
	return wruntime.ClipboardSetText(a.ctx, text) == nil
}

// ── Ticket terminals ──────────────────────────────────────────────

// TermOpen starts the shell bound to a ticket (no-op if already running).
// Output arrives via the "term:data" event as base64.
// session is the ticket key, or "KEY#n" for extra terminals on the same ticket.
func (a *App) TermOpen(session string, cols, rows int) (bool, error) {
	if term.IsFreestyle(session) {
		// Not linked to a ticket: home folder, no ticket context, only the global terminal setup.
		c := a.termSetup("")
		c.Session = session
		created, err := a.term.Open(c, uint16(cols), uint16(rows))
		return created, a.fail("open freestyle terminal", err, "session", session)
	}
	key := term.SessionTicket(session)
	is, err := a.tr().GetIssue(key)
	if err != nil {
		return false, err
	}
	f := a.fl().Flow(key)
	branch := f.Branch
	if branch == "" {
		branch = a.fl().BranchFor(is)
	}
	summary := is.Summary
	if is.Security != "" {
		summary = "" // a secured issue's content stays out of the shell and agents
	}
	c := a.termSetup(flow.Project(key))
	c.Key, c.Session, c.Summary, c.Branch, c.Dir = key, session, summary, branch, a.repoDir(key)
	created, err := a.term.Open(c, uint16(cols), uint16(rows))
	return created, a.fail("open terminal", err, "key", key)
}

// termSetup is the configured shell, environment and startup commands for a
// new terminal: global ones, then the project's (project "" = freestyle).
func (a *App) termSetup(project string) term.Context {
	t := a.conf().Terminal
	c := term.Context{Startup: append([]string{}, t.Startup...)}
	if t.Shell != "" {
		sh := config.ExpandHome(t.Shell)
		if p, err := exec.LookPath(sh); err == nil {
			c.Shell = p
		} else {
			a.log.Warn("terminal.shell not found; using $SHELL", "shell", t.Shell)
		}
	}
	env := maps.Clone(t.Env)
	if pt, ok := t.Projects[project]; ok && project != "" {
		if env == nil {
			env = map[string]string{}
		}
		maps.Copy(env, pt.Env)
		c.Startup = append(c.Startup, pt.Startup...)
	}
	for _, k := range slices.Sorted(maps.Keys(env)) {
		c.Env = append(c.Env, k+"="+env[k])
	}
	return c
}

// TermHistory is a terminal's recent output (base64) and the output length it
// ends at, to refill a view that was recreated while the shell kept running.
type TermHistory struct {
	Data string `json:"data"`
	End  int64  `json:"end"`
}

func (a *App) TermHistory(session string) TermHistory {
	data, end, _ := a.term.History(session)
	return TermHistory{Data: base64.StdEncoding.EncodeToString(data), End: end}
}

func (a *App) TermWrite(key, data string) error      { return a.term.Write(key, data) }
func (a *App) TermResize(key string, cols, rows int) { a.term.Resize(key, uint16(cols), uint16(rows)) }
func (a *App) TermClose(key string)                  { a.term.Close(key) }
func (a *App) TermSessions() []string                { return a.term.Keys() }

// ── Keybindings ───────────────────────────────────────────────────

// KeybindingsInfo is the user's overrides plus where they are stored.
type KeybindingsInfo struct {
	Overrides config.Keybindings `json:"overrides"`
	Path      string             `json:"path"`
}

func (a *App) GetKeybindings() (KeybindingsInfo, error) {
	dir, err := config.Dir()
	if err != nil {
		return KeybindingsInfo{}, err
	}
	kb, err := config.LoadKeybindings(dir)
	return KeybindingsInfo{Overrides: kb, Path: filepath.Join(dir, "keybindings.json")}, err
}

// SaveKeybindings replaces the stored overrides.
func (a *App) SaveKeybindings(overrides config.Keybindings) (KeybindingsInfo, error) {
	dir, err := config.Dir()
	if err != nil {
		return KeybindingsInfo{}, err
	}
	path, err := config.SaveKeybindings(dir, overrides)
	return KeybindingsInfo{Overrides: overrides, Path: path}, err
}

// ── Logs ──────────────────────────────────────────────────────────

// LogFrontend records a UI error (uncaught exception, rejected promise, React
// render error). Rate limited in applog.
func (a *App) LogFrontend(level, message, stack, source string) {
	a.ui.Log(level, message, stack, source)
}

// LogInfo tells the UI where the logs are and whether the last run crashed.
type LogInfo struct {
	Dir           string `json:"dir"`
	File          string `json:"file"`
	PreviousCrash string `json:"previousCrash"`
}

func (a *App) GetLogInfo() LogInfo {
	if a.logs == nil {
		return LogInfo{}
	}
	return LogInfo{Dir: a.logs.Dir, File: a.logs.File, PreviousCrash: a.logs.PreviousCrash}
}

// OpenLogFolder shows the log directory in Finder / the file manager.
func (a *App) OpenLogFolder() {
	if a.logs != nil {
		wruntime.BrowserOpenURL(a.ctx, "file://"+a.logs.Dir)
	}
}

// repoDir is the local repository of the issue's project (PRJ-11), so the
// ticket terminal starts where git commands make sense. "" = home directory.
func (a *App) repoDir(key string) string {
	project := key
	if i := strings.LastIndex(key, "-"); i > 0 {
		project = key[:i]
	}
	for _, p := range a.conf().Projects {
		if p.Key == project && len(p.Repos) > 0 && p.Repos[0].Path != "" {
			dir := config.ExpandHome(p.Repos[0].Path)
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				return dir
			}
			a.log.Warn("repository path not found; terminal starts in home", "project", project, "path", p.Repos[0].Path)
		}
	}
	return ""
}

// TermActivity is a terminal activity change plus whether to chirp.
type TermActivity struct {
	term.Activity
	Sound bool `json:"sound"`
}

// onTermActivity forwards running / waiting / finished changes to the UI.
// The program name is logged; what it prints is not.
func (a *App) onTermActivity(ev term.Activity) {
	switch {
	case ev.Finished && ev.Notify:
		a.log.Info("terminal command finished", "session", ev.Session, "command", ev.Command, "seconds", ev.Seconds)
	case ev.State == term.Waiting:
		a.log.Info("terminal waiting for input", "session", ev.Session, "command", ev.Command)
	}
	a.emit("term:activity", TermActivity{Activity: ev, Sound: a.conf().Terminal.Notify.HasSound()})
}
