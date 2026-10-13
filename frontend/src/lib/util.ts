import type { Flow, Issue, IssueType } from '../types'

export function timeAgo(iso: string): string {
  if (!iso) return ''
  const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000)
  if (s < 45) return 'just now'
  if (s < 3600) return `${Math.round(s / 60)}m ago`
  if (s < 86400) return `${Math.round(s / 3600)}h ago`
  if (s < 86400 * 30) return `${Math.round(s / 86400)}d ago`
  return new Date(iso).toLocaleDateString()
}

export function formatDate(iso: string): string {
  if (!iso) return ''
  return new Date(iso).toLocaleString(undefined, { month: 'short', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit' })
}

/** CSS colour variable for a status name. */
// Status name → Maestro state (todo | inProgress | inReview | blocked | done),
// loaded from the backend so any Jira workflow gets the right colours.
let statusStates: Record<string, string> = {}
export function setStatusStates(m: Record<string, string>) { statusStates = m ?? {} }

const FALLBACK_STATE: Record<string, string> = { 'To Do': 'todo', 'In Progress': 'inProgress', 'In Review': 'inReview', Blocked: 'blocked', Done: 'done' }

export function stateOf(status: string): string {
  return statusStates[status] ?? FALLBACK_STATE[status] ?? (['todo', 'inProgress', 'inReview', 'blocked', 'done'].includes(status) ? status : 'inProgress')
}

const TONE: Record<string, string> = { todo: 'todo', inProgress: 'progress', inReview: 'review', blocked: 'blocked', done: 'done' }

/** CSS tone for a status name (or a Maestro state). */
export function statusTone(status: string): string {
  return TONE[stateOf(status)] ?? 'todo'
}

export const typeMeta: Record<IssueType, { label: string; tone: string }> = {
  initiative: { label: 'Initiative', tone: 'initiative' },
  epic: { label: 'Epic', tone: 'epic' },
  story: { label: 'Story', tone: 'story' },
  task: { label: 'Task', tone: 'task' },
  bug: { label: 'Bug', tone: 'bug' },
  subtask: { label: 'Sub-task', tone: 'task' },
}

export function flowProgress(f?: Flow): number {
  if (!f) return 0
  return f.stages.filter(s => s.state === 'done').length / f.stages.length
}

export function activeStage(f?: Flow) {
  return f?.stages.find(s => s.state === 'active')
}

/** Walk up the parent chain: [initiative, epic, ...] excluding the issue itself. */
export function ancestors(issue: Issue, byKey: Record<string, Issue>): Issue[] {
  const out: Issue[] = []
  let p = byKey[issue.parentKey]
  while (p) {
    out.unshift(p)
    p = byKey[p.parentKey]
  }
  return out
}

/** Work items are what you actually pick up: stories, tasks, bugs and sub-tasks. */
export const isWork = (i: Issue) => i.type !== 'initiative' && i.type !== 'epic'

/** Planning containers (initiatives, epics) group work but aren't worked on directly. */
export const isContainer = (i: Issue) => !isWork(i)

/** All stories/tasks/bugs under an issue (sub-tasks roll up into their parent). */
export function leaves(issue: Issue, byKey: Record<string, Issue>): Issue[] {
  if (isWork(issue)) return [issue]
  return issue.childKeys.map(k => byKey[k]).filter(Boolean).flatMap(k => leaves(k, byKey))
}

/** Human label for a link type, Jira style ("blocks" → "Blocks"). */
export const linkLabel = (t: string) => t.charAt(0).toUpperCase() + t.slice(1)

export function cx(...c: (string | false | null | undefined)[]) {
  return c.filter(Boolean).join(' ')
}

/** Branch name Maestro suggests for a ticket (mirrors flow.BranchName in Go). */
export function branchFor(key: string, summary: string, type: string) {
  const slug = summary.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').split('-').slice(0, 5).join('-')
  return `${type === 'bug' ? 'fix' : 'feature'}/${key}-${slug}`
}

/** "Nov 1, 2026" for a YYYY-MM-DD date (no time zone shift). */
export function formatDay(d: string): string {
  const [y, m, day] = d.split('-').map(Number)
  if (!y || !m || !day) return d
  return new Date(y, m - 1, day).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })
}

/** Days until a YYYY-MM-DD date (negative when past), in local time. */
export function daysUntil(d: string): number {
  const [y, m, day] = d.split('-').map(Number)
  const due = new Date(y, m - 1, day).getTime()
  const today = new Date(); today.setHours(0, 0, 0, 0)
  return Math.round((due - today.getTime()) / 86400000)
}

/** CSS class for a due date: overdue (unfinished, past) or due soon (≤ 2 days). */
export function dueClass(i: Issue): string {
  if (!i.dueDate || i.statusCategory === 'done') return 'due'
  const n = daysUntil(i.dueDate)
  return n < 0 ? 'due overdue' : n <= 2 ? 'due soon' : 'due'
}

export function dueLabel(d: string): string {
  const n = daysUntil(d)
  if (n === 0) return 'due today'
  if (n === 1) return 'due tomorrow'
  if (n < 0) return `${-n}d overdue`
  return n < 14 ? `due in ${n}d` : `due ${formatDay(d)}`
}

/** "3d", "5h" since an ISO time. */
export function timeSince(iso: string): string {
  const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000)
  if (s < 3600) return `${Math.max(1, Math.round(s / 60))}m`
  if (s < 86400) return `${Math.round(s / 3600)}h`
  return `${Math.round(s / 86400)}d`
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}
