package main

import (
	"net/http"
	"sync"
	"time"

	"maestro/internal/config"
	"maestro/internal/devsync"
	"maestro/internal/flow"
	"maestro/internal/github"
	"maestro/internal/secrets"
	"maestro/internal/tracker"
)

// devState is the GitHub/git side of the app: the watcher and the PR links
// already written to Jira.
type devState struct {
	mu      sync.Mutex
	w       *devsync.Watcher
	status  devsync.Status
	written map[string]string // key + PR URL → state written to Jira
}

// GitHubStatus is shown in the sidebar.
func (a *App) GetGitHubStatus() devsync.Status {
	a.dev.mu.Lock()
	defer a.dev.mu.Unlock()
	return a.dev.status
}

func (a *App) setGitHubStatus(s devsync.Status) {
	a.dev.mu.Lock()
	a.dev.status = s
	a.dev.mu.Unlock()
	a.emit("github:status", s)
}

// configRepos lists the repositories of all configured projects.
func configRepos(cfg *config.Config) []devsync.Repo {
	var out []devsync.Repo
	seen := map[string]bool{}
	for _, p := range cfg.Projects {
		for _, r := range p.Repos {
			if r.Name == "" || seen[r.Name] {
				continue
			}
			seen[r.Name] = true
			out = append(out, devsync.Repo{Name: r.Name, Path: config.ExpandHome(r.Path), Base: r.BaseBranch})
		}
	}
	return out
}

// githubTokens picks the token source from dev.github.auth.
func githubTokens(gh config.GitHub, host string) github.TokenSource {
	if gh.Auth == "token" {
		return github.Keychain{Store: secrets.NewStore(), Ref: secrets.GitHubRef(host)}
	}
	return github.GHCLI{Host: host}
}

// startDevSync starts watching pull requests and local branches for the
// tickets of the live tracker. known says whether a key is a synced issue.
func (a *App) startDevSync(fl *flow.Engine, tr tracker.Tracker, known func(string) bool) {
	gh := a.conf().Dev.GitHub
	repos := configRepos(a.conf())
	host := github.WebHost(gh.APIURL)
	if !gh.IsEnabled() || len(repos) == 0 {
		msg := "Add repositories under projects[].repos to follow pull requests."
		if !gh.IsEnabled() {
			msg = "Turned off (dev.github.enabled: false)."
		}
		a.setGitHubStatus(devsync.Status{State: "off", Host: host, Error: msg})
		return
	}
	client, err := github.New(github.Options{
		APIURL: gh.APIURL, Tokens: githubTokens(gh, host), UserAgent: "Maestro/" + version,
		HTTPClient: &http.Client{Timeout: 20 * time.Second}, Logger: a.log.With("component", "github"),
	})
	if err != nil {
		a.setGitHubStatus(devsync.Status{State: "error", Host: host, Error: err.Error()})
		return
	}
	linker, _ := tr.(tracker.PRLinker)
	if wb := a.conf().Jira.WriteBack.RemoteLinks; wb != nil && !*wb {
		linker = nil
	}
	w, err := devsync.New(devsync.Options{
		Client: client, Host: host, Repos: repos, KeyPattern: a.conf().Dev.KeyPattern, Known: known,
		RequiredChecks: a.conf().Dev.Checks.Required, RequiredApprovals: a.conf().Dev.Review.RequiredApprovals,
		Interval: time.Duration(gh.PollSeconds) * time.Second, Logger: a.log,
		OnStatus: a.setGitHubStatus,
		Apply: func(key string, ds flow.DevState, fire bool) {
			if res, changed := fl.ApplyDev(key, ds, fire); changed {
				a.emit("flow:step", res)
			}
			if linker != nil && ds.PR != nil {
				a.writePRLink(linker, key, ds.PR)
			}
		},
	})
	if err != nil {
		a.setGitHubStatus(devsync.Status{State: "error", Host: host, Error: err.Error()})
		return
	}
	a.dev.mu.Lock()
	a.dev.w = w
	a.dev.mu.Unlock()
	a.setGitHubStatus(w.Status())
	w.Start()
}

// writePRLink puts the PR on the Jira issue once per state (open, merged,
// closed). Failures are retried on the next poll.
func (a *App) writePRLink(l tracker.PRLinker, key string, pr *flow.PullRequest) {
	id := key + " " + pr.URL
	a.dev.mu.Lock()
	done := a.dev.written[id] == pr.State
	a.dev.mu.Unlock()
	if done {
		return
	}
	err := l.LinkPullRequest(key, tracker.PRLink{URL: pr.URL, Repo: pr.Repo, Number: pr.Number, Title: pr.Title, State: pr.State})
	if err != nil {
		a.log.Warn("pull request link failed", "key", key, "number", pr.Number, "err", err.Error())
		return
	}
	a.dev.mu.Lock()
	if a.dev.written == nil {
		a.dev.written = map[string]string{}
	}
	a.dev.written[id] = pr.State
	a.dev.mu.Unlock()
}

// pokeDev asks the watcher to look soon (a git or gh command was typed).
func (a *App) pokeDev(string) {
	a.dev.mu.Lock()
	w := a.dev.w
	a.dev.mu.Unlock()
	if w != nil {
		w.Poke()
	}
}

func (a *App) stopDevSync() {
	a.dev.mu.Lock()
	w := a.dev.w
	a.dev.mu.Unlock()
	if w != nil {
		w.Stop()
	}
}
