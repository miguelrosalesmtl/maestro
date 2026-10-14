package config

// Schema for config.yaml (personal) and maestro.team.yaml (shared).
//
// Spec references: docs/jira-configuration-spec.md (CON-, PRJ-, HIE-, FLD-, WFL-,
// LNK-, BRD-, USR-, DEV-, AUT-, TRM-, LOC-).
//
// Neither file may contain a secret (configuration spec rule 4.7); credentials
// are referenced through jira.auth.credentialRef and live in the OS keychain.

// CurrentVersion is the only schema version this build understands.
const CurrentVersion = 1

type Config struct {
	Version int `yaml:"version"`
	// TeamFile points at the shared team file. Only read from the personal file.
	TeamFile string `yaml:"teamFile,omitempty"`

	Jira       Jira       `yaml:"jira"`
	Projects   []Project  `yaml:"projects,omitempty"`
	Hierarchy  Hierarchy  `yaml:"hierarchy,omitempty"`
	Fields     Fields     `yaml:"fields,omitempty"`
	Workflow   Workflow   `yaml:"workflow,omitempty"`
	Links      Links      `yaml:"links,omitempty"`
	Team       Team       `yaml:"team,omitempty"`
	Defaults   Defaults   `yaml:"defaults,omitempty"`
	Dev        Dev        `yaml:"dev,omitempty"`
	Automation Automation `yaml:"automation,omitempty"`
	Terminal   Terminal   `yaml:"terminal,omitempty"`
	Agent      Agent      `yaml:"agent,omitempty"`
	UI         UI         `yaml:"ui,omitempty"`
	Log        Log        `yaml:"log,omitempty"`
	Sprints    Sprints    `yaml:"sprints,omitempty"`
	Versions   Versions   `yaml:"versions,omitempty"`
}

// Log configures the log file (see internal/applog). MAESTRO_LOG_LEVEL overrides it.
type Log struct {
	Level string `yaml:"level,omitempty"` // debug | info | warn | error
}

// ── Jira connection ─────────────────────────────────────────────

type Deployment string

const (
	Cloud      Deployment = "cloud"
	DataCenter Deployment = "datacenter"
)

type Transport string

const (
	TransportREST Transport = "rest"
	TransportMCP  Transport = "mcp"
	TransportMock Transport = "mock"
)

type AuthMethod string

const (
	AuthAPIToken       AuthMethod = "apiToken"
	AuthScopedAPIToken AuthMethod = "scopedApiToken"
	AuthOAuth          AuthMethod = "oauth" // (v2)
	AuthPAT            AuthMethod = "pat"
)

type Jira struct {
	Site       string     `yaml:"site,omitempty"`       // CON-01
	Deployment Deployment `yaml:"deployment,omitempty"` // CON-02, discovered
	CloudID    string     `yaml:"cloudId,omitempty"`    // discovered
	Transport  Transport  `yaml:"transport,omitempty"`
	Auth       Auth       `yaml:"auth,omitempty"`
	HTTP       HTTP       `yaml:"http,omitempty"`
	Proxy      string     `yaml:"proxy,omitempty"`
	TLS        TLS        `yaml:"tls,omitempty"`
	Sync       Sync       `yaml:"sync,omitempty"`
	WriteBack  WriteBack  `yaml:"writeBack,omitempty"`
	MCP        MCP        `yaml:"mcp,omitempty"` // CON-03, only for transport: mcp
}

type Auth struct {
	Method        AuthMethod `yaml:"method,omitempty"`
	Email         string     `yaml:"email,omitempty"` // not a secret
	CredentialRef string     `yaml:"credentialRef,omitempty"`
	TokenExpires  string     `yaml:"tokenExpires,omitempty"` // YYYY-MM-DD
}

type HTTP struct {
	TimeoutSeconds int `yaml:"timeoutSeconds,omitempty"`
	MaxConcurrency int `yaml:"maxConcurrency,omitempty"`
}

type TLS struct {
	CAFile        string `yaml:"caFile,omitempty"`
	AllowInsecure bool   `yaml:"allowInsecure,omitempty"` // AUTH rule 4: http:// for DC only
}

type Sync struct {
	IntervalSeconds   int  `yaml:"intervalSeconds,omitempty"`
	BurstSeconds      int  `yaml:"burstSeconds,omitempty"`
	FullResyncMinutes int  `yaml:"fullResyncMinutes,omitempty"`
	DiscoveryTTLHours int  `yaml:"discoveryTtlHours,omitempty"`
	Webhooks          bool `yaml:"webhooks,omitempty"` // not in v1
}

