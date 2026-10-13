package jira

import (
	"encoding/json"
	"strconv"
	"strings"
)

// These types mirror Jira's JSON closely and keep only what Maestro reads.
// Mapping to Maestro's own model (tracker.Issue) happens in the adapter, not here.

type User struct {
	AccountID    string            `json:"accountId,omitempty"` // Cloud
	Name         string            `json:"name,omitempty"`      // Data Center
	Key          string            `json:"key,omitempty"`       // Data Center
	DisplayName  string            `json:"displayName"`
	EmailAddress string            `json:"emailAddress,omitempty"`
	Active       bool              `json:"active"`
	TimeZone     string            `json:"timeZone,omitempty"`
	AvatarURLs   map[string]string `json:"avatarUrls,omitempty"`
}

// ID is the identifier Jira expects in user fields.
func (u User) ID() string {
	if u.AccountID != "" {
		return u.AccountID
	}
	return u.Name
}

type ServerInfo struct {
	BaseURL        string `json:"baseUrl"`
	Version        string `json:"version"`
	DeploymentType string `json:"deploymentType"` // Cloud | Server | DataCenter
	ServerTime     string `json:"serverTime"`
	ServerTitle    string `json:"serverTitle"`
}

type Project struct {
	ID         string      `json:"id"`
	Key        string      `json:"key"`
	Name       string      `json:"name"`
	Style      string      `json:"style,omitempty"` // classic (company-managed) | next-gen (team-managed); PRJ-03
	Simplified bool        `json:"simplified,omitempty"`
	IssueTypes []IssueType `json:"issueTypes,omitempty"`
	Lead       *User       `json:"lead,omitempty"`
}

// TeamManaged reports a team-managed (next-gen) project.
func (p Project) TeamManaged() bool { return p.Style == "next-gen" || p.Simplified }

type IssueType struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	IconURL        string `json:"iconUrl,omitempty"`
	Subtask        bool   `json:"subtask"`
	HierarchyLevel int    `json:"hierarchyLevel"` // -1 sub-task, 0 standard, 1 epic, 2+ above (HIE-02)
}

type Field struct {
	ID     string      `json:"id"`
	Key    string      `json:"key,omitempty"`
	Name   string      `json:"name"`
	Custom bool        `json:"custom"`
	Schema FieldSchema `json:"schema"`
}

type FieldSchema struct {
	Type     string `json:"type,omitempty"`
	Items    string `json:"items,omitempty"`
	System   string `json:"system,omitempty"`
	Custom   string `json:"custom,omitempty"` // e.g. com.pyxis.greenhopper.jira:gh-sprint
	CustomID int    `json:"customId,omitempty"`
}

type StatusCategory struct {
	ID   int    `json:"id"`
	Key  string `json:"key"` // new | indeterminate | done
	Name string `json:"name"`
}

// Status from GET /issue/{key} (nested statusCategory).
type Status struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	StatusCategory StatusCategory `json:"statusCategory"`
}

// StatusDef from GET /statuses/search (flat statusCategory: TODO | IN_PROGRESS | DONE).
type StatusDef struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	StatusCategory string `json:"statusCategory"`
	Scope          *struct {
		Type    string `json:"type"` // GLOBAL | PROJECT
		Project *struct {
			ID string `json:"id"`
		} `json:"project,omitempty"`
	} `json:"scope,omitempty"`
}

type Priority struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IconURL   string `json:"iconUrl,omitempty"`
	StatusCol string `json:"statusColor,omitempty"`
	IsDefault bool   `json:"isDefault,omitempty"`
}

type Resolution struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault,omitempty"`
}

type IssueLinkType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`    // e.g. Blocks
	Inward  string `json:"inward"`  // e.g. is blocked by
	Outward string `json:"outward"` // e.g. blocks
}

type Component struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Version struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Released    bool   `json:"released"`
	Archived    bool   `json:"archived"`
	Overdue     bool   `json:"overdue,omitempty"`
	StartDate   string `json:"startDate,omitempty"`
	ReleaseDate string `json:"releaseDate,omitempty"` // YYYY-MM-DD
	ProjectID   int    `json:"projectId,omitempty"`
}

type Permission struct {
	Key            string `json:"key"`
	HavePermission bool   `json:"havePermission"`
}

// ── Agile ───────────────────────────────────────────────────────

type Board struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"` // scrum | kanban | simple
	Location *struct {
		ProjectID  int    `json:"projectId"`
		ProjectKey string `json:"projectKey"`
	} `json:"location,omitempty"`
}

