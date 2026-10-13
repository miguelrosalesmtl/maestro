package live

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"

	"maestro/internal/config"
	"maestro/internal/tracker"
	"maestro/internal/tracker/jira"
)

// System fields, standard custom fields found by schema, and the
// organization's custom fields from fields.custom.

// customDef is a configured custom field resolved against Jira.
type customDef struct {
	cfg   config.CustomField
	field jira.Field
}

func (c customDef) label() string { return firstNonEmpty(c.cfg.Label, c.field.Name) }

func (c customDef) show() []string {
	if len(c.cfg.Show) == 0 {
		return []string{"view"}
	}
	return c.cfg.Show
}

// newMapper resolves field IDs once per discovery.
func newMapper(cfg *config.Config, d *jira.Discovery, c *jira.Client) *mapper {
	std := d.Standard()
	m := &mapper{cfg: cfg, browse: c.BrowseURL, dep: c.Deployment(), std: std, fields: map[string]jira.Field{}}
	for _, f := range d.Fields {
		m.fields[f.ID] = f
	}
	// "auto" (or unset) means: the board's estimation field.
	m.pointsID = std.StoryPoints
	if sp := cfg.Fields.StoryPoints; sp != "" && sp != config.StoryPointsAuto {
		if f := d.FieldByIDOrName(sp); f != nil {
			m.pointsID = f.ID
		}
	}
	m.sprintID = std.Sprint
	if s := cfg.Fields.Sprint; s != "" {
		if f := d.FieldByIDOrName(s); f != nil {
			m.sprintID = f.ID
		}
	}
	if c.Deployment() == jira.DataCenter {
		m.epicLinkID = std.EpicLink
	}
	for _, cf := range cfg.Fields.Custom {
		if f := d.FieldByIDOrName(cf.Ref()); f != nil {
			m.custom = append(m.custom, customDef{cfg: cf, field: *f})
			if cf.Role == "blockedFlag" {
				m.flagID = f.ID
			}
		}
	}
	if m.flagID == "" {
		m.flagID = std.Flagged
	}
	return m
}

// systemFields are the system fields every sync reads.
var systemFields = []string{"summary", "status", "issuetype", "priority", "assignee", "reporter", "creator", "parent",
	"labels", "components", "created", "updated", "description", "issuelinks", "project", "fixVersions", "versions",
	"duedate", "resolution", "resolutiondate", "security", "statuscategorychangedate", "environment", "watches", "votes"}

// listFields is what syncs request: system fields, the standard custom fields
// Maestro uses, and the configured custom fields (never "*all", rule 7.2).
func (m *mapper) listFields() []string {
	f := slices.Clone(systemFields)
	for _, id := range []string{m.pointsID, m.sprintID, m.epicLinkID, m.flagID, m.std.StartDate} {
		if id != "" && !slices.Contains(f, id) {
			f = append(f, id)
		}
	}
	for _, c := range m.custom {
		if !slices.Contains(f, c.field.ID) {
			f = append(f, c.field.ID)
		}
	}
	return f
}

// fill adds versions, dates, flags, security and custom fields to a mapped
// issue. full is true for a
// single-issue read (Jira view), which may include unconfigured custom fields.
func (m *mapper) fill(is *tracker.Issue, j *jira.Issue, full bool) {
	f := j.Fields
	is.FixVersions, is.AffectsVersions = versionNames(f.FixVersions), versionNames(f.Versions)
	is.DueDate = f.DueDate
	if f.Resolution != nil {
		is.Resolution = f.Resolution.Name
	}
	is.ResolvedAt = isoTime(f.ResolutionDate)
	is.Environment = jira.RichTextToMarkdown(f.Environment)
	if tt := f.TimeTracking; tt != nil && (tt.OriginalEstimate != "" || tt.RemainingEstimate != "" || tt.TimeSpent != "") {
		is.TimeTracking = &tracker.TimeTracking{Original: tt.OriginalEstimate, Remaining: tt.RemainingEstimate, Spent: tt.TimeSpent}
	}
	if f.Security != nil {
		is.Security = f.Security.Name
	}
	is.Attachments = []tracker.Attachment{}
	for _, a := range f.Attachment {
		is.Attachments = append(is.Attachments, tracker.Attachment{ID: a.ID, Name: a.Filename, Size: a.Size, MimeType: a.MimeType, Created: isoTime(a.Created)})
	}
	if f.Watches != nil {
		is.Watchers, is.Watching = f.Watches.WatchCount, f.Watches.IsWatching
	}
	if f.Votes != nil {
		is.Votes = f.Votes.Votes
	}
	if f.Creator != nil {
		u := user(*f.Creator)
		is.Creator = &u
	}
	is.StatusChangedAt = isoTime(f.StatusCategoryChangeDate)

	if m.flagID != "" {
		is.Flagged = !jira.DecodeValue(m.fields[m.flagID].Schema, j.Raw[m.flagID]).Empty
	}
	if m.sprintID != "" && is.StatusCategory != tracker.CategoryDone {
		is.CarriedOver = j.CarriedOver(m.sprintID)
	}
	if m.std.StartDate != "" {
		is.StartDate = jira.DecodeValue(m.fields[m.std.StartDate].Schema, j.Raw[m.std.StartDate]).Text
	}

	is.Custom = []tracker.FieldValue{}
	for _, c := range m.custom {
		v := jira.DecodeValue(c.field.Schema, j.Raw[c.field.ID])
		if v.Empty && !slices.Contains(c.cfg.RequiredOn, "create") {
			continue
		}
		is.Custom = append(is.Custom, fieldValue(c.field.ID, c.label(), c.cfg.Role, c.show(), v))
	}
	// default: with no fields.custom, show other custom fields that
	// have a value, in the Jira view only.
	if full && len(m.custom) == 0 {
		skip := m.std.IDs()
		for id, raw := range j.Raw {
			fd, ok := m.fields[id]
			if !ok || !fd.Custom || slices.Contains(skip, id) {
				continue
			}
			v := jira.DecodeValue(fd.Schema, raw)
			if v.Empty || v.Kind == jira.KindUnknown {
				continue
			}
			is.Custom = append(is.Custom, fieldValue(id, fd.Name, "", []string{"view"}, v))
		}
		slices.SortFunc(is.Custom, func(a, b tracker.FieldValue) int { return strings.Compare(a.Label, b.Label) })
	}
}

