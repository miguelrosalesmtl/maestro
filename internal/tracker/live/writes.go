package live

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"maestro/internal/config"
	"maestro/internal/tracker"
	"maestro/internal/tracker/jira"
)

// Writes follow: one Jira call, then re-read the issue
// (rule 3) and push the result to the UI. Creates are never retried
// (enforced by the client).

// Transition moves an issue to a status. target may be:
//   - a Jira status name or ID ("In Review", "10003"), or
//   - a Maestro state ("inReview") — used by automation rules (AUT-01 rule 2).
//
// If no transition reaches the exact status, one that reaches a status with
// the same Maestro state is used. Required screen fields are filled when
// Maestro knows a value (WFL-05/06), else the error names the field.
func (t *Tracker) Transition(key, target string) (tracker.Issue, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	t.mu.RLock()
	cur, ok := t.issues[key]
	t.mu.RUnlock()
	if ok && (strings.EqualFold(cur.issue.Status, target) || cur.issue.State == target) {
		return cur.issue, nil
	}

	trs, err := t.c.Transitions(ctx, key)
	if err != nil {
		return tracker.Issue{}, errf(key, "transitions", err)
	}
	tr := t.pickTransition(trs, target)
	if tr == nil {
		from := ""
		if ok {
			from = cur.issue.Status
		}
		return tracker.Issue{}, fmt.Errorf("Jira has no transition for %s from %q to %q", key, from, target)
	}
	if ok { // team rules before review or done
		if err := t.requiredBefore(cur.issue, t.m.state(tr.To.Name, tr.To.ID, tr.To.StatusCategory.Key)); err != nil {
			return tracker.Issue{}, err
		}
	}

	fields := map[string]any{}
	for _, id := range tr.RequiredFields() {
		switch id {
		case "resolution":
			fields["resolution"] = map[string]string{"name": t.defaultResolution()}
		default:
			name := tr.Fields[id].Name
			return tracker.Issue{}, fmt.Errorf("moving %s to %q needs the field %q; set it in Jira", key, tr.To.Name, firstNonEmpty(name, id))
		}
	}
	if err := t.c.DoTransition(ctx, key, tr.ID, fields); err != nil {
		var api *jira.APIError
		if errors.As(err, &api) && api.FieldOrTransitionStale() {
			t.InvalidateDiscovery()
		}
		return tracker.Issue{}, errf(key, "transition", err)
	}
	t.log.Info("transitioned", "key", key, "to", tr.To.Name, "transition", tr.ID)
	return t.afterWrite(ctx, key)
}

func (t *Tracker) pickTransition(trs []jira.Transition, target string) *jira.Transition {
	for i := range trs {
		if strings.EqualFold(trs[i].To.Name, target) || trs[i].To.ID == target || strings.EqualFold(trs[i].Name, target) {
			return &trs[i]
		}
	}
	want := config.State(target)
	if !slices.Contains(config.States, want) {
		t.mu.RLock()
		want = config.State(t.meta.StatusStates[target])
		t.mu.RUnlock()
	}
	if want == "" {
		return nil
	}
	for i := range trs {
		to := trs[i].To
		if t.m.state(to.Name, to.ID, to.StatusCategory.Key) == want {
			return &trs[i]
		}
	}
	return nil
}

