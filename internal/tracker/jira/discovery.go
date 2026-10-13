package jira

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// Discovery is everything Maestro asks Jira about its configuration
// (configuration spec section 5). It is cached on disk and refreshed after
// jira.sync.discoveryTtlHours.
type Discovery struct {
	At       time.Time          `json:"at"`
	Me       *User              `json:"me"`
	Server   *ServerInfo        `json:"server,omitempty"`
	Projects []ProjectDiscovery `json:"projects"`

	Fields       []Field             `json:"fields"`
	Priorities   []Priority          `json:"priorities"`
	Resolutions  []Resolution        `json:"resolutions"`
	LinkTypes    []IssueLinkType     `json:"linkTypes"`
	TimeTracking *TimeTrackingConfig `json:"timeTracking,omitempty"`

	// Warnings are partial failures (e.g. no board access). Discovery
	// still succeeds; the health check shows them.
	Warnings []string `json:"warnings,omitempty"`
}

type ProjectDiscovery struct {
	Project     Project          `json:"project"`
	IssueTypes  []IssueType      `json:"issueTypes"`
	Statuses    []StatusDef      `json:"statuses"`
	Permissions map[string]bool  `json:"permissions"`
	Users       []User           `json:"users"`
	Components  []Component      `json:"components"`
	Versions    []Version        `json:"versions"` // unreleased + released in the last 90 days
	Boards      []BoardDiscovery `json:"boards"`
}

type BoardDiscovery struct {
	Board  Board        `json:"board"`
	Config *BoardConfig `json:"config,omitempty"`
	// A board can run several sprints at once.
	ActiveSprints []Sprint `json:"activeSprints,omitempty"`
	FutureSprints []Sprint `json:"futureSprints,omitempty"`
}

// FirstActive is the earliest active sprint, if any.
func (b *BoardDiscovery) FirstActive() *Sprint {
	if len(b.ActiveSprints) == 0 {
		return nil
	}
	return &b.ActiveSprints[0]
}

// NextFuture is the next planned sprint, if any.
func (b *BoardDiscovery) NextFuture() *Sprint {
	if len(b.FutureSprints) == 0 {
		return nil
	}
	return &b.FutureSprints[0]
}

// Discover runs discovery for the given project keys in parallel (bounded by
// the client's concurrency limit). A 401 or a cancelled context aborts it;
// any other failure of an optional item becomes a warning.
func (c *Client) Discover(ctx context.Context, projectKeys []string) (*Discovery, error) {
	d := &Discovery{At: time.Now().UTC()}
	me, err := c.Myself(ctx) // also proves the token works
	if err != nil {
		return nil, err
	}
	d.Me = me

	var mu sync.Mutex
	warn := func(format string, args ...any) {
		mu.Lock()
		d.Warnings = append(d.Warnings, fmt.Sprintf(format, args...))
		mu.Unlock()
	}
	// soft runs an optional step: fatal errors stop discovery, others warn.
	soft := func(what string, err error) error {
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrUnauthorized) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		warn("%s: %v", what, err)
		return nil
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { d.Server, err = c.ServerInfo(gctx); return soft("server info", err) })
	g.Go(func() (err error) { d.Fields, err = c.Fields(gctx); return soft("fields", err) })
	g.Go(func() (err error) { d.Priorities, err = Collect(c.Priorities(gctx)); return soft("priorities", err) })
	g.Go(func() (err error) { d.Resolutions, err = Collect(c.Resolutions(gctx)); return soft("resolutions", err) })
	g.Go(func() (err error) { d.LinkTypes, err = c.IssueLinkTypes(gctx); return soft("link types", err) })
	g.Go(func() (err error) { d.TimeTracking, err = c.TimeTracking(gctx); return soft("time tracking", err) })

	d.Projects = make([]ProjectDiscovery, len(projectKeys))
	for i, key := range projectKeys {
		pd := &d.Projects[i]
		g.Go(func() error {
			p, err := c.Project(gctx, key)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					warn("project %s: not found or no access", key) // rule 18.2
					pd.Project = Project{Key: key}
					return nil
				}
				return soft("project "+key, err)
			}
			pd.Project = *p
			return c.discoverProject(gctx, pd, soft)
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return d, nil
}

