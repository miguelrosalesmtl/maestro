package config

// Default returns the built-in configuration. Files are layered on top of it,
// so every value here is what Maestro uses when no file says otherwise.
func Default() *Config {
	return &Config{
		Version: CurrentVersion,
		Jira: Jira{
			Transport: TransportREST,
			HTTP:      HTTP{TimeoutSeconds: 20, MaxConcurrency: 4},
			Sync: Sync{
				IntervalSeconds:   60,
				BurstSeconds:      15,
				FullResyncMinutes: 15,
				DiscoveryTTLHours: 24,
			},
		},
		Links:    Links{Blocking: "Blocks"},
		Fields:   Fields{StoryPoints: StoryPointsAuto},
		Defaults: Defaults{Create: CreateDefault{Sprint: "active", FixVersion: "none", AffectsVersion: "none"}},
		Sprints:  Sprints{ClosedHistory: 3, VelocitySprints: 3, NameTemplate: "{board} Sprint {n}"},
		Versions: Versions{
			Pattern:              `\d+\.\d+(\.\d+)?`,
			FromBranch:           FromBranch{Template: "release/{version}"},
			ReleaseNotesTemplate: "- [{key}] {summary}",
		},
		Dev: Dev{
			KeyPattern: `[A-Z][A-Z0-9]+-\d+`,
			Branch: Branch{
				Template:  "{prefix}/{key}-{slug}",
				Prefix:    map[string]string{string(RoleBug): "fix", "default": "feature"},
				SlugWords: 5,
			},
			PR: PR{
				TitleTemplate: "[{key}] {summary}",
				BodyTemplate:  "Jira: {url}",
				MergeMethod:   "squash",
			},
			Review: Review{RequiredApprovals: 1},
			GitHub: GitHub{Auth: "gh", APIURL: "https://api.github.com"},
		},
		Automation: Automation{
			Rules: []Rule{
				{Trigger: TriggerBranchCreated, To: string(StateInProgress)},
				{Trigger: TriggerPROpened, To: string(StateInReview)},
				{Trigger: TriggerChangesRequested, To: string(StateInProgress)},
				{Trigger: TriggerPRMerged, To: string(StateDone)},
				{Trigger: TriggerCIFailed, To: string(StateBlocked), Enabled: ptr(false)},
			},
			AssignOnStart: ptr(true),
		},
		Terminal: Terminal{
			Watch: []string{"git checkout -b", "git switch -c", "git push", "gh pr create", "gh pr merge"},
			Notify: TerminalNotify{MinSeconds: 10, WaitSeconds: 8, Ignore: []string{
				"vim", "nvim", "vi", "nano", "emacs", "less", "more", "man", "ssh", "mosh", "top", "htop", "btop",
				"tmux", "screen", "watch", "tail", "k9s", "lazygit", "lazydocker", "fzf", "psql", "mysql", "sqlite3", "redis-cli"}},
		},
		Agent: Agent{
			MCP: AgentMCP{Server: "atlassian-remote", URL: "https://mcp.atlassian.com/v1/mcp"},
		},
		UI:  UI{DefaultView: "mine"},
		Log: Log{Level: "info"},
	}
}

// DefaultChildren is the hierarchy Maestro supports (HIE-04). A file may only
// narrow it, never add relations.
var DefaultChildren = map[Role][]Role{
	RoleInitiative: {RoleEpic},
	RoleEpic:       {RoleStory, RoleTask, RoleBug},
	RoleStory:      {RoleSubtask},
	RoleTask:       {RoleSubtask},
	RoleBug:        {RoleSubtask},
	RoleSubtask:    {},
}

func ptr[T any](v T) *T { return &v }