func (t *Tracker) defaultResolution() string {
	if r := t.cfg.Workflow.DefaultResolution; r != "" {
		return r
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, r := range t.disc.Resolutions {
		if strings.EqualFold(r.Name, "Done") {
			return r.Name
		}
	}
	for _, r := range t.disc.Resolutions {
		if r.IsDefault {
			return r.Name
		}
	}
	if len(t.disc.Resolutions) > 0 {
		return t.disc.Resolutions[0].Name
	}
	return "Done"
}

func (t *Tracker) AddComment(key, body string) (tracker.Issue, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	if _, err := t.c.AddComment(ctx, key, body); err != nil {
		return tracker.Issue{}, errf(key, "comment on", err)
	}
	t.bumpBurst()
	is, err := t.refresh(ctx, key, true)
	if err != nil {
		return tracker.Issue{}, err
	}
	t.emit(Change{Issues: []tracker.Issue{is}})
	return is, nil
}

// CreateIssue resolves Maestro's NewIssue into Jira IDs (project, issue type,
// priority, sprint, story points) and creates it.
func (t *Tracker) CreateIssue(in tracker.NewIssue) (tracker.Issue, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	role := config.Role(in.Type)

	project, err := t.projectFor(in, role)
	if err != nil {
		return tracker.Issue{}, err
	}
	typeID, err := t.issueTypeFor(project, role)
	if err != nil {
		return tracker.Issue{}, err
	}
	ci := jira.CreateInput{
		ProjectKey: project, IssueTypeID: typeID, Summary: in.Summary, Description: in.Description,
		AssigneeID: in.AssigneeID, Labels: in.Labels, Extra: map[string]any{},
	}
	t.mu.RLock()
	for _, p := range t.disc.Priorities {
		if strings.EqualFold(p.Name, in.Priority) {
			ci.PriorityID = p.ID
		}
	}
	parentRole := config.Role("")
	if pe, ok := t.issues[in.ParentKey]; ok {
		parentRole = config.Role(pe.issue.Type)
	}
	t.mu.RUnlock()

	if in.ParentKey != "" {
		if t.m.epicLinkID != "" && parentRole == config.RoleEpic && role != config.RoleSubtask {
			ci.Extra[t.m.epicLinkID] = in.ParentKey // Data Center (FLD-04)
		} else {
			ci.ParentKey = in.ParentKey
		}
	}
	if in.StoryPoints > 0 && t.m.pointsID != "" && role != config.RoleSubtask {
		ci.Extra[t.m.pointsID] = in.StoryPoints
	}
	// Sprint: a sprint name, "active" or "next"; empty = backlog.
	// Sub-tasks follow their parent; epics and initiatives are not in sprints.
	if in.Sprint != "" && t.m.sprintID != "" && role != config.RoleSubtask && role != config.RoleEpic && role != config.RoleInitiative {
		id, err := t.sprintID(project, in.Sprint)
		if err != nil {
			return tracker.Issue{}, err
		}
		if id != 0 {
			ci.Extra[t.m.sprintID] = id // the sprint ID as a number
		}
	}
	// Versions: names are accepted by Jira.
	for field, vs := range map[string][]string{"fixVersions": in.FixVersions, "versions": in.AffectsVersions} {
		if len(vs) > 0 {
			refs := make([]map[string]string, len(vs))
			for i, v := range vs {
				refs[i] = map[string]string{"name": v}
			}
			ci.Extra[field] = refs
		}
	}
	// Custom fields, encoded by their schema; team-required ones checked.
	for _, c := range t.m.custom {
		v, given := in.Custom[c.field.ID]
		filled := given && strings.Join(v, "") != ""
		if slices.Contains(c.cfg.RequiredOn, "create") && !filled {
			return tracker.Issue{}, fmt.Errorf("%s is required to create an issue (team rule)", c.label())
		}
	}
	for id, v := range in.Custom {
		fd, ok := t.m.fields[id]
		if !ok || strings.Join(v, "") == "" {
			continue
		}
		enc, err := jira.EncodeValue(fd.Schema, v, t.c.Deployment())
		if err != nil {
			return tracker.Issue{}, fmt.Errorf("%s: %w", fd.Name, err)
		}
		ci.Extra[id] = enc
	}

	created, err := t.c.CreateIssue(ctx, ci)
	if err != nil {
		return tracker.Issue{}, errf(project, "create issue in", err)
	}
	t.log.Info("issue created", "key", created.Key, "type", string(role), "parent", in.ParentKey)
	return t.afterWrite(ctx, created.Key)
}

// projectFor picks the project for a new issue: explicit, else the parent's
// (unless the parent is an initiative, which lives in its own project), else
// defaults.epicProject, else the first project in scope.
func (t *Tracker) projectFor(in tracker.NewIssue, role config.Role) (string, error) {
	if in.Project != "" {
		return in.Project, nil
	}
	if role == config.RoleInitiative && t.cfg.Hierarchy.InitiativeProject != "" {
		return t.cfg.Hierarchy.InitiativeProject, nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	if pe, ok := t.issues[in.ParentKey]; ok && pe.issue.Type != tracker.TypeInitiative {
		if pe.raw.Fields.Project != nil {
			return pe.raw.Fields.Project.Key, nil
		}
		if p, _ := splitKey(in.ParentKey); p != "" {
			return p, nil
		}
	}
	if t.cfg.Defaults.EpicProject != "" {
		return t.cfg.Defaults.EpicProject, nil
	}
	if len(t.keys) > 0 {
		return t.keys[0], nil
	}
	return "", errors.New("no project to create the issue in")
}

func (t *Tracker) issueTypeFor(project string, role config.Role) (string, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, p := range t.disc.Projects {
		if p.Project.Key != project {
			continue
		}
		for _, it := range p.IssueTypes {
			if t.m.role(&it) == role {
				return it.ID, nil
			}
		}
		return "", fmt.Errorf("project %s has no issue type for %q; map one under hierarchy.types in config.yaml", project, role)
	}
	return "", fmt.Errorf("project %s is not in scope; add it to projects in config.yaml", project)
}

// ── links ───────────────────────────────────────────────────────

// Link creates "from <phrase> to", e.g. "PLAT-1 blocks PLAT-2".
//
// Jira's POST /issueLink names its sides inwardIssue/outwardIssue, and which
// side reads "blocks" is a known source of confusion. Reading an issue is
// unambiguous (an outwardIssue entry on A means "A <outward> B"), so Maestro
// creates the link, reads it back, and if the direction came out reversed it
// deletes it, recreates it the other way and remembers the convention.
func (t *Tracker) Link(from, phrase, to string) ([]tracker.Issue, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	lt, src, dst, err := t.resolveLink(from, phrase, to)
	if err != nil {
		return nil, err
	}
	create := func(swap bool) error {
		if swap {
			return t.c.LinkIssues(ctx, lt.Name, dst, src)
		}
		return t.c.LinkIssues(ctx, lt.Name, src, dst)
	}
	t.mu.RLock()
	swap := t.linkSwap
	t.mu.RUnlock()
	if err := create(swap); err != nil {
		return nil, errf(from, "link", err)
	}

	if lt.Inward != lt.Outward { // symmetric types ("relates to") need no check
		raw, err := t.c.Issue(ctx, src, []string{"issuelinks"})
		if err != nil {
			return nil, err
		}
		if id, reversed := findLink(raw, lt, dst); reversed {
			t.log.Warn("issue link direction was reversed; correcting and remembering", "type", lt.Name)
			if err := t.c.DeleteIssueLink(ctx, id); err != nil {
				return nil, errf(from, "fix link", err)
			}
			if err := create(!swap); err != nil {
				return nil, errf(from, "link", err)
			}
			t.mu.Lock()
			t.linkSwap = !swap
			t.mu.Unlock()
		}
	}
	t.log.Info("linked", "from", from, "type", phrase, "to", to)
	return t.afterWriteMany(ctx, from, to)
}

func (t *Tracker) Unlink(from, phrase, to string) ([]tracker.Issue, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	lt, src, dst, err := t.resolveLink(from, phrase, to)
	if err != nil {
		return nil, err
	}
	raw, err := t.c.Issue(ctx, src, []string{"issuelinks"})
	if err != nil {
		return nil, errf(src, "read links of", err)
	}
	id := ""
	for _, l := range raw.Fields.IssueLinks {
		if l.Type.Name != lt.Name {
			continue
		}
		if (l.OutwardIssue != nil && l.OutwardIssue.Key == dst) || (lt.Inward == lt.Outward && l.InwardIssue != nil && l.InwardIssue.Key == dst) {
			id = l.ID
		}
	}
	if id == "" {
		return nil, fmt.Errorf("no %q link between %s and %s", phrase, from, to)
	}
	if err := t.c.DeleteIssueLink(ctx, id); err != nil {
		return nil, errf(from, "unlink", err)
	}
	t.log.Info("unlinked", "from", from, "type", phrase, "to", to)
	return t.afterWriteMany(ctx, from, to)
}

// resolveLink finds the Jira link type for a phrase and orders the issues so
// that "src <outward> dst" holds.
func (t *Tracker) resolveLink(from, phrase, to string) (jira.IssueLinkType, string, string, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, lt := range t.disc.LinkTypes {
		if strings.EqualFold(lt.Outward, phrase) {
			return lt, from, to, nil
		}
	}
	for _, lt := range t.disc.LinkTypes {
		if strings.EqualFold(lt.Inward, phrase) {
			return lt, to, from, nil
		}
	}
	return jira.IssueLinkType{}, "", "", fmt.Errorf("unknown link type %q", phrase)
}

// findLink reports the link of type lt between src and dst, and whether it
// came out reversed (dst <outward> src instead of src <outward> dst).
func findLink(src *jira.Issue, lt jira.IssueLinkType, dst string) (id string, reversed bool) {
	for _, l := range src.Fields.IssueLinks {
		if l.Type.Name != lt.Name {
			continue
		}
		if l.OutwardIssue != nil && l.OutwardIssue.Key == dst {
			return l.ID, false
		}
		if l.InwardIssue != nil && l.InwardIssue.Key == dst {
			id = l.ID
			reversed = true
		}
	}
	return id, reversed
}

// ── after a write ───────────────────────────────────────────────

func (t *Tracker) afterWrite(ctx context.Context, key string) (tracker.Issue, error) {
	t.bumpBurst()
	is, err := t.refresh(ctx, key, false)
	if err != nil {
		return tracker.Issue{}, err
	}
	// The parent's ChildKeys may have changed too.
	keys := []string{key}
	if is.ParentKey != "" {
		keys = append(keys, is.ParentKey)
	}
	t.emit(Change{Issues: t.snapshotOf(keys...)})
	return is, nil
}

func (t *Tracker) afterWriteMany(ctx context.Context, keys ...string) ([]tracker.Issue, error) {
	t.bumpBurst()
	for _, k := range keys {
		if _, err := t.refresh(ctx, k, false); err != nil && !errors.Is(err, jira.ErrNotFound) {
			return nil, err
		}
	}
	out := t.snapshotOf(keys...)
	t.emit(Change{Issues: out})
	return out, nil
}

// sprintID resolves "active", "next" or a sprint name to a sprint of the
// project's board (0 = backlog).
func (t *Tracker) sprintID(project, which string) (int, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	b := t.boardFor(project)
	if b == nil {
		return 0, nil // Kanban or no board: no sprints
	}
	switch which {
	case "backlog":
		return 0, nil
	case "active":
		if s := b.FirstActive(); s != nil {
			return s.ID, nil
		}
		return 0, nil
	case "next":
		if s := b.NextFuture(); s != nil {
			return s.ID, nil
		}
		return 0, fmt.Errorf("%s has no future sprint; create one in Jira or pick another sprint", project)
	}
	for _, s := range append(slices.Clone(b.ActiveSprints), b.FutureSprints...) {
		if s.Name == which {
			return s.ID, nil
		}
	}
	return 0, fmt.Errorf("sprint %q is not active or planned on the %s board", which, project)
}

// LinkPullRequest adds the pull request as a web link on the issue, or
// updates it (the PR URL is the link's global ID). A merged or closed PR is
// shown resolved (struck through) in Jira.
func (t *Tracker) LinkPullRequest(key string, pr tracker.PRLink) error {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	title := fmt.Sprintf("%s#%d: %s", pr.Repo, pr.Number, pr.Title)
	summary := "Pull request · " + pr.State
	link := jira.RemoteLink{GlobalID: pr.URL, Object: jira.RemoteLinkObject{URL: pr.URL, Title: title, Summary: summary,
		Icon: &jira.RemoteLinkIcon{URL: "https://github.com/favicon.ico", Title: "GitHub"}}}
	if pr.State != "open" {
		link.Object.Status = &struct {
			Resolved bool `json:"resolved"`
		}{Resolved: true}
	}
	if err := t.c.UpsertRemoteLink(ctx, key, link); err != nil {
		return errf(key, "link pull request to", err)
	}
	t.log.Info("pull request linked", "key", key, "repo", pr.Repo, "number", pr.Number, "state", pr.State)
	return nil
}