type WriteBack struct {
	RemoteLinks        *bool `yaml:"remoteLinks,omitempty"` // nil = decide from DEV-10
	TransitionComments bool  `yaml:"transitionComments,omitempty"`
}

type MCP struct {
	Command []string `yaml:"command,omitempty"`
	URL     string   `yaml:"url,omitempty"`
}

// ── Projects and repositories ───────────────────────────────────

type Project struct {
	Key   string `yaml:"key"`             // PRJ-01
	Board int    `yaml:"board,omitempty"` // BRD-01
	Repos []Repo `yaml:"repos,omitempty"`
}

type Repo struct {
	Name       string   `yaml:"name"`                 // PRJ-10, owner/name
	Host       string   `yaml:"host,omitempty"`       // PRJ-12
	Path       string   `yaml:"path,omitempty"`       // PRJ-11, may start with ~
	BaseBranch string   `yaml:"baseBranch,omitempty"` // PRJ-13
	Components []string `yaml:"components,omitempty"` // PRJ-14
}

// ── Hierarchy, fields, workflow, links ──────────────────────────

// Role is the Maestro role of a Jira issue type (HIE-03).
type Role string

const (
	RoleInitiative Role = "initiative"
	RoleEpic       Role = "epic"
	RoleStory      Role = "story"
	RoleTask       Role = "task"
	RoleBug        Role = "bug"
	RoleSubtask    Role = "subtask"
)

var Roles = []Role{RoleInitiative, RoleEpic, RoleStory, RoleTask, RoleBug, RoleSubtask}

type Hierarchy struct {
	InitiativeProject string          `yaml:"initiativeProject,omitempty"` // PRJ-05
	Types             []IssueTypeRole `yaml:"types,omitempty"`             // HIE-03
	Children          map[Role][]Role `yaml:"children,omitempty"`          // HIE-04 (limits only)
}

// IssueTypeRole maps a Jira issue type (by ID, or by name for hand-written files) to a role.
type IssueTypeRole struct {
	ID   string `yaml:"id,omitempty"`
	Name string `yaml:"name,omitempty"`
	Role Role   `yaml:"role"`
}

type Fields struct {
	StoryPoints      string   `yaml:"storyPoints,omitempty"`      // FLD-03
	Parent           string   `yaml:"parent,omitempty"`           // FLD-04
	InitiativeParent string   `yaml:"initiativeParent,omitempty"` // FLD-05
	Sprint           string   `yaml:"sprint,omitempty"`           // FLD-06
	Priorities       []string `yaml:"priorities,omitempty"`       // FLD-07, order only
	Team             string   `yaml:"team,omitempty"`             // FLD-14
	FixVersion       string   `yaml:"fixVersion,omitempty"`       // FLD-15
	// Custom are the organization's custom fields Maestro shows.
	Custom []CustomField `yaml:"custom,omitempty"`
}

// StoryPointsAuto means "use the board's estimation field".
const StoryPointsAuto = "auto"

// CustomField configures one organization custom field.
type CustomField struct {
	ID         string   `yaml:"id,omitempty"`   // e.g. customfield_10050
	Name       string   `yaml:"name,omitempty"` // e.g. "Severity" (when no ID)
	Show       []string `yaml:"show,omitempty"` // view | create | column | card | filter
	Role       string   `yaml:"role,omitempty"`
	Default    string   `yaml:"default,omitempty"`
	DefaultFor []string `yaml:"defaultFor,omitempty"` // issue roles; empty = all
	RequiredOn []string `yaml:"requiredOn,omitempty"` // create | done | inReview
	Label      string   `yaml:"label,omitempty"`
}

// Ref is how the field is named in config (ID, else name).
func (f CustomField) Ref() string {
	if f.ID != "" {
		return f.ID
	}
	return f.Name
}

// Shows reports whether the field appears in a place (default: the Jira view).
func (f CustomField) Shows(place string) bool {
	if len(f.Show) == 0 {
		return place == "view"
	}
	for _, s := range f.Show {
		if s == place {
			return true
		}
	}
	return false
}

// Custom field roles and places.
var (
	CustomRoles  = []string{"severity", "acceptanceCriteria", "qaAssignee", "customer", "rootCause", "releaseNotes", "blockedFlag"}
	CustomPlaces = []string{"view", "create", "column", "card", "filter"}
	RequiredOns  = []string{"create", "done", "inReview"}
)

// State is a Maestro state (configuration spec 10.2).
type State string

const (
	StateTodo       State = "todo"
	StateInProgress State = "inProgress"
	StateInReview   State = "inReview"
	StateBlocked    State = "blocked"
	StateDone       State = "done"
)

var States = []State{StateTodo, StateInProgress, StateInReview, StateBlocked, StateDone}

