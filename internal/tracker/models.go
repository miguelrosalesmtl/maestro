// Package tracker models the issue tracker (Jira) side of Maestro.
//
// The Tracker interface is what the UI talks to. Today it is backed by an
// in-memory mock; later it will be backed by the Jira MCP server.
package tracker

// IssueType mirrors the Jira hierarchy levels Maestro cares about.
type IssueType string

const (
	TypeInitiative IssueType = "initiative"
	TypeEpic       IssueType = "epic"
	TypeStory      IssueType = "story"
	TypeTask       IssueType = "task"
	TypeBug        IssueType = "bug"
	TypeSubtask    IssueType = "subtask"
)

// ChildTypes is the Jira hierarchy: which issue types may sit under which.
var ChildTypes = map[IssueType][]IssueType{
	TypeInitiative: {TypeEpic},
	TypeEpic:       {TypeStory, TypeTask, TypeBug},
	TypeStory:      {TypeSubtask},
	TypeTask:       {TypeSubtask},
	TypeBug:        {TypeSubtask},
	TypeSubtask:    {},
}

// CanParent reports whether a child of type child may be created under parent.
func CanParent(parent, child IssueType) bool {
	for _, t := range ChildTypes[parent] {
		if t == child {
			return true
		}
	}
	return false
}

// StatusCategory is Jira's coarse grouping of statuses (drives colours).
type StatusCategory string

const (
	CategoryTodo       StatusCategory = "todo"
	CategoryInProgress StatusCategory = "inprogress"
	CategoryDone       StatusCategory = "done"
)

// Workflow statuses used by the mock project.
const (
	StatusTodo       = "To Do"
	StatusInProgress = "In Progress"
	StatusInReview   = "In Review"
	StatusBlocked    = "Blocked"
	StatusDone       = "Done"
)

// Statuses is the ordered workflow, left to right on the board.
var Statuses = []string{StatusTodo, StatusInProgress, StatusInReview, StatusBlocked, StatusDone}

// CategoryOf maps a status name to its category.
func CategoryOf(status string) StatusCategory {
	switch status {
	case StatusDone:
		return CategoryDone
	case StatusTodo:
		return CategoryTodo
	default:
		return CategoryInProgress
	}
}

type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Initials string `json:"initials"`
	Color    string `json:"color"`
}

type Comment struct {
	ID      string `json:"id"`
	Author  User   `json:"author"`
	Body    string `json:"body"`
	Created string `json:"created"`
}

// Link types, as Jira names them from the point of view of the issue holding the link.
var LinkTypes = map[string]string{
	"blocks":           "is blocked by",
	"is blocked by":    "blocks",
	"relates to":       "relates to",
	"duplicates":       "is duplicated by",
	"is duplicated by": "duplicates",
	"clones":           "is cloned by",
	"is cloned by":     "clones",
}

type Link struct {
	Type string `json:"type"` // e.g. "blocks", "is blocked by", "relates to"
	Key  string `json:"key"`
}

type Issue struct {
	Key            string         `json:"key"`
	Summary        string         `json:"summary"`
	Type           IssueType      `json:"type"`
	Status         string         `json:"status"`
	StatusCategory StatusCategory `json:"statusCategory"`
	// State is Maestro's own state for the status (todo, inProgress, inReview,
	// blocked, done); it drives colours and automation (configuration spec 10.2).
	State       string    `json:"state"`
	Priority    string    `json:"priority"`
	Assignee    *User     `json:"assignee"`
	Reporter    User      `json:"reporter"`
	Labels      []string  `json:"labels"`
	Components  []string  `json:"components"`
	Sprint      string    `json:"sprint"`
	StoryPoints int       `json:"storyPoints"`
	ParentKey   string    `json:"parentKey"`
	ChildKeys   []string  `json:"childKeys"`
	Links       []Link    `json:"links"`
	Description string    `json:"description"`
	Comments    []Comment `json:"comments"`
	Created     string    `json:"created"`
	Updated     string    `json:"updated"`
	URL         string    `json:"url"`

	// System fields.
	FixVersions     []string      `json:"fixVersions"`
	AffectsVersions []string      `json:"affectsVersions"`
	DueDate         string        `json:"dueDate,omitempty"` // YYYY-MM-DD
	Resolution      string        `json:"resolution,omitempty"`
	ResolvedAt      string        `json:"resolvedAt,omitempty"`
	Environment     string        `json:"environment,omitempty"` // markdown
	TimeTracking    *TimeTracking `json:"timeTracking,omitempty"`
	Security        string        `json:"security,omitempty"` // security level name
	Attachments     []Attachment  `json:"attachments"`
	Watchers        int           `json:"watchers"`
	Watching        bool          `json:"watching"`
	Votes           int           `json:"votes"`
	Creator         *User         `json:"creator,omitempty"`
	StatusChangedAt string        `json:"statusChangedAt,omitempty"` // time in status

	// Atlassian custom fields: flag, sprints, start date.
	Flagged     bool   `json:"flagged"`     // or role blockedFlag: shown as blocked
	CarriedOver int    `json:"carriedOver"` // closed sprints it went through unfinished
	StartDate   string `json:"startDate,omitempty"`

	// Custom are the organization's custom fields, in config order.
	Custom []FieldValue `json:"custom"`
}

type TimeTracking struct {
	Original  string `json:"original,omitempty"` // "2d 4h"
	Remaining string `json:"remaining,omitempty"`
	Spent     string `json:"spent,omitempty"`
}

