package tracker

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const jiraBaseURL = "https://your-org.atlassian.net/browse/"

var (
	userMe     = User{ID: "u-me", Name: "Miguel Rosales", Initials: "MR", Color: "#7c5cff"}
	userAna    = User{ID: "u-ana", Name: "Ana Torres", Initials: "AT", Color: "#22c5a0"}
	userDev    = User{ID: "u-dev", Name: "Devon Park", Initials: "DP", Color: "#ff8a3d"}
	userPriya  = User{ID: "u-priya", Name: "Priya Shah", Initials: "PS", Color: "#ff4d8d"}
	userJordan = User{ID: "u-jordan", Name: "Jordan Lee", Initials: "JL", Color: "#3db4ff"}
	allUsers   = []User{userMe, userAna, userDev, userPriya, userJordan}
)

// CurrentSprint is the active sprint in the mock project.
const CurrentSprint = "Sprint 42"

// Mock is an in-memory Tracker populated with a realistic hierarchy.
type Mock struct {
	mu     sync.RWMutex
	issues map[string]*Issue
	seq    int
}

func NewMock() *Mock {
	m := &Mock{issues: map[string]*Issue{}}
	m.seed()
	return m
}

func (m *Mock) CurrentUser() User { return userMe }

// StateOf maps the mock's status names to Maestro states.
func StateOf(status string) string {
	switch status {
	case StatusTodo:
		return "todo"
	case StatusInReview:
		return "inReview"
	case StatusBlocked:
		return "blocked"
	case StatusDone:
		return "done"
	}
	return "inProgress"
}

func (m *Mock) Meta() Meta {
	states := map[string]string{}
	for _, s := range Statuses {
		states[s] = StateOf(s)
	}
	end := time.Now().AddDate(0, 0, 6).UTC().Format(time.RFC3339)
	start := time.Now().AddDate(0, 0, -8).UTC().Format(time.RFC3339)
	return Meta{Statuses: Statuses, StatusStates: states, Sprint: CurrentSprint, ChildTypes: ChildTypes, LinkTypes: LinkTypes,
		Projects: []string{"PLAT", "ONB"},
		Sprints: []SprintInfo{
			{ID: 42, Name: CurrentSprint, State: "active", Goal: "Ship health-gated deploys", Start: start, End: end, Project: "PLAT"},
			{ID: 43, Name: "Sprint 43", State: "future", Project: "PLAT"},
		},
		Versions: map[string][]VersionInfo{
			"PLAT": {{ID: "v24", Name: "2.4.0", ReleaseDate: time.Now().AddDate(0, 0, 9).Format(time.DateOnly)},
				{ID: "v25", Name: "2.5.0", ReleaseDate: time.Now().AddDate(0, 1, 0).Format(time.DateOnly)},
				{ID: "v23", Name: "2.3.0", Released: true, ReleaseDate: time.Now().AddDate(0, 0, -20).Format(time.DateOnly)}},
			"ONB": {{ID: "o11", Name: "1.1.0"}},
		},
		CreateDefaults: CreateDefaults{Sprint: "active", FixVersion: "none", AffectsVersion: "none"},
		TimeTracking:   true,
		CustomColumns:  []CustomColumn{{ID: sevField, Label: "Severity", Role: "severity", Show: []string{"view", "create", "column", "card"}}},
	}
}

// The mock's one custom field, to show custom fields without a Jira site.
const sevField = "customfield_severity"

var severities = []string{"Critical", "Major", "Minor"}

func sevValue(v string) []FieldValue {
	fv := FieldValue{ID: sevField, Label: "Severity", Role: "severity", Kind: "option", Text: v,
		Show: []string{"view", "create", "column", "card"}, Editable: true}
	return []FieldValue{fv}
}

func (m *Mock) UpdateFields(key string, p FieldPatch) (Issue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	is, ok := m.issues[key]
	if !ok {
		return Issue{}, fmt.Errorf("issue %s not found", key)
	}
	if p.FixVersions != nil {
		is.FixVersions = slices.Clone(*p.FixVersions)
	}
	if p.AffectsVersions != nil {
		is.AffectsVersions = slices.Clone(*p.AffectsVersions)
	}
	if p.DueDate != nil {
		is.DueDate = *p.DueDate
	}
	if p.Labels != nil {
		is.Labels = slices.Clone(*p.Labels)
	}
	if v, ok := p.Custom[sevField]; ok {
		is.Custom = nil
		if len(v) > 0 && v[0] != "" {
			is.Custom = sevValue(v[0])
		}
	}
	is.Updated = now()
	return clone(is), nil
}