type Workflow struct {
	// States maps a Jira status (name or ID) to a Maestro state (WFL-10).
	States            map[string]State `yaml:"states,omitempty"`
	Order             []string         `yaml:"order,omitempty"`             // WFL-03
	DefaultResolution string           `yaml:"defaultResolution,omitempty"` // WFL-06
}

type Links struct {
	Blocking string   `yaml:"blocking,omitempty"` // LNK-02
	Visible  []string `yaml:"visible,omitempty"`  // LNK-03
}

type Team struct {
	Members []string `yaml:"members,omitempty"` // USR-02, account IDs
}

type Defaults struct {
	EpicProject string        `yaml:"epicProject,omitempty"` // PRJ-04
	Priority    string        `yaml:"priority,omitempty"`    // FLD-08
	Create      CreateDefault `yaml:"create,omitempty"`      // FLD-12
}

type CreateDefault struct {
	AssignToMe     *bool  `yaml:"assignToMe,omitempty"`
	Sprint         string `yaml:"sprint,omitempty"`         // active | next | backlog
	FixVersion     string `yaml:"fixVersion,omitempty"`     // none | next | "<version name>"
	AffectsVersion string `yaml:"affectsVersion,omitempty"` // none | next | "<version name>"
}

// ── Dev, automation, terminal, agent ────────────────────────────

type Dev struct {
	ScanDirs   []string `yaml:"scanDirs,omitempty"`   // 4.4
	KeyPattern string   `yaml:"keyPattern,omitempty"` // DEV-09
	Branch     Branch   `yaml:"branch,omitempty"`
	PR         PR       `yaml:"pr,omitempty"`
	Checks     Checks   `yaml:"checks,omitempty"`
	Review     Review   `yaml:"review,omitempty"`
	GitHub     GitHub   `yaml:"github,omitempty"`
}

type Branch struct {
	Template  string            `yaml:"template,omitempty"`  // DEV-01
	Prefix    map[string]string `yaml:"prefix,omitempty"`    // DEV-02, role → prefix, "default" key
	SlugWords int               `yaml:"slugWords,omitempty"` // DEV-03
}

type PR struct {
	TitleTemplate string `yaml:"titleTemplate,omitempty"` // DEV-04
	BodyTemplate  string `yaml:"bodyTemplate,omitempty"`  // DEV-05
	MergeMethod   string `yaml:"mergeMethod,omitempty"`   // DEV-06
}

type Checks struct {
	Required []string `yaml:"required,omitempty"` // DEV-07
}

type Review struct {
	RequiredApprovals int `yaml:"requiredApprovals,omitempty"` // DEV-08
}

type GitHub struct {
	Enabled     *bool  `yaml:"enabled,omitempty"`     // watch pull requests (default true when repos are configured)
	Auth        string `yaml:"auth,omitempty"`        // gh (use `gh auth token`) | token (keychain)
	APIURL      string `yaml:"apiUrl,omitempty"`      // https://api.github.com, or GitHub Enterprise …/api/v3
	PollSeconds int    `yaml:"pollSeconds,omitempty"` // how often to check pull requests
}

func (g GitHub) IsEnabled() bool { return g.Enabled == nil || *g.Enabled }

// Trigger is a dev event that can drive an automation rule.
type Trigger string

const (
	TriggerBranchCreated    Trigger = "branch_created"
	TriggerPROpened         Trigger = "pr_opened"
	TriggerChangesRequested Trigger = "changes_requested"
	TriggerReviewApproved   Trigger = "review_approved"
	TriggerPRMerged         Trigger = "pr_merged"
	TriggerCIFailed         Trigger = "ci_failed"
)

var Triggers = []Trigger{TriggerBranchCreated, TriggerPROpened, TriggerChangesRequested, TriggerReviewApproved, TriggerPRMerged, TriggerCIFailed}

type Automation struct {
	Rules         []Rule `yaml:"rules,omitempty"` // AUT-01
	AssignOnStart *bool  `yaml:"assignOnStart,omitempty"`
	RollUp        RollUp `yaml:"rollUp,omitempty"` // AUT-04
	// Rules move only tickets assigned to you, for your own branches and pull
	// requests. AnyAssignee and AnyAuthor widen that; DryRun only logs.
	// A ticket in a Done status is never moved by a rule.
	AnyAssignee bool `yaml:"anyAssignee,omitempty"`
	AnyAuthor   bool `yaml:"anyAuthor,omitempty"`
	DryRun      bool `yaml:"dryRun,omitempty"`
}

