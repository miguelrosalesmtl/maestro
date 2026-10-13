// Plain shapes of what the Go backend returns (see internal/tracker, internal/flow).

export type IssueType = 'initiative' | 'epic' | 'story' | 'task' | 'bug' | 'subtask'
export type StatusCategory = 'todo' | 'inprogress' | 'done'
export type StageState = 'pending' | 'active' | 'done' | 'failed'

export interface User {
  id: string
  name: string
  initials: string
  color: string
}

export interface Comment {
  id: string
  author: User
  body: string
  created: string
}

export interface Link {
  type: string // "blocks" | "is blocked by" | "relates to" | ...
  key: string
}

/** One custom field value, ready to show. */
export interface FieldValue {
  id: string
  label: string
  role?: string
  kind: string
  text?: string
  items?: string[]
  show: string[] // view | create | column | card | filter
  editable: boolean
}

/** An editable field (from Jira's edit/create screens). */
export interface FieldSpec {
  id: string
  label: string
  kind: string
  required: boolean
  options?: string[]
  role?: string
  default?: string
  requiredBy?: 'jira' | 'team'
}

export interface FieldPatch {
  fixVersions?: string[]
  affectsVersions?: string[]
  dueDate?: string
  labels?: string[]
  custom?: Record<string, string[]>
}

export interface SprintInfo { id: number; name: string; state: 'active' | 'future' | 'closed'; goal?: string; start?: string; end?: string; project: string }

/** A Jira agile board (Board view → Find board). */
export interface BoardInfo { id: number; name: string; type: 'scrum' | 'kanban' | 'simple' | string; project?: string }
export interface JiraBoardColumn { name: string; keys: string[]; statuses: string[] }
/** One board showing one sprint (or a kanban board's open work). */
export interface JiraBoard { board: BoardInfo; sprint: SprintInfo | null; columns: JiraBoardColumn[]; issues: Issue[]; truncated: boolean }
export interface VersionInfo { id: string; name: string; released: boolean; releaseDate?: string; overdue?: boolean }
export interface CustomColumn { id: string; label: string; role?: string; show: string[] }
export interface CreateDefaults { sprint: string; fixVersion: string; affectsVersion: string }

export interface NewIssue {
  type: IssueType
  summary: string
  description: string
  parentKey: string
  project: string
  priority: string
  assigneeId: string
  storyPoints: number
  labels: string[]
  sprint: string
  fixVersions: string[]
  affectsVersions: string[]
  custom: Record<string, string[]>
}

export interface Issue {
  key: string
  summary: string
  type: IssueType
  status: string
  statusCategory: StatusCategory
  state: string // todo | inProgress | inReview | blocked | done
  security?: string // security level name; never leak the summary of a secured issue
  // Versions, dates, flags, security and custom fields
  fixVersions: string[]
  affectsVersions: string[]
  dueDate?: string
  resolution?: string
  resolvedAt?: string
  environment?: string
  timeTracking?: { original?: string; remaining?: string; spent?: string }
  attachments: { id: string; name: string; size: number; mimeType: string; created: string }[]
  watchers: number
  watching: boolean
  votes: number
  creator?: User
  statusChangedAt?: string
  flagged: boolean
  carriedOver: number
  startDate?: string
  custom: FieldValue[]
  priority: string
  assignee: User | null
  reporter: User
  labels: string[]
  components: string[]
  sprint: string
  storyPoints: number
  parentKey: string
  childKeys: string[]
  links: Link[]
  description: string
  comments: Comment[]
  created: string
  updated: string
  url: string
}

export interface Transition {
  id: string
  name: string
  to: string
  category: StatusCategory
}

export interface Stage {
  id: string
  label: string
  state: StageState
  detail: string
  at: string
  trigger: string
}

export interface Commit {
  sha: string
  message: string
  author: string
  at: string
  adds: number
  dels: number
}

export interface Check {
  name: string
  state: StageState // active = running or queued
  url?: string
  required?: boolean
}

export interface PullRequest {
  number: number
  title: string
  url: string
  state: 'open' | 'merged' | 'closed'
  checks: Check[]
  approvals: number
  reviewers: string[]
  repo?: string
  draft?: boolean
  author?: string
  head?: string
  base?: string
  requiredApprovals?: number
  approvedBy?: string[]
  changesRequested?: string[]
  mergedAt?: string
  updatedAt?: string
  live?: boolean // from GitHub, not simulated
}

/** Pull request watching (GitHub + local git), as the backend reports it. */
export interface GitHubStatus {
  state: 'off' | 'connecting' | 'ok' | 'error'
  host: string
  user: string
  repos: string[] | null
  error: string
  lastSync: string
  prs: number
}

export interface Flow {
  issueKey: string
  repo: string
  branch: string
  started: boolean
  stages: Stage[]
  commits: Commit[]
  pr: PullRequest | null
  live?: boolean // stages come from GitHub and git
}

export interface FlowEvent {
  id: string
  issueKey: string
  kind: 'dev' | 'jira' | 'automation' | 'term'
  title: string
  detail: string
  at: string
}

export interface Rule {
  id: string
  trigger: string
  label: string
  toStatus: string
  enabled: boolean
}

export interface StepResult {
  flow: Flow
  issue: Issue
  events: FlowEvent[]
}

export interface Snapshot {
  me: User
  issues: Issue[]
  statuses: string[]
  rules: Rule[]
  activity: FlowEvent[]
  flows: Flow[]
  users: User[]
  sprint: string
  childTypes: Record<IssueType, IssueType[]>
  linkTypes: Record<string, string>
  statusStates: Record<string, string>
  projects: string[]
  live: boolean
  sprints: SprintInfo[]
  versions: Record<string, VersionInfo[]>
  createDefaults: CreateDefaults
  timeTracking: boolean
  customColumns: CustomColumn[]
}

/** Where the data comes from (backend connect.go). */
export interface Connection {
  mode: 'live' | 'demo' | 'connecting'
  state: string // live mode: syncing | live | offline | error
  configured: boolean
  site: string
  deployment: string
  error?: string
  lastSync?: string
  issues: number
}

/** Terminal activity of one session (backend internal/term/activity.go). */
export interface TermActivity {
  session: string
  ticket: string
  state: 'idle' | 'running' | 'waiting'
  command?: string
  since?: string
  finished?: boolean
  seconds?: number
  notify?: boolean
  sound?: boolean
}

/** What the UI shows for a session: running/waiting, or "finished" until seen. */
export interface TermLightState {
  state: 'idle' | 'running' | 'waiting' | 'finished'
  command?: string
  since?: string
  seconds?: number
}

export type View = 'mine' | 'focus' | 'board' | 'roadmap' | 'automations' | 'freestyle'

/** A terminal that belongs to no ticket (Freestyle view). id is "free:N". */
export interface FreeSession {
  id: string
  name: string
}