type Attachment struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType"`
	Created  string `json:"created"`
}

// FieldValue is one custom field value, ready to show.
type FieldValue struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"` // else the Jira name
	Role     string   `json:"role,omitempty"`
	Kind     string   `json:"kind"`
	Text     string   `json:"text,omitempty"`
	Items    []string `json:"items,omitempty"`
	Show     []string `json:"show"` // places
	Editable bool     `json:"editable"`
}

// FieldSpec describes an editable field for the UI (from editmeta/createmeta).
type FieldSpec struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
	Role     string   `json:"role,omitempty"`
	Default  string   `json:"default,omitempty"` // for create
	// RequiredBy says who requires it: "jira" (screen) or "team".
	RequiredBy string `json:"requiredBy,omitempty"`
}

// FieldPatch is a partial edit; nil means "leave unchanged".
type FieldPatch struct {
	FixVersions     *[]string           `json:"fixVersions,omitempty"`
	AffectsVersions *[]string           `json:"affectsVersions,omitempty"`
	DueDate         *string             `json:"dueDate,omitempty"` // "" clears
	Labels          *[]string           `json:"labels,omitempty"`
	Custom          map[string][]string `json:"custom,omitempty"` // field ID → value(s); empty clears
}

type SprintInfo struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	State   string `json:"state"` // active | future
	Goal    string `json:"goal,omitempty"`
	Start   string `json:"start,omitempty"`
	End     string `json:"end,omitempty"`
	Project string `json:"project"`
}

type VersionInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Released    bool   `json:"released"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	Overdue     bool   `json:"overdue,omitempty"`
}

// CustomColumn tells the UI which custom fields go where.
type CustomColumn struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Role  string   `json:"role,omitempty"`
	Show  []string `json:"show"`
}

type CreateDefaults struct {
	Sprint         string `json:"sprint"`         // active | next | backlog
	FixVersion     string `json:"fixVersion"`     // none | next | name
	AffectsVersion string `json:"affectsVersion"` // none | next | name
}

type Transition struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	To       string         `json:"to"`
	Category StatusCategory `json:"category"`
}

// NewIssue is the payload for creating a ticket.
type NewIssue struct {
	Type        IssueType `json:"type"`
	Summary     string    `json:"summary"`
	Description string    `json:"description"`
	ParentKey   string    `json:"parentKey"`
	Project     string    `json:"project"` // key prefix when there is no parent, e.g. "PLAT"
	Priority    string    `json:"priority"`
	AssigneeID  string    `json:"assigneeId"`
	StoryPoints int       `json:"storyPoints"`
	Labels      []string  `json:"labels"`
	Sprint      string    `json:"sprint"` // sprint name; empty = backlog

	FixVersions     []string            `json:"fixVersions"`
	AffectsVersions []string            `json:"affectsVersions"`
	Custom          map[string][]string `json:"custom"` // field ID → value(s)
}

// Meta describes the tracker's configuration for the UI.
type Meta struct {
	Statuses     []string                  `json:"statuses"`     // board order
	StatusStates map[string]string         `json:"statusStates"` // status name → Maestro state
	Sprint       string                    `json:"sprint"`       // active sprint name, "" if none
	ChildTypes   map[IssueType][]IssueType `json:"childTypes"`
	LinkTypes    map[string]string         `json:"linkTypes"` // link phrase → reciprocal
	Live         bool                      `json:"live"`      // real Jira data
	Projects     []string                  `json:"projects"`  // project keys in scope

	Sprints        []SprintInfo             `json:"sprints"`  // active then future
	Versions       map[string][]VersionInfo `json:"versions"` // project → versions
	CreateDefaults CreateDefaults           `json:"createDefaults"`
	TimeTracking   bool                     `json:"timeTracking"`
	CustomColumns  []CustomColumn           `json:"customColumns"`
}

// Tracker is what the UI talks to: the in-memory Mock or the live Jira adapter
// (internal/tracker/live).
// PRLink is a pull request to show as a web link on an issue.
type PRLink struct {
	URL    string
	Repo   string // owner/name
	Number int
	Title  string
	State  string // open | merged | closed
}

// PRLinker is implemented by trackers that can store PR links (live Jira).
type PRLinker interface {
	LinkPullRequest(key string, pr PRLink) error
}

type Tracker interface {
	Meta() Meta
	CurrentUser() User
	ListIssues() []Issue
	GetIssue(key string) (Issue, error)
	Transitions(key string) ([]Transition, error)
	Transition(key, toStatus string) (Issue, error)
	AddComment(key, body string) (Issue, error)
	Users() []User
	CreateIssue(in NewIssue) (Issue, error)
	// Link adds a link and its reciprocal on the other issue; returns both issues.
	Link(fromKey, linkType, toKey string) ([]Issue, error)
	Unlink(fromKey, linkType, toKey string) ([]Issue, error)
	// UpdateFields edits fields other than status.
	UpdateFields(key string, p FieldPatch) (Issue, error)
	// FieldSpecs lists the editable fields of an issue (versions, due date, custom).
	FieldSpecs(key string) ([]FieldSpec, error)
	// CreateFieldSpecs lists extra fields for the create dialog: required by
	// Jira or the team, or configured with show: create.
	CreateFieldSpecs(project string, t IssueType) ([]FieldSpec, error)
}