type BoardConfig struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Filter struct {
		ID string `json:"id"`
	} `json:"filter"`
	ColumnConfig struct {
		Columns []BoardColumn `json:"columns"`
	} `json:"columnConfig"`
	Ranking struct {
		RankCustomFieldID int `json:"rankCustomFieldId"`
	} `json:"ranking"`
	Estimation *BoardEstimation `json:"estimation,omitempty"`
}

// BoardEstimation is the board's estimation statistic.
type BoardEstimation struct {
	Type  string          `json:"type"`
	Field EstimationField `json:"field"`
}

type EstimationField struct {
	FieldID     string `json:"fieldId"`
	DisplayName string `json:"displayName"`
}

type BoardColumn struct {
	Name     string `json:"name"`
	Statuses []struct {
		ID string `json:"id"`
	} `json:"statuses"`
}

type Sprint struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	State         string `json:"state"` // active | future | closed
	StartDate     string `json:"startDate,omitempty"`
	EndDate       string `json:"endDate,omitempty"`
	CompleteDate  string `json:"completeDate,omitempty"`
	OriginBoardID int    `json:"originBoardId,omitempty"`
	Goal          string `json:"goal,omitempty"`
}

type Filter struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	JQL  string `json:"jql"`
}

// ── Issues ──────────────────────────────────────────────────────

// Issue keeps typed system fields and the raw map for custom fields, whose
// IDs are only known after discovery (e.g. story points = customfield_10016).
type Issue struct {
	ID             string                     `json:"id"`
	Key            string                     `json:"key"`
	Fields         IssueFields                `json:"-"`
	Raw            map[string]json.RawMessage `json:"-"`
	RenderedFields map[string]json.RawMessage `json:"renderedFields,omitempty"`
	Names          map[string]string          `json:"names,omitempty"` // expand=names
}

type IssueFields struct {
	Summary     string      `json:"summary"`
	Status      *Status     `json:"status"`
	IssueType   *IssueType  `json:"issuetype"`
	Priority    *Priority   `json:"priority"`
	Assignee    *User       `json:"assignee"`
	Reporter    *User       `json:"reporter"`
	Parent      *ParentRef  `json:"parent"`
	Project     *Project    `json:"project"`
	Labels      []string    `json:"labels"`
	Components  []Component `json:"components"`
	FixVersions []Version   `json:"fixVersions"`
	Resolution  *Resolution `json:"resolution"`
	// System fields.
	ResolutionDate           string          `json:"resolutiondate"`
	Versions                 []Version       `json:"versions"` // affects versions
	Environment              json.RawMessage `json:"environment"`
	TimeTracking             *TimeTracking   `json:"timetracking"`
	Security                 *SecurityLevel  `json:"security"`
	Attachment               []Attachment    `json:"attachment"`
	Watches                  *Watches        `json:"watches"`
	Votes                    *Votes          `json:"votes"`
	Creator                  *User           `json:"creator"`
	StatusCategoryChangeDate string          `json:"statuscategorychangedate"`
	Created                  string          `json:"created"`
	Updated                  string          `json:"updated"`
	DueDate                  string          `json:"duedate"`
	Description              json.RawMessage `json:"description"` // ADF (Cloud) or string (Data Center)
	IssueLinks               []IssueLink     `json:"issuelinks"`
	Subtasks                 []ParentRef     `json:"subtasks"`
}

// TimeTracking holds Jira's formatted estimates ("1d 2h").
type TimeTracking struct {
	OriginalEstimate  string `json:"originalEstimate,omitempty"`
	RemainingEstimate string `json:"remainingEstimate,omitempty"`
	TimeSpent         string `json:"timeSpent,omitempty"`
}

// SecurityLevel restricts who can see an issue.
type SecurityLevel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Attachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType"`
	Created  string `json:"created"`
	Content  string `json:"content"` // download URL; needs the same Authorization header
	Author   *User  `json:"author,omitempty"`
}

type Watches struct {
	WatchCount int  `json:"watchCount"`
	IsWatching bool `json:"isWatching"`
}

type Votes struct {
	Votes    int  `json:"votes"`
	HasVoted bool `json:"hasVoted"`
}

type ParentRef struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Fields *struct {
		Summary   string     `json:"summary"`
		Status    *Status    `json:"status"`
		IssueType *IssueType `json:"issuetype"`
	} `json:"fields,omitempty"`
}

type IssueLink struct {
	ID           string        `json:"id"`
	Type         IssueLinkType `json:"type"`
	InwardIssue  *ParentRef    `json:"inwardIssue,omitempty"`
	OutwardIssue *ParentRef    `json:"outwardIssue,omitempty"`
}

