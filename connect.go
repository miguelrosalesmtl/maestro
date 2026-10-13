package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"maestro/internal/config"
	"maestro/internal/devsync"
	"maestro/internal/flow"
	"maestro/internal/secrets"
	"maestro/internal/setup"
	"maestro/internal/tracker"
	"maestro/internal/tracker/live"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// connectTimeout bounds discovery + the first sync; after it the UI gets the
// demo data and the reason, instead of waiting forever.
const connectTimeout = 45 * time.Second

// Connection tells the UI where its data comes from. It never claims a live
// connection Maestro does not have.
type Connection struct {
	// Mode is "live" (real Jira data), "demo" (sample data) or "connecting".
	Mode string `json:"mode"`
	// State is the sync state in live mode: syncing | live | offline | error.
	State      string `json:"state"`
	Configured bool   `json:"configured"` // a Jira login exists (maestro-jira login)
	Site       string `json:"site"`       // host only
	Deployment string `json:"deployment"`
	Error      string `json:"error,omitempty"` // why live mode is not active / degraded
	LastSync   string `json:"lastSync,omitempty"`
	Issues     int    `json:"issues"`
}

func (a *App) GetConnection() Connection {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.conn
}

// connect picks the tracker: live Jira when configured and reachable, else
// the demo mock. It always ends by closing a.ready.
func (a *App) connect() {
	conn := Connection{Mode: "connecting"}
	j := a.cfg.Jira
	if j.Transport == config.TransportREST && j.Site != "" {
		conn.Configured, conn.Deployment = true, string(j.Deployment)
		if u, err := url.Parse(j.Site); err == nil {
			conn.Site = u.Host
		}
	}
	a.setConn(conn)

	if !conn.Configured {
		a.useDemo(conn, "")
		return
	}
	lt, err := a.startLive()
	if err != nil {
		a.log.Error("jira connection failed; showing demo data", "err", err.Error())
		a.useDemo(conn, explainConnectError(err))
		return
	}

	// Config vs. Jira (configuration spec 18.2–18.6): log, never block.
	for _, p := range setup.CheckAgainst(a.cfg, lt.Discovery()) {
		level := slog.LevelWarn
		if p.Severity == config.SeverityError {
			level = slog.LevelError
		}
		a.log.Log(context.Background(), level, "config does not match Jira", "key", p.Path, "problem", p.Message)
	}

	rules, repo := liveRules(a.cfg, lt.Meta())
	fl := flow.NewEngineWith(lt, flow.Options{Rules: rules, Repo: repo, Poke: a.pokeDev})
	st := lt.Status()
	conn.Mode, conn.State, conn.LastSync, conn.Issues = "live", string(st.State), st.LastSync, st.Issues
	a.mu.Lock()
	a.tracker, a.flow, a.live, a.conn = lt, fl, lt, conn
	a.mu.Unlock()
	close(a.ready)
	a.log.Info("tracker ready", "mode", "live", "site", conn.Site, "issues", st.Issues)
	a.emit("tracker:connection", a.GetConnection())
	a.startDevSync(fl, lt, lt.Has)
}

func (a *App) startLive() (*live.Tracker, error) {
	client, err := setup.Connect(a.cfg, secrets.NewStore(), a.log.With("component", "jira"))
	if err != nil {
		return nil, err
	}
	cacheDir := ""
	if base, err := os.UserCacheDir(); err == nil {
		cacheDir = filepath.Join(base, "maestro", "jira")
	}
	var lt *live.Tracker
	lt = live.New(live.Options{
		Client: client, Config: a.cfg, CacheDir: cacheDir,
		OnChange: func(c live.Change) { a.onTrackerChange(c) },
		OnStatus: func(s live.Status) { a.onTrackerStatus(s) },
	})
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	if err := lt.Start(ctx); err != nil {
		return nil, err
	}
	return lt, nil
}

func (a *App) useDemo(conn Connection, reason string) {
	tr := tracker.NewMock()
	conn.Mode, conn.Error = "demo", reason
	a.mu.Lock()
	a.tracker, a.flow, a.conn = tr, flow.NewEngine(tr), conn
	a.mu.Unlock()
	close(a.ready)
	a.log.Info("tracker ready", "mode", "demo", "reason", reason)
	a.emit("tracker:connection", a.GetConnection())
	a.setGitHubStatus(devsync.Status{State: "off", Error: "Demo mode: pull requests are followed once Jira is connected."})
}

func (a *App) setConn(c Connection) {
	a.mu.Lock()
	a.conn = c
	a.mu.Unlock()
}

// onTrackerChange forwards sync results to the UI and records status changes
// made in Jira (by other people or Jira Automation) on the timeline.
func (a *App) onTrackerChange(c live.Change) {
	select {
	case <-a.ready:
	default:
		return // the first sync is part of the snapshot
	}
	fl := a.fl()
	for _, sc := range c.StatusChanges {
		fl.Record(sc.Key, "jira", fmt.Sprintf("%s → %s", sc.From, sc.To), "Changed in Jira")
	}
	payload := map[string]any{"issues": c.Issues, "removed": c.Removed}
	if len(c.StatusChanges) > 0 {
		payload["activity"] = fl.Activity(50)
	}
	a.emit("tracker:changed", payload)
}

func (a *App) onTrackerStatus(s live.Status) {
	a.mu.Lock()
	if a.conn.Mode == "live" {
		a.conn.State, a.conn.Error, a.conn.LastSync, a.conn.Issues = string(s.State), s.Error, s.LastSync, s.Issues
	}
	c := a.conn
	a.mu.Unlock()
	a.emit("tracker:connection", c)
}

func (a *App) emit(event string, data any) {
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, event, data)
	}
}

// liveRules turns config automation rules into flow rules. A rule targeting a
// Maestro state ("inReview") is shown as the first status with that state;
// the live tracker resolves it to a real transition either way.
func liveRules(cfg *config.Config, meta tracker.Meta) ([]flow.Rule, string) {
	var rules []flow.Rule
	for i, r := range cfg.Automation.Rules {
		to := r.To
		for _, s := range meta.Statuses {
			if meta.StatusStates[s] == r.To {
				to = s
				break
			}
		}
		rules = append(rules, flow.Rule{ID: fmt.Sprintf("r%d", i+1), Trigger: flow.Trigger(r.Trigger),
			Label: flow.TriggerLabel(flow.Trigger(r.Trigger)), ToStatus: to, Enabled: r.IsEnabled()})
	}
	repo := ""
	for _, p := range cfg.Projects {
		if len(p.Repos) > 0 {
			repo = p.Repos[0].Name
			break
		}
	}
	return rules, repo
}

func explainConnectError(err error) string {
	switch {
	case errors.Is(err, secrets.ErrNotFound):
		return "Jira token not found in the keychain. Run: maestro-jira login"
	case errors.Is(err, secrets.ErrNoKeychain):
		return "No OS keychain available. Set MAESTRO_JIRA_TOKEN or install a Secret Service."
	}
	return err.Error()
}