func fieldValue(id, label, role string, show []string, v jira.Value) tracker.FieldValue {
	return tracker.FieldValue{ID: id, Label: label, Role: role, Kind: string(v.Kind), Text: v.Text, Items: v.Items,
		Show: show, Editable: v.Kind.Editable()}
}

func versionNames(vs []jira.Version) []string {
	out := []string{}
	for _, v := range vs {
		out = append(out, v.Name)
	}
	return out
}

// customColumns tells the UI where configured custom fields go.
func (m *mapper) customColumns() []tracker.CustomColumn {
	out := []tracker.CustomColumn{}
	for _, c := range m.custom {
		out = append(out, tracker.CustomColumn{ID: c.field.ID, Label: c.label(), Role: c.cfg.Role, Show: c.show()})
	}
	return out
}

// ── edit and create specs ───────────────────────────────────────

// handledOnCreate are fields the create dialog already has its own controls for.
var handledOnCreate = []string{"summary", "description", "issuetype", "project", "priority", "assignee", "reporter",
	"labels", "parent", "fixVersions", "versions"}

func (m *mapper) spec(id string, meta jira.CreateMetaField) tracker.FieldSpec {
	label := firstNonEmpty(meta.Name, id)
	role, def := "", ""
	for _, c := range m.custom {
		if c.field.ID == id {
			label, role, def = c.label(), c.cfg.Role, c.cfg.Default
		}
	}
	sp := tracker.FieldSpec{ID: id, Label: label, Kind: string(jira.KindOf(meta.Schema)), Required: meta.Required,
		Options: jira.AllowedValues(meta.AllowedValues), Role: role, Default: def}
	if meta.Required {
		sp.RequiredBy = "jira"
	}
	return sp
}

func (t *Tracker) FieldSpecs(key string) ([]tracker.FieldSpec, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	meta, err := t.c.EditMeta(ctx, key)
	if err != nil {
		return nil, errf(key, "edit fields of", err)
	}
	var out []tracker.FieldSpec
	for _, id := range []string{"fixVersions", "versions", "duedate"} {
		if f, ok := meta[id]; ok { // only if on the edit screen
			out = append(out, t.m.spec(id, f))
		}
	}
	for _, c := range t.m.custom {
		if f, ok := meta[c.field.ID]; ok && jira.KindOf(f.Schema).Editable() {
			out = append(out, t.m.spec(c.field.ID, f))
		}
	}
	return out, nil
}

// createMetaCache keeps createmeta per project and issue type for the session.
type createMetaCache struct {
	mu sync.Mutex
	m  map[string][]jira.CreateMetaField
}

func (t *Tracker) createMeta(ctx context.Context, project, typeID string) ([]jira.CreateMetaField, error) {
	k := project + "/" + typeID
	t.cmeta.mu.Lock()
	if v, ok := t.cmeta.m[k]; ok {
		t.cmeta.mu.Unlock()
		return v, nil
	}
	t.cmeta.mu.Unlock()
	v, err := t.c.CreateMeta(ctx, project, typeID)
	if err != nil {
		return nil, err
	}
	t.cmeta.mu.Lock()
	if t.cmeta.m == nil {
		t.cmeta.m = map[string][]jira.CreateMetaField{}
	}
	t.cmeta.m[k] = v
	t.cmeta.mu.Unlock()
	return v, nil
}