func (m *Mock) FieldSpecs(key string) ([]FieldSpec, error) {
	vs := []string{"2.4.0", "2.5.0", "2.3.0"}
	return []FieldSpec{
		{ID: "fixVersions", Label: "Fix versions", Kind: "versions", Options: vs},
		{ID: "versions", Label: "Affects versions", Kind: "versions", Options: vs},
		{ID: "duedate", Label: "Due date", Kind: "date"},
		{ID: sevField, Label: "Severity", Kind: "option", Options: severities, Role: "severity"},
	}, nil
}

func (m *Mock) CreateFieldSpecs(project string, t IssueType) ([]FieldSpec, error) {
	spec := FieldSpec{ID: sevField, Label: "Severity", Kind: "option", Options: severities, Role: "severity"}
	if t == TypeBug {
		spec.Required, spec.RequiredBy, spec.Default = true, "team", "Major"
	}
	return []FieldSpec{spec}, nil
}

func (m *Mock) ListIssues() []Issue {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Issue, 0, len(m.issues))
	for _, is := range m.issues {
		out = append(out, clone(is))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func (m *Mock) GetIssue(key string) (Issue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	is, ok := m.issues[key]
	if !ok {
		return Issue{}, fmt.Errorf("issue %s not found", key)
	}
	return clone(is), nil
}

func clone(is *Issue) Issue {
	c := *is
	c.Labels = slices.Clone(is.Labels)
	c.Components = slices.Clone(is.Components)
	c.ChildKeys = slices.Clone(is.ChildKeys)
	c.Links = slices.Clone(is.Links)
	c.Comments = slices.Clone(is.Comments)
	c.FixVersions = slices.Clone(is.FixVersions)
	c.AffectsVersions = slices.Clone(is.AffectsVersions)
	c.Attachments = slices.Clone(is.Attachments)
	c.Custom = slices.Clone(is.Custom)
	if is.TimeTracking != nil {
		tt := *is.TimeTracking
		c.TimeTracking = &tt
	}
	return c
}

func (m *Mock) Transitions(key string) ([]Transition, error) {
	is, err := m.GetIssue(key)
	if err != nil {
		return nil, err
	}
	var out []Transition
	for i, s := range Statuses {
		if s == is.Status {
			continue
		}
		out = append(out, Transition{ID: fmt.Sprint(i + 1), Name: s, To: s, Category: CategoryOf(s)})
	}
	return out, nil
}

func (m *Mock) Transition(key, toStatus string) (Issue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	is, ok := m.issues[key]
	if !ok {
		return Issue{}, fmt.Errorf("issue %s not found", key)
	}
	// Like the live tracker, accept a Maestro state as the target.
	if s, ok := map[string]string{"todo": StatusTodo, "inProgress": StatusInProgress, "inReview": StatusInReview, "blocked": StatusBlocked, "done": StatusDone}[toStatus]; ok {
		toStatus = s
	}
	is.Status = toStatus
	is.StatusCategory = CategoryOf(toStatus)
	is.State = StateOf(toStatus)
	is.Updated = now()
	return clone(is), nil
}

func (m *Mock) AddComment(key, body string) (Issue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	is, ok := m.issues[key]
	if !ok {
		return Issue{}, fmt.Errorf("issue %s not found", key)
	}
	m.seq++
	is.Comments = append(is.Comments, Comment{ID: fmt.Sprintf("c-new-%d", m.seq), Author: userMe, Body: body, Created: now()})
	is.Updated = now()
	return clone(is), nil
}

func (m *Mock) Users() []User { return slices.Clone(allUsers) }

func projectOf(key string) string { return key[:strings.LastIndex(key, "-")] }

func (m *Mock) nextKey(project string) string {
	max := 0
	for k := range m.issues {
		if projectOf(k) == project {
			if n, err := strconv.Atoi(k[len(project)+1:]); err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("%s-%d", project, max+1)
}

func (m *Mock) CreateIssue(in NewIssue) (Issue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	in.Summary = strings.TrimSpace(in.Summary)
	if in.Summary == "" {
		return Issue{}, fmt.Errorf("summary is required")
	}
	if _, ok := ChildTypes[in.Type]; !ok {
		return Issue{}, fmt.Errorf("unknown issue type %q", in.Type)
	}

	project := in.Project
	var parent *Issue
	switch {
	case in.Type == TypeInitiative:
		if in.ParentKey != "" {
			return Issue{}, fmt.Errorf("initiatives are top level")
		}
		project = "INIT"
	case in.ParentKey == "" && in.Type == TypeSubtask:
		return Issue{}, fmt.Errorf("a sub-task needs a parent")
	case in.ParentKey != "":
		p, ok := m.issues[in.ParentKey]
		if !ok {
			return Issue{}, fmt.Errorf("parent %s not found", in.ParentKey)
		}
		if !CanParent(p.Type, in.Type) {
			return Issue{}, fmt.Errorf("a %s can't go under a %s", in.Type, p.Type)
		}
		parent = p
		if p.Type != TypeInitiative {
			project = projectOf(p.Key)
		}
	}
	if project == "" {
		project = "PLAT"
	}
	if in.Priority == "" {
		in.Priority = "Medium"
	}

	is := Issue{
		Key: m.nextKey(project), Summary: in.Summary, Type: in.Type, Status: StatusTodo,
		Priority: in.Priority, Reporter: userMe, Labels: slices.Clone(in.Labels), StoryPoints: in.StoryPoints,
		Sprint: in.Sprint, ParentKey: in.ParentKey, Description: in.Description,
		Created: now(), Updated: now(),
		FixVersions: slices.Clone(in.FixVersions), AffectsVersions: slices.Clone(in.AffectsVersions),
	}
	if v := in.Custom[sevField]; len(v) > 0 && v[0] != "" {
		is.Custom = sevValue(v[0])
	}
	for _, u := range allUsers {
		if u.ID == in.AssigneeID {
			u := u
			is.Assignee = &u
		}
	}
	if parent != nil && in.Type == TypeSubtask {
		is.Sprint = parent.Sprint // sub-tasks always follow their parent
	}
	m.add(is)
	return clone(m.issues[is.Key]), nil
}

func (m *Mock) Link(from, linkType, to string) ([]Issue, error) {
	return m.relink(from, linkType, to, true)
}

func (m *Mock) Unlink(from, linkType, to string) ([]Issue, error) {
	return m.relink(from, linkType, to, false)
}

func (m *Mock) relink(from, linkType, to string, add bool) ([]Issue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inverse, ok := LinkTypes[linkType]
	if !ok {
		return nil, fmt.Errorf("unknown link type %q", linkType)
	}
	a, b := m.issues[from], m.issues[to]
	if a == nil || b == nil {
		return nil, fmt.Errorf("issue not found")
	}
	if from == to {
		return nil, fmt.Errorf("an issue can't link to itself")
	}
	set := func(is *Issue, l Link) {
		is.Links = slices.DeleteFunc(is.Links, func(x Link) bool { return x == l })
		if add {
			is.Links = append(is.Links, l)
		}
		is.Updated = now()
	}
	set(a, Link{Type: linkType, Key: to})
	set(b, Link{Type: inverse, Key: from})
	return []Issue{clone(a), clone(b)}, nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func ago(d time.Duration) string { return time.Now().Add(-d).UTC().Format(time.RFC3339) }

func (m *Mock) add(is Issue) {
	is.StatusCategory = CategoryOf(is.Status)
	is.State = StateOf(is.Status)
	is.URL = jiraBaseURL + is.Key
	if is.Created == "" {
		is.Created = ago(21 * 24 * time.Hour)
	}
	if is.Updated == "" {
		is.Updated = ago(3 * time.Hour)
	}
	if is.Labels == nil {
		is.Labels = []string{}
	}
	if is.Components == nil {
		is.Components = []string{}
	}
	if is.Comments == nil {
		is.Comments = []Comment{}
	}
	is.ChildKeys = []string{}
	if is.Links == nil {
		is.Links = []Link{}
	}
	for _, s := range []*[]string{&is.FixVersions, &is.AffectsVersions} {
		if *s == nil {
			*s = []string{}
		}
	}
	if is.Attachments == nil {
		is.Attachments = []Attachment{}
	}
	if is.Custom == nil {
		is.Custom = []FieldValue{}
	}
	m.issues[is.Key] = &is
	if p, ok := m.issues[is.ParentKey]; ok {
		p.ChildKeys = append(p.ChildKeys, is.Key)
	}
}

func (m *Mock) seed() {
	h := time.Hour
	d := 24 * h

	// ── Initiatives ──────────────────────────────────────────────
	m.add(Issue{Key: "INIT-7", Summary: "Platform Reliability 2026", Type: TypeInitiative, Status: StatusInProgress, Priority: "High",
		Assignee: &userAna, Reporter: userAna, Labels: []string{"h2-2026", "okr"},
		Description: "Raise platform availability to **99.95%** and halve MTTR.\n\nKey results:\n- Zero-downtime deploys for all tier-1 services\n- Unified observability across regions\n- Automated rollback on SLO burn"})
	m.add(Issue{Key: "INIT-9", Summary: "Self-serve Onboarding", Type: TypeInitiative, Status: StatusInProgress, Priority: "Medium",
		Assignee: &userPriya, Reporter: userPriya, Labels: []string{"growth"},
		Description: "Let new teams go from sign-up to first deploy in under **10 minutes** without talking to anyone."})

	// ── Epics ────────────────────────────────────────────────────
	m.add(Issue{Key: "PLAT-100", Summary: "Zero-downtime deploy pipeline", Type: TypeEpic, Status: StatusInProgress, Priority: "High",
		Assignee: &userMe, Reporter: userAna, ParentKey: "INIT-7", Components: []string{"deploy"},
		Description: "Blue/green deploys with automatic health-gated traffic shifting."})
	m.add(Issue{Key: "PLAT-120", Summary: "Unified observability", Type: TypeEpic, Status: StatusTodo, Priority: "Medium",
		Assignee: &userDev, Reporter: userAna, ParentKey: "INIT-7", Components: []string{"observability"},
		Description: "One pane of glass: traces, logs and metrics correlated by request ID."})
	m.add(Issue{Key: "ONB-40", Summary: "Guided project setup", Type: TypeEpic, Status: StatusInProgress, Priority: "Medium",
		Assignee: &userJordan, Reporter: userPriya, ParentKey: "INIT-9", Components: []string{"web"},
		Description: "A wizard that scaffolds repo, CI and first environment."})

	// ── Stories / tasks / bugs ───────────────────────────────────
	m.add(Issue{Key: "PLAT-142", Summary: "Health-gated traffic shifting for blue/green", Type: TypeStory, Status: StatusInProgress, Priority: "High",
		Assignee: &userMe, Reporter: userAna, ParentKey: "PLAT-100", Sprint: "Sprint 42", StoryPoints: 5,
		Labels: []string{"backend", "deploy"}, Components: []string{"deploy"}, Created: ago(6 * d), Updated: ago(40 * time.Minute),
		Description: "## Context\nToday traffic flips 100% at once. We want progressive shifting (10 → 50 → 100) gated on health checks.\n\n## Acceptance criteria\n- [x] Shift weights configurable per service\n- [ ] Abort and roll back if error rate > 2% for 60s\n- [ ] Emit `deploy.shift` events to the audit log\n\n## Notes\nReuse the existing `healthz` probes; do **not** add a new endpoint.",
		Comments: []Comment{
			{ID: "c1", Author: userAna, Body: "Can we make the error threshold configurable too? Payments wants 0.5%.", Created: ago(2 * d)},
			{ID: "c2", Author: userMe, Body: "Yep — adding `abortThreshold` to the service manifest.", Created: ago(47 * h)},
		}})
	m.add(Issue{Key: "PLAT-143", Summary: "Automatic rollback on SLO burn", Type: TypeStory, Status: StatusTodo, Priority: "High",
		Assignee: &userMe, Reporter: userAna, ParentKey: "PLAT-100", Sprint: "Sprint 42", StoryPoints: 8, Labels: []string{"backend", "sre"},
		Description: "When the 1h burn rate exceeds 14.4× budget, trigger rollback to the previous release."})
	m.add(Issue{Key: "PLAT-147", Summary: "Deploy dashboard shows stale release after rollback", Type: TypeBug, Status: StatusTodo, Priority: "Highest",
		Assignee: &userMe, Reporter: userDev, ParentKey: "PLAT-100", Sprint: "Sprint 42", StoryPoints: 2, Labels: []string{"frontend"},
		Description: "Steps:\n1. Deploy v2\n2. Roll back to v1\n3. Dashboard still shows v2 as active\n\nExpected: v1 shown as active within 5s."})
	m.add(Issue{Key: "PLAT-139", Summary: "Pre-warm connection pools on green stack", Type: TypeTask, Status: StatusInReview, Priority: "Medium",
		Assignee: &userMe, Reporter: userMe, ParentKey: "PLAT-100", Sprint: "Sprint 42", StoryPoints: 3, Labels: []string{"backend"},
		Description: "Avoid cold-start latency spike when traffic shifts."})
	m.add(Issue{Key: "PLAT-131", Summary: "Deploy manifest schema v2", Type: TypeStory, Status: StatusDone, Priority: "Medium",
		Assignee: &userDev, Reporter: userAna, ParentKey: "PLAT-100", Sprint: "Sprint 41", StoryPoints: 3, Labels: []string{"backend"},
		Description: "Versioned schema with strategy block."})
	m.add(Issue{Key: "PLAT-121", Summary: "Propagate request ID through gateway", Type: TypeStory, Status: StatusTodo, Priority: "Medium",
		Assignee: &userDev, Reporter: userAna, ParentKey: "PLAT-120", Sprint: "Sprint 43", StoryPoints: 3, Labels: []string{"gateway"},
		Description: "Inject `x-request-id` when missing and forward downstream."})
	m.add(Issue{Key: "PLAT-124", Summary: "Trace ↔ log correlation in UI", Type: TypeStory, Status: StatusBlocked, Priority: "Low",
		Assignee: &userMe, Reporter: userDev, ParentKey: "PLAT-120", Sprint: "Sprint 43", StoryPoints: 5, Labels: []string{"frontend"},
		Description: "Blocked on PLAT-121."})
	m.add(Issue{Key: "ONB-41", Summary: "Repo template picker", Type: TypeStory, Status: StatusDone, Priority: "Medium",
		Assignee: &userJordan, Reporter: userPriya, ParentKey: "ONB-40", Sprint: "Sprint 41", StoryPoints: 3, Labels: []string{"web"},
		Description: "Pick from curated templates."})
	m.add(Issue{Key: "ONB-44", Summary: "Generate CI config from template", Type: TypeTask, Status: StatusInProgress, Priority: "Medium",
		Assignee: &userJordan, Reporter: userPriya, ParentKey: "ONB-40", Sprint: "Sprint 42", StoryPoints: 5, Labels: []string{"ci"},
		Description: "Emit GitHub Actions workflow for the chosen template."})
	m.add(Issue{Key: "ONB-46", Summary: "First-deploy celebration screen", Type: TypeTask, Status: StatusTodo, Priority: "Low",
		Assignee: &userMe, Reporter: userPriya, ParentKey: "ONB-40", Sprint: "Sprint 42", StoryPoints: 1, Labels: []string{"web", "delight"},
		Description: "Confetti. Obviously."})

	// ── Sub-tasks ────────────────────────────────────────────────
	m.add(Issue{Key: "PLAT-150", Summary: "Weighted shift controller", Type: TypeSubtask, Status: StatusDone, Priority: "Medium",
		Assignee: &userMe, Reporter: userMe, ParentKey: "PLAT-142", Sprint: "Sprint 42"})
	m.add(Issue{Key: "PLAT-151", Summary: "abortThreshold in service manifest", Type: TypeSubtask, Status: StatusInProgress, Priority: "Medium",
		Assignee: &userMe, Reporter: userMe, ParentKey: "PLAT-142", Sprint: "Sprint 42"})
	m.add(Issue{Key: "PLAT-152", Summary: "Emit deploy.shift audit events", Type: TypeSubtask, Status: StatusTodo, Priority: "Low",
		Assignee: &userMe, Reporter: userMe, ParentKey: "PLAT-142", Sprint: "Sprint 42"})

	// ── Links ────────────────────────────────────────────────────
	link := func(a, t, b string) {
		m.issues[a].Links = append(m.issues[a].Links, Link{Type: t, Key: b})
		m.issues[b].Links = append(m.issues[b].Links, Link{Type: LinkTypes[t], Key: a})
	}
	// ── Demo values: versions, dates, flags, custom fields ───────
	day := func(d int) string { return time.Now().AddDate(0, 0, d).Format(time.DateOnly) }
	p142 := m.issues["PLAT-142"]
	p142.FixVersions, p142.DueDate, p142.CarriedOver = []string{"2.4.0"}, day(3), 1
	p142.TimeTracking = &TimeTracking{Original: "3d", Remaining: "1d 2h", Spent: "2d 1h"}
	p142.Watchers, p142.Watching, p142.Votes, p142.Creator = 3, true, 1, &userAna
	p142.StatusChangedAt = ago(26 * time.Hour)
	p142.Attachments = []Attachment{{ID: "a1", Name: "shift-diagram.png", Size: 182_000, MimeType: "image/png", Created: ago(5 * 24 * time.Hour)}}
	p147 := m.issues["PLAT-147"]
	p147.Custom, p147.DueDate, p147.Security = sevValue("Critical"), day(-1), "Internal"
	p147.AffectsVersions, p147.Environment = []string{"2.3.0"}, "Production, eu-west-1"
	m.issues["PLAT-139"].Flagged = true
	m.issues["PLAT-139"].FixVersions = []string{"2.4.0"}

	link("PLAT-121", "blocks", "PLAT-124")
	link("PLAT-142", "blocks", "PLAT-143")
	link("PLAT-147", "relates to", "PLAT-143")
}