type Rule struct {
	Trigger Trigger `yaml:"trigger"`
	// To is a Maestro state ("inReview") or an exact Jira status name or ID.
	To         string   `yaml:"to"`
	Enabled    *bool    `yaml:"enabled,omitempty"`    // nil = true
	Projects   []string `yaml:"projects,omitempty"`   // AUT-02
	IssueTypes []string `yaml:"issueTypes,omitempty"` // AUT-02
}

// IsEnabled treats an omitted "enabled" as true.
func (r Rule) IsEnabled() bool { return r.Enabled == nil || *r.Enabled }

type RollUp struct {
	SubtasksToParent bool `yaml:"subtasksToParent,omitempty"`
}

type Terminal struct {
	Shell    string            `yaml:"shell,omitempty"`    // TRM-01
	Env      map[string]string `yaml:"env,omitempty"`      // TRM-03
	Startup  []string          `yaml:"startup,omitempty"`  // commands run when a terminal opens
	Commands []QuickCommand    `yaml:"commands,omitempty"` // TRM-04
	Watch    []string          `yaml:"watch,omitempty"`    // TRM-05
	Notify   TerminalNotify    `yaml:"notify,omitempty"`
	// Projects adds env and startup commands for one project's ticket terminals
	// (a map, so the team and personal files merge per project).
	Projects map[string]ProjectTerminal `yaml:"projects,omitempty"`
}

// ProjectTerminal is the extra setup for one project's ticket terminals.
type ProjectTerminal struct {
	Env     map[string]string `yaml:"env,omitempty" json:"env"`
	Startup []string          `yaml:"startup,omitempty" json:"startup"`
}

// TerminalNotify controls the activity light and chirp for ticket terminals.
type TerminalNotify struct {
	Enabled     *bool    `yaml:"enabled,omitempty"`     // show activity and notify (default true)
	Sound       *bool    `yaml:"sound,omitempty"`       // chirp (default true)
	MinSeconds  int      `yaml:"minSeconds,omitempty"`  // notify "finished" only for commands this long
	WaitSeconds int      `yaml:"waitSeconds,omitempty"` // silence before a running command counts as waiting
	Ignore      []string `yaml:"ignore,omitempty"`      // interactive programs that never notify
}

func (n TerminalNotify) IsEnabled() bool { return n.Enabled == nil || *n.Enabled }
func (n TerminalNotify) HasSound() bool  { return n.Sound == nil || *n.Sound }

type QuickCommand struct {
	Label   string `yaml:"label" json:"label"`
	Command string `yaml:"command" json:"command"`
}

type Agent struct {
	MCP                AgentMCP `yaml:"mcp,omitempty"`
	ShareTicketContent bool     `yaml:"shareTicketContent,omitempty"`
	ShareSecuredIssues bool     `yaml:"shareSecuredIssues,omitempty"`
}

// Sprints is.3.
type Sprints struct {
	ClosedHistory   int    `yaml:"closedHistory,omitempty"`
	VelocitySprints int    `yaml:"velocitySprints,omitempty"`
	AllowManage     bool   `yaml:"allowManage,omitempty"` // (v1: read only)
	NameTemplate    string `yaml:"nameTemplate,omitempty"`
}

// Versions is.4.
type Versions struct {
	Pattern                 string     `yaml:"pattern,omitempty"`
	RequireFixVersionOnDone bool       `yaml:"requireFixVersionOnDone,omitempty"`
	FromBranch              FromBranch `yaml:"fromBranch,omitempty"`
	AllowManage             bool       `yaml:"allowManage,omitempty"` // (v1: read only)
	ReleaseNotesTemplate    string     `yaml:"releaseNotesTemplate,omitempty"`
}

type FromBranch struct {
	Enabled  bool   `yaml:"enabled,omitempty"`
	Template string `yaml:"template,omitempty"`
}

type AgentMCP struct {
	Register *bool  `yaml:"register,omitempty"`
	Server   string `yaml:"server,omitempty"` // atlassian-remote | mcp-atlassian | none
	URL      string `yaml:"url,omitempty"`
}

// ── UI (local preferences) ──────────────────────────────────────

type UI struct {
	DefaultView string            `yaml:"defaultView,omitempty"` // LOC-02
	TimeZone    string            `yaml:"timeZone,omitempty"`    // CON-06
	Keybindings map[string]string `yaml:"keybindings,omitempty"` // LOC-01
	Board       UIBoard           `yaml:"board,omitempty"`
	JiraView    UIJiraView        `yaml:"jiraView,omitempty"`
}

type UIBoard struct {
	ShowSubtasks bool `yaml:"showSubtasks,omitempty"` // BRD-05
}

type UIJiraView struct {
	Fields []string `yaml:"fields,omitempty"` // FLD-13
}