func (t *Tracker) CreateFieldSpecs(project string, it tracker.IssueType) ([]tracker.FieldSpec, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	if project == "" {
		var err error
		if project, err = t.projectFor(tracker.NewIssue{Type: it}, config.Role(it)); err != nil {
			return nil, err
		}
	}
	typeID, err := t.issueTypeFor(project, config.Role(it))
	if err != nil {
		return nil, err
	}
	meta, err := t.createMeta(ctx, project, typeID)
	if err != nil {
		return nil, errf(project, "create fields of", err)
	}
	skip := append(slices.Clone(handledOnCreate), t.m.pointsID, t.m.sprintID, t.m.epicLinkID)
	var out []tracker.FieldSpec
	seen := map[string]bool{}
	for _, f := range meta {
		id := firstNonEmpty(f.FieldID, f.Key)
		if slices.Contains(skip, id) {
			continue
		}
		cfgd := slices.IndexFunc(t.m.custom, func(c customDef) bool { return c.field.ID == id })
		wanted := f.Required && !f.HasDefault // FLD-11: Jira requires it
		if cfgd >= 0 {
			c := t.m.custom[cfgd]
			wanted = wanted || c.cfg.Shows("create") || slices.Contains(c.cfg.RequiredOn, "create")
		}
		if !wanted || seen[id] {
			continue
		}
		seen[id] = true
		sp := t.m.spec(id, f)
		if cfgd >= 0 {
			c := t.m.custom[cfgd]
			if slices.Contains(c.cfg.RequiredOn, "create") && !sp.Required {
				sp.Required, sp.RequiredBy = true, "team"
			}
			if len(c.cfg.DefaultFor) > 0 && !slices.Contains(c.cfg.DefaultFor, string(it)) {
				sp.Default = ""
			}
		}
		if sp.Required && !jira.Kind(sp.Kind).Editable() {
			return nil, fmt.Errorf("Jira requires %q to create a %s in %s, and Maestro cannot edit that field type yet", sp.Label, it, project)
		}
		out = append(out, sp)
	}
	return out, nil
}

// ── edits ───────────────────────────────────────────────────────

func (t *Tracker) UpdateFields(key string, p tracker.FieldPatch) (tracker.Issue, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	fields := map[string]any{}
	names := func(vs []string) []map[string]string {
		out := []map[string]string{}
		for _, v := range vs {
			out = append(out, map[string]string{"name": v})
		}
		return out
	}
	if p.FixVersions != nil {
		fields["fixVersions"] = names(*p.FixVersions)
	}
	if p.AffectsVersions != nil {
		fields["versions"] = names(*p.AffectsVersions)
	}
	if p.DueDate != nil {
		if *p.DueDate == "" {
			fields["duedate"] = nil
		} else {
			fields["duedate"] = *p.DueDate
		}
	}
	if p.Labels != nil {
		fields["labels"] = *p.Labels
	}
	for id, in := range p.Custom {
		fd, ok := t.m.fields[id]
		if !ok {
			return tracker.Issue{}, fmt.Errorf("unknown field %s", id)
		}
		v, err := jira.EncodeValue(fd.Schema, in, t.c.Deployment())
		if err != nil {
			return tracker.Issue{}, fmt.Errorf("%s: %w", fd.Name, err)
		}
		fields[id] = v
	}
	if len(fields) == 0 {
		return t.cachedIssue(key)
	}
	if err := t.c.EditIssue(ctx, key, fields); err != nil {
		return tracker.Issue{}, errf(key, "update", err)
	}
	t.log.Info("fields updated", "key", key, "fields", len(fields))
	return t.afterWrite(ctx, key)
}

func (t *Tracker) cachedIssue(key string) (tracker.Issue, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if e, ok := t.issues[key]; ok {
		return e.issue, nil
	}
	return tracker.Issue{}, errors.New("issue not loaded: " + key)
}

// requiredBefore checks team rules before a transition.
func (t *Tracker) requiredBefore(is tracker.Issue, state config.State) error {
	var missing []string
	for _, c := range t.m.custom {
		if !slices.Contains(c.cfg.RequiredOn, string(state)) {
			continue
		}
		if !slices.ContainsFunc(is.Custom, func(v tracker.FieldValue) bool { return v.ID == c.field.ID && (v.Text != "" || len(v.Items) > 0) }) {
			missing = append(missing, c.label())
		}
	}
	if state == config.StateDone && t.cfg.Versions.RequireFixVersionOnDone && len(is.FixVersions) == 0 && is.Type != tracker.TypeSubtask {
		missing = append(missing, "Fix version")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s needs %s before moving to %s (team rule)", is.Key, strings.Join(missing, ", "), state)
	}
	return nil
}

// points rounds a story points value.
func points(v float64) int { return int(math.Round(v)) }