func (c *Client) discoverProject(ctx context.Context, pd *ProjectDiscovery, soft func(string, error) error) error {
	key, id := pd.Project.Key, pd.Project.ID
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		pd.IssueTypes, err = c.ProjectIssueTypes(gctx, id)
		if err != nil && len(pd.Project.IssueTypes) > 0 {
			pd.IssueTypes, err = pd.Project.IssueTypes, nil // from expand=issueTypes
		}
		return soft(key+" issue types", err)
	})
	g.Go(func() (err error) {
		pd.Statuses, err = Collect(c.Statuses(gctx, id))
		return soft(key+" statuses", err)
	})
	g.Go(func() (err error) {
		pd.Permissions, err = c.MyPermissions(gctx, key, WritePermissions)
		return soft(key+" permissions", err)
	})
	g.Go(func() (err error) {
		pd.Users, err = Collect(c.AssignableUsers(gctx, key))
		return soft(key+" assignable users", err)
	})
	g.Go(func() (err error) { pd.Components, err = c.Components(gctx, key); return soft(key+" components", err) })
	g.Go(func() error {
		all, err := Collect(c.Versions(gctx, key))
		if err != nil {
			return soft(key+" versions", err)
		}
		pd.Versions = RecentVersions(all, time.Now())
		return nil
	})
	g.Go(func() error {
		boards, err := Collect(c.Boards(gctx, key))
		if err != nil {
			return soft(key+" boards (board view disabled)", err)
		}
		pd.Boards = make([]BoardDiscovery, len(boards))
		for i, b := range boards {
			bd := &pd.Boards[i]
			bd.Board = b
			cfg, err := c.BoardConfiguration(gctx, b.ID)
			if err := soft(fmt.Sprintf("%s board %d configuration", key, b.ID), err); err != nil {
				return err
			}
			bd.Config = cfg
			if b.Type == "scrum" {
				sprints, err := Collect(c.Sprints(gctx, b.ID, "active", "future"))
				if err := soft(fmt.Sprintf("%s board %d sprints", key, b.ID), err); err != nil {
					return err
				}
				for _, s := range sprints {
					switch s.State {
					case "active":
						bd.ActiveSprints = append(bd.ActiveSprints, s)
					case "future":
						bd.FutureSprints = append(bd.FutureSprints, s)
					}
				}
				sortSprints(bd.ActiveSprints)
				sortSprints(bd.FutureSprints)
			}
		}
		return nil
	})
	return g.Wait()
}

// ── field lookups (delegate to the catalog in fields.go) ───────

func (d *Discovery) SprintField() string      { return d.Standard().Sprint }
func (d *Discovery) RankField() string        { return d.Standard().Rank }
func (d *Discovery) EpicLinkField() string    { return d.Standard().EpicLink }
func (d *Discovery) ParentLinkField() string  { return d.Standard().ParentLink }
func (d *Discovery) StoryPointsField() string { return d.Standard().StoryPoints }

// RecentVersions keeps what pickers show: unreleased versions and
// versions released in the last 90 days; never archived ones. Unreleased come
// first by release date (undated last), then released, newest first.
func RecentVersions(all []Version, now time.Time) []Version {
	cutoff := now.AddDate(0, 0, -90).Format(time.DateOnly)
	var unreleased, released []Version
	for _, v := range all {
		switch {
		case v.Archived:
		case !v.Released:
			unreleased = append(unreleased, v)
		case v.ReleaseDate == "" || v.ReleaseDate >= cutoff:
			released = append(released, v)
		}
	}
	byDate := func(vs []Version, newestFirst bool) {
		sort.SliceStable(vs, func(i, j int) bool {
			a, b := vs[i].ReleaseDate, vs[j].ReleaseDate
			if a == "" || b == "" {
				return a != "" // dated before undated
			}
			if newestFirst {
				return a > b
			}
			return a < b
		})
	}
	byDate(unreleased, false)
	byDate(released, true)
	return append(unreleased, released...)
}

// NextVersion is the "next" version: the earliest unreleased version with a
// release date, else the first unreleased one.
func NextVersion(vs []Version) *Version {
	for i := range vs {
		if !vs[i].Released && vs[i].ReleaseDate != "" {
			return &vs[i]
		}
	}
	for i := range vs {
		if !vs[i].Released {
			return &vs[i]
		}
	}
	return nil
}

// sortSprints orders sprints by start date (undated last), then ID.
func sortSprints(ss []Sprint) {
	sort.SliceStable(ss, func(i, j int) bool {
		a, b := ss[i].StartDate, ss[j].StartDate
		if a != b {
			return a != "" && (b == "" || a < b)
		}
		return ss[i].ID < ss[j].ID
	})
}
