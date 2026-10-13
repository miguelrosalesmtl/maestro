package live

import (
	"fmt"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
	"time"

	"maestro/internal/config"
	"maestro/internal/setup"
	"maestro/internal/tracker"
	"maestro/internal/tracker/jira"
)

// mapper turns Jira objects into Maestro's model using the configuration
// (overrides) and discovery (defaults). It holds no mutable state.
type mapper struct {
	cfg        *config.Config
	browse     func(key string) string
	dep        jira.Deployment
	std        jira.StandardFields   // Atlassian custom fields, found by schema
	fields     map[string]jira.Field // all fields by ID
	custom     []customDef           // fields.custom, resolved
	pointsID   string                // FLD-03
	sprintID   string                // FLD-06
	epicLinkID string                // FLD-04 on Data Center
	flagID     string                // or role blockedFlag
}

func (m *mapper) role(t *jira.IssueType) config.Role {
	if t == nil {
		return config.RoleStory
	}
	for _, r := range m.cfg.Hierarchy.Types {
		if (r.ID != "" && r.ID == t.ID) || (r.ID == "" && strings.EqualFold(r.Name, t.Name)) {
			return r.Role
		}
	}
	return setup.ProposeRole(*t).Role
}

// state resolves a status to a Maestro state: the configured mapping (by name
// or ID) wins, else the proposal from the status category (WFL-10).
func (m *mapper) state(name, id, categoryKey string) config.State {
	if s, ok := m.cfg.Workflow.States[name]; ok {
		return s
	}
	if s, ok := m.cfg.Workflow.States[id]; ok {
		return s
	}
	return setup.ProposeState(jira.StatusDef{Name: name, StatusCategory: categoryOf(categoryKey)}).State
}

// categoryOf maps an issue's statusCategory.key to the /statuses/search form.
func categoryOf(key string) string {
	switch key {
	case "new":
		return "TODO"
	case "done":
		return "DONE"
	}
	return "IN_PROGRESS"
}

func trackerCategory(key string) tracker.StatusCategory {
	switch key {
	case "new":
		return tracker.CategoryTodo
	case "done":
		return tracker.CategoryDone
	}
	return tracker.CategoryInProgress
}

func (m *mapper) issue(j *jira.Issue) tracker.Issue { return m.issueWith(j, false) }

// issueWith maps an issue; full adds unconfigured custom fields (Jira view).
func (m *mapper) issueWith(j *jira.Issue, full bool) tracker.Issue {
	f := j.Fields
	is := tracker.Issue{
		Key:         j.Key,
		Summary:     f.Summary,
		Type:        tracker.IssueType(m.role(f.IssueType)),
		Labels:      nonNil(f.Labels),
		Components:  []string{},
		ChildKeys:   []string{},
		Links:       []tracker.Link{},
		Comments:    []tracker.Comment{},
		Description: jira.RichTextToMarkdown(f.Description),
		Created:     isoTime(f.Created),
		Updated:     isoTime(f.Updated),
		URL:         m.browse(j.Key),
	}
	if f.Status != nil {
		is.Status = f.Status.Name
		is.StatusCategory = trackerCategory(f.Status.StatusCategory.Key)
		is.State = string(m.state(f.Status.Name, f.Status.ID, f.Status.StatusCategory.Key))
	}
	if f.Priority != nil {
		is.Priority = f.Priority.Name
	}
	if f.Assignee != nil {
		u := user(*f.Assignee)
		is.Assignee = &u
	}
	if f.Reporter != nil {
		is.Reporter = user(*f.Reporter)
	}
	for _, c := range f.Components {
		is.Components = append(is.Components, c.Name)
	}
	if m.pointsID != "" {
		if v, ok := j.Number(m.pointsID); ok {
			is.StoryPoints = points(v)
		}
	}
	if m.sprintID != "" {
		if s := j.ActiveSprint(m.sprintID); s != nil && s.State != "closed" {
			is.Sprint = s.Name
		}
	}
	is.ParentKey = m.parentKey(j)
	for _, l := range f.IssueLinks {
		switch {
		case l.OutwardIssue != nil:
			is.Links = append(is.Links, tracker.Link{Type: l.Type.Outward, Key: l.OutwardIssue.Key})
		case l.InwardIssue != nil:
			is.Links = append(is.Links, tracker.Link{Type: l.Type.Inward, Key: l.InwardIssue.Key})
		}
	}
	m.fill(&is, j, full)
	return is
}

// parentKey reads the parent: the "parent" field (Cloud, sub-tasks everywhere),
// else the Data Center "Epic Link" field (FLD-04).
func (m *mapper) parentKey(j *jira.Issue) string {
	if j.Fields.Parent != nil && j.Fields.Parent.Key != "" {
		return j.Fields.Parent.Key
	}
	if m.epicLinkID != "" {
		if raw, ok := j.Raw[m.epicLinkID]; ok {
			if k := strings.Trim(string(raw), `"`); k != "null" {
				return k
			}
		}
	}
	return ""
}

func (m *mapper) comment(c jira.Comment) tracker.Comment {
	tc := tracker.Comment{ID: c.ID, Body: jira.RichTextToMarkdown(c.Body), Created: isoTime(c.Created)}
	if c.Author != nil {
		tc.Author = user(*c.Author)
	}
	return tc
}

func user(u jira.User) tracker.User {
	return tracker.User{ID: u.ID(), Name: u.DisplayName, Initials: initials(u.DisplayName), Color: colorFor(u.ID())}
}

func initials(name string) string {
	var out []rune
	for _, w := range strings.Fields(name) {
		if r := []rune(w); len(r) > 0 {
			out = append(out, r[0])
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "?"
	}
	return strings.ToUpper(string(out))
}

var palette = []string{"#7c5cff", "#22c5a0", "#ff8a3d", "#ff4d8d", "#3db4ff", "#f5b83d", "#a78bfa", "#34d399"}

func colorFor(id string) string {
	h := fnv.New32a()
	h.Write([]byte(id))
	return palette[h.Sum32()%uint32(len(palette))]
}

// isoTime turns Jira's "2026-10-13T14:37:05.334-0400" into RFC 3339, which
// every browser parses (Safari rejects the offset without a colon).
func isoTime(s string) string {
	if s == "" {
		return ""
	}
	for _, layout := range []string{"2006-01-02T15:04:05.000-0700", time.RFC3339Nano, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return s
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// sortKeys orders issue keys by project, then number (PLAT-9 before PLAT-10).
func sortKeys(keys []string) {
	slices.SortFunc(keys, func(a, b string) int {
		pa, na := splitKey(a)
		pb, nb := splitKey(b)
		if pa != pb {
			return strings.Compare(pa, pb)
		}
		return na - nb
	})
}

func splitKey(k string) (string, int) {
	i := strings.LastIndex(k, "-")
	if i < 0 {
		return k, 0
	}
	n, _ := strconv.Atoi(k[i+1:])
	return k[:i], n
}

func quoteKeys(keys []string) string {
	q := make([]string, len(keys))
	for i, k := range keys {
		q[i] = fmt.Sprintf("%q", k)
	}
	return strings.Join(q, ",")
}