func (i *Issue) UnmarshalJSON(b []byte) error {
	var wire struct {
		ID             string                     `json:"id"`
		Key            string                     `json:"key"`
		Fields         json.RawMessage            `json:"fields"`
		RenderedFields map[string]json.RawMessage `json:"renderedFields"`
		Names          map[string]string          `json:"names"`
	}
	if err := json.Unmarshal(b, &wire); err != nil {
		return err
	}
	i.ID, i.Key, i.RenderedFields, i.Names = wire.ID, wire.Key, wire.RenderedFields, wire.Names
	if len(wire.Fields) == 0 || string(wire.Fields) == "null" {
		return nil
	}
	if err := json.Unmarshal(wire.Fields, &i.Fields); err != nil {
		return err
	}
	return json.Unmarshal(wire.Fields, &i.Raw)
}

// Number reads a numeric custom field such as story points. ok is false when
// the field is absent or empty.
func (i *Issue) Number(fieldID string) (v float64, ok bool) {
	raw, found := i.Raw[fieldID]
	if !found || string(raw) == "null" {
		return 0, false
	}
	if json.Unmarshal(raw, &v) == nil {
		return v, true
	}
	var s string // some Data Center fields arrive as strings
	if json.Unmarshal(raw, &s) == nil {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// Sprints reads the sprint custom field.
func (i *Issue) Sprints(fieldID string) []Sprint {
	var out []Sprint
	if raw, ok := i.Raw[fieldID]; ok {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

// CarriedOver counts closed sprints an unfinished issue went through.
func (i *Issue) CarriedOver(fieldID string) int {
	n := 0
	for _, s := range i.Sprints(fieldID) {
		if s.State == "closed" {
			n++
		}
	}
	return n
}

// ActiveSprint is the active sprint, else the last one.
func (i *Issue) ActiveSprint(fieldID string) *Sprint {
	ss := i.Sprints(fieldID)
	for k := range ss {
		if ss[k].State == "active" {
			return &ss[k]
		}
	}
	if len(ss) > 0 {
		return &ss[len(ss)-1]
	}
	return nil
}

type Transition struct {
	ID     string                     `json:"id"`
	Name   string                     `json:"name"`
	To     Status                     `json:"to"`
	Fields map[string]TransitionField `json:"fields,omitempty"` // expand=transitions.fields (WFL-05)
}

type TransitionField struct {
	Required      bool            `json:"required"`
	Name          string          `json:"name"`
	Schema        FieldSchema     `json:"schema"`
	AllowedValues json.RawMessage `json:"allowedValues,omitempty"`
}

// RequiredFields lists the fields the transition screen requires.
func (t Transition) RequiredFields() []string {
	var out []string
	for id, f := range t.Fields {
		if f.Required {
			out = append(out, id)
		}
	}
	return out
}

type Comment struct {
	ID           string          `json:"id"`
	Author       *User           `json:"author"`
	Body         json.RawMessage `json:"body"` // ADF (Cloud) or string (DC)
	RenderedBody string          `json:"renderedBody,omitempty"`
	Created      string          `json:"created"`
	Updated      string          `json:"updated"`
}

// CreateMetaField from GET /issue/createmeta/{project}/issuetypes/{id} (FLD-11).
type CreateMetaField struct {
	FieldID       string          `json:"fieldId"`
	Key           string          `json:"key"`
	Name          string          `json:"name"`
	Required      bool            `json:"required"`
	HasDefault    bool            `json:"hasDefaultValue"`
	Schema        FieldSchema     `json:"schema"`
	AllowedValues json.RawMessage `json:"allowedValues,omitempty"`
}

// CreatedIssue is the response of POST /issue.
type CreatedIssue struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Self string `json:"self"`
}

// RemoteLink is a web link on an issue, used for pull requests.
type RemoteLink struct {
	GlobalID string           `json:"globalId,omitempty"` // same ID updates the link
	Object   RemoteLinkObject `json:"object"`
}

type RemoteLinkObject struct {
	URL     string          `json:"url"`
	Title   string          `json:"title"`
	Summary string          `json:"summary,omitempty"`
	Icon    *RemoteLinkIcon `json:"icon,omitempty"`
	Status  *struct {
		Resolved bool `json:"resolved"`
	} `json:"status,omitempty"`
}

type RemoteLinkIcon struct {
	URL   string `json:"url16x16"`
	Title string `json:"title,omitempty"`
}
