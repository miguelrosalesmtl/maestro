import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { api } from './api'
import { EventsOn } from '../wailsjs/runtime/runtime'
import { reportError } from './lib/errors'
import type { Connection, FreeSession, GitHubStatus, CreateDefaults, CustomColumn, FieldPatch, SprintInfo, TermActivity, TermLightState, VersionInfo, Flow, FlowEvent, Issue, IssueType, NewIssue, Rule, StepResult, User, View } from './types'
import { isWork, setStatusStates } from './lib/util'
import { chirp, loadMuted, saveMuted } from './lib/chirp'
import { WindowSetTitle } from '../wailsjs/runtime/runtime'

export interface Toast {
  id: string
  tone: 'automation' | 'dev' | 'jira' | 'error'
  title: string
  detail?: string
  issueKey?: string
  onClick?: () => void
}

interface Store {
  ready: boolean
  me: User | null
  issues: Record<string, Issue>
  flows: Record<string, Flow>
  statuses: string[]
  rules: Rule[]
  activity: FlowEvent[]
  toasts: Toast[]
  users: User[]
  sprint: string
  childTypes: Record<IssueType, IssueType[]>
  linkTypes: Record<string, string>

  view: View
  setView: (v: View) => void
  focusKey: string
  setFocusKey: (k: string) => void
  jiraKey: string | null
  openJira: (k: string | null) => void
  paletteOpen: boolean
  setPaletteOpen: (o: boolean) => void
  termOpen: boolean
  setTermOpen: (o: boolean) => void
  /** Terminal sessions: the ticket key, or "KEY#2", "KEY#3" for extra terminals. */
  termSessions: string[]
  activeSession: string | null
  selectSession: (session: string) => void
  termActivity: Record<string, TermLightState>
  chirpMuted: boolean
  setChirpMuted: (m: boolean) => void
  /** Opens (and by default focuses) the ticket's terminal. */
  /** Opens (and by default focuses) a terminal for the ticket; fresh = a new one. Returns the session. */
  openTerminal: (ticket: string, opts?: { focus?: boolean; fresh?: boolean }) => string
  closeTerminal: (session: string) => void
  /** Freestyle terminals: not linked to any ticket. */
  freeSessions: FreeSession[]
  activeFree: string | null
  selectFree: (id: string) => void
  newFree: () => string
  closeFree: (id: string) => void
  renameFree: (id: string, name: string) => void
  createDraft: Partial<NewIssue> | null
  openCreate: (defaults?: Partial<NewIssue>) => void
  closeCreate: () => void

  transition: (key: string, to: string) => Promise<void>
  startWork: (key: string) => Promise<void>
  nextEvent: (key: string) => Promise<void>
  resetFlow: (key: string) => Promise<void>
  comment: (key: string, body: string) => Promise<void>
  createIssue: (input: NewIssue) => Promise<Issue | null>
  link: (from: string, type: string, to: string) => Promise<void>
  unlink: (from: string, type: string, to: string) => Promise<void>
  toggleRule: (id: string) => Promise<void>
  setRuleStatus: (id: string, status: string) => Promise<void>
  copy: (text: string, label?: string) => void
  dismissToast: (id: string) => void
  /** Makes an issue known to the app (e.g. opened from another team's board). */
  adoptIssue: (issue: Issue) => void
  /** Re-reads one issue (with comments) from the tracker. */
  refreshIssue: (key: string) => Promise<void>
  connection: Connection | null
  github: GitHubStatus | null
  live: boolean
  projects: string[]
  sprints: SprintInfo[]
  versions: Record<string, VersionInfo[]>
  createDefaults: CreateDefaults
  timeTracking: boolean
  customColumns: CustomColumn[]
  updateFields: (key: string, p: FieldPatch) => Promise<boolean>
}

const Ctx = createContext<Store | null>(null)

export const useStore = () => {
  const s = useContext(Ctx)
  if (!s) throw new Error('useStore outside provider')
  return s
}

let toastSeq = 0

// Deep-link support: ?view=board&ticket=PLAT-142&jira=PLAT-100
const params = new URLSearchParams(window.location.search)

export function StoreProvider({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState(false)
  const [me, setMe] = useState<User | null>(null)
  const [issues, setIssues] = useState<Record<string, Issue>>({})
  const [flows, setFlows] = useState<Record<string, Flow>>({})
  const [statuses, setStatuses] = useState<string[]>([])
  const [rules, setRules] = useState<Rule[]>([])
  const [activity, setActivity] = useState<FlowEvent[]>([])
  const [toasts, setToasts] = useState<Toast[]>([])
  const [users, setUsers] = useState<User[]>([])
  const [sprint, setSprint] = useState('')
  const [childTypes, setChildTypes] = useState({} as Record<IssueType, IssueType[]>)
  const [linkTypes, setLinkTypes] = useState<Record<string, string>>({})
  const [createDraft, setCreateDraft] = useState<Partial<NewIssue> | null>(null)
  const [connection, setConnection] = useState<Connection | null>(null)
  const [github, setGitHub] = useState<GitHubStatus | null>(null)
  const [live, setLive] = useState(false)
  const [projects, setProjects] = useState<string[]>([])
  const [sprints, setSprints] = useState<SprintInfo[]>([])
  const [versions, setVersions] = useState<Record<string, VersionInfo[]>>({})
  const [createDefaults, setCreateDefaults] = useState<CreateDefaults>({ sprint: 'active', fixVersion: 'none', affectsVersion: 'none' })
  const [timeTracking, setTimeTracking] = useState(false)
  const [customColumns, setCustomColumns] = useState<CustomColumn[]>([])
  const [view, setView] = useState<View>((params.get('view') as View) || 'mine')
  const [focusKey, setFocusKey] = useState(params.get('ticket') || 'PLAT-142')
  const [jiraKey, setJiraKey] = useState<string | null>(params.get('jira'))
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [termOpen, setTermOpen] = useState(true)
  const [termSessions, setTermSessions] = useState<string[]>([])
  const [activeSession, setActiveSession] = useState<string | null>(null)
  const [termActivity, setTermActivity] = useState<Record<string, TermLightState>>({})
  const [chirpMuted, setChirpMutedState] = useState(loadMuted)
  const sessionsRef = useRef<string[]>([])
  sessionsRef.current = termSessions
  const [freeSessions, setFreeSessionsState] = useState<FreeSession[]>(loadFree)
  const [activeFree, setActiveFree] = useState<string | null>(() => loadFree().at(-1)?.id ?? null)
  const freeRef = useRef(freeSessions)
  freeRef.current = freeSessions
  const setFreeSessions = useCallback((f: (s: FreeSession[]) => FreeSession[]) => {
    setFreeSessionsState(s => { const n = f(s); saveFree(n); freeRef.current = n; return n })
  }, [])
  const viewRef = useRef({ activeSession, termOpen, view, activeFree })
  viewRef.current = { activeSession, termOpen, view, activeFree }

  useEffect(() => {
    api.snapshot().then(s => {
      setMe(s.me)
      // ?? guards: one empty field from a live site must not blank the app.
      setIssues(Object.fromEntries((s.issues ?? []).map(i => [i.key, i])))
      setFlows(Object.fromEntries((s.flows ?? []).map(f => [f.issueKey, f])))
      setStatuses(s.statuses ?? [])
      setRules(s.rules ?? [])
      setActivity(s.activity ?? [])
      setUsers(s.users ?? [])
      setSprint(s.sprint ?? '')
      setChildTypes(s.childTypes ?? {})
      setLinkTypes(s.linkTypes ?? {})
      setStatusStates(s.statusStates ?? {})
      setProjects(s.projects ?? [])
      setSprints(s.sprints ?? [])
      setVersions(s.versions ?? {})
      if (s.createDefaults) setCreateDefaults(s.createDefaults)
      setTimeTracking(!!s.timeTracking)
      setCustomColumns(s.customColumns ?? [])
      setLive(!!s.live)
      // Focus a ticket that exists: the deep link, else my first open work item.
      setFocusKey(k => {
        if (s.issues.some(i => i.key === k)) return k
        const mine = s.issues.filter(i => isWork(i) && i.type !== 'subtask' && i.assignee?.id === s.me.id && i.statusCategory !== 'done')
        const pick = mine.find(i => i.statusCategory === 'inprogress') ?? mine[0] ?? s.issues.find(i => isWork(i))
        return pick?.key ?? k
      })
      setReady(true)
      api.connection().then(setConnection).catch(() => {})
    }).catch(err => reportError('error', err, 'snapshot'))
  }, [])

  // Live Jira: sync results and connection state pushed by the backend.
  useEffect(() => EventsOn('tracker:changed', (c: { issues: Issue[] | null; removed: string[] | null; activity?: FlowEvent[] }) => {
    setIssues(m => {
      const next = { ...m }
      for (const i of c.issues ?? []) next[i.key] = { ...i, comments: i.comments?.length ? i.comments : m[i.key]?.comments ?? [] }
      for (const k of c.removed ?? []) delete next[k]
      return next
    })
    if (c.activity) setActivity(c.activity)
  }), [])
  useEffect(() => EventsOn('tracker:connection', (c: Connection) => setConnection(c)), [])
  useEffect(() => { api.connection().then(setConnection).catch(() => {}) }, [])
  useEffect(() => EventsOn('github:status', (g: GitHubStatus) => setGitHub(g)), [])
  useEffect(() => { api.githubStatus().then(setGitHub).catch(() => {}) }, [])

  const pushToast = useCallback((t: Omit<Toast, 'id'>) => {
    const id = `t${++toastSeq}`
    setToasts(ts => [...ts.slice(-3), { ...t, id }])
    setTimeout(() => setToasts(ts => ts.filter(x => x.id !== id)), 4800)
  }, [])

  const putIssue = (i: Issue) => setIssues(m => ({ ...m, [i.key]: i }))
  const putIssues = (list: Issue[]) => setIssues(m => ({ ...m, ...Object.fromEntries(list.map(i => [i.key, i])) }))

  const applyStep = useCallback((r: StepResult) => {
    putIssue(r.issue)
    setFlows(m => ({ ...m, [r.flow.issueKey]: r.flow }))
    if (r.events.length) setActivity(a => [...[...r.events].reverse(), ...a])
    for (const e of r.events) {
      if (e.kind === 'term') continue
      if (e.kind === 'automation') pushToast({ tone: 'automation', title: `${e.issueKey} · ${e.title}`, detail: e.detail, issueKey: e.issueKey })
      else pushToast({ tone: 'dev', title: e.title, detail: e.detail, issueKey: e.issueKey })
    }
  }, [pushToast])

  // Tell the user once if the previous run crashed; the report is in the log folder.
  useEffect(() => {
    api.logInfo().then(info => {
      if (info.previousCrash) pushToast({ tone: 'error', title: 'Maestro crashed last time', detail: 'A crash report was saved. Click to open the log folder.', onClick: () => void api.openLogFolder() })
    }).catch(() => {})
  }, [pushToast])

  useEffect(() => EventsOn('flow:step', (r: StepResult) => applyStep(r)), [applyStep])
  useEffect(() => EventsOn('term:exit', (session: string) => {
    setTermSessions(ss => ss.filter(s => s !== session))
    setTermActivity(a => { const n = { ...a }; delete n[session]; return n })
    // A freestyle shell that exits (e.g. "exit") closes its tab.
    if (session.startsWith(FREE)) setFreeSessions(fs => fs.filter(f => f.id !== session))
  }), [setFreeSessions])

  // Terminal activity: light on the ticket, and a chirp + notice when a long
  // command finishes or a command starts waiting — unless you're looking at it.
  useEffect(() => EventsOn('term:activity', (ev: TermActivity) => {
    const v = viewRef.current
    const free = ev.session.startsWith(FREE)
    const watching = document.hasFocus() && (free ? v.view === 'freestyle' && v.activeFree === ev.session : v.termOpen && v.activeSession === ev.session)
    setTermActivity(a => ({
      ...a,
      [ev.session]: ev.finished
        ? (ev.notify && !watching ? { state: 'finished', command: ev.command, seconds: ev.seconds } : { state: 'idle' })
        : { state: ev.state, command: ev.command, since: ev.since },
    }))
    if (!ev.notify || watching) return
    const label = free ? freeRef.current.find(f => f.id === ev.session)?.name ?? 'Freestyle' : ev.session.includes('#') ? ev.session.replace('#', ' #') : ev.session
    const what = ev.command || 'command'
    if (ev.sound && !loadMuted()) chirp(ev.finished ? 'done' : 'waiting')
    pushToast({
      tone: ev.finished ? 'dev' : 'automation',
      title: ev.finished ? `${label}: ${what} finished` : `${label}: ${what} is waiting for input`,
      detail: ev.finished ? `after ${formatSeconds(ev.seconds ?? 0)} · click to open the terminal` : 'Click to open the terminal',
      onClick: free
        ? () => { setView('freestyle'); setActiveFree(ev.session); focusTerminalSoon(ev.session) }
        : () => { setActiveSession(ev.session); setFocusKey(ev.ticket); setTermOpen(true); focusTerminalSoon(ev.session) },
    })
  }), [pushToast])

  // Seeing a freestyle terminal clears its "finished" light.
  useEffect(() => {
    if (view !== 'freestyle' || !activeFree) return
    setTermActivity(a => a[activeFree]?.state === 'finished' ? { ...a, [activeFree]: { state: 'idle' } } : a)
  }, [view, activeFree])

  // Seeing a session clears its "finished" light.
  useEffect(() => {
    if (!termOpen || !activeSession) return
    setTermActivity(a => a[activeSession]?.state === 'finished' ? { ...a, [activeSession]: { state: 'idle' } } : a)
  }, [termOpen, activeSession])

  // Window title shows how many terminals need attention.
  const attention = Object.values(termActivity).filter(t => t.state === 'waiting' || t.state === 'finished').length
  useEffect(() => {
    try { WindowSetTitle(attention ? `(${attention}) Maestro` : 'Maestro') } catch { /* bridge not ready */ }
  }, [attention])

  const guard = useCallback(<A extends unknown[]>(fn: (...a: A) => Promise<void>) => async (...a: A) => {
    try { await fn(...a) } catch (err) {
      reportError('warn', err, 'action')
      pushToast({ tone: 'error', title: 'Something went wrong', detail: String(err), onClick: () => void api.openLogFolder() })
    }
  }, [pushToast])

  const value = useMemo<Store>(() => ({
    ready, me, issues, flows, statuses, rules, activity, toasts, users, sprint, childTypes, linkTypes,
    view, setView, focusKey, setFocusKey,
    jiraKey, openJira: setJiraKey,
    paletteOpen, setPaletteOpen,
    termOpen, setTermOpen, termSessions, activeSession, termActivity, chirpMuted,
    setChirpMuted: m => { saveMuted(m); setChirpMutedState(m) },
    selectSession: session => { setActiveSession(session); setFocusKey(sessionTicket(session)); setTermOpen(true); focusTerminalSoon(session) },
    openTerminal: (ticket, opts) => {
      const all = sessionsRef.current
      const mine = all.filter(s => sessionTicket(s) === ticket)
      let session: string
      if (opts?.fresh || mine.length === 0) {
        session = mine.length === 0 && !all.includes(ticket) ? ticket : `${ticket}#${nextIndex(mine)}`
        sessionsRef.current = [...all, session]
        setTermSessions(ss => ss.includes(session) ? ss : [...ss, session])
      } else {
        const active = viewRef.current.activeSession
        session = active && sessionTicket(active) === ticket ? active : mine[mine.length - 1]
      }
      setActiveSession(session); setFocusKey(ticket); setTermOpen(true)
      if (opts?.focus !== false) focusTerminalSoon(session)
      return session
    },
    adoptIssue: i => setIssues(m => (m[i.key] ? m : { ...m, [i.key]: i })),
    freeSessions, activeFree,
    selectFree: id => { setActiveFree(id); focusTerminalSoon(id) },
    newFree: () => {
      const n = nextFree(freeRef.current)
      const id = `${FREE}${n}`
      const next = [...freeRef.current, { id, name: `Freestyle ${n}` }]
      freeRef.current = next // two quick calls must not reuse the same id
      setFreeSessions(() => next)
      setActiveFree(id)
      focusTerminalSoon(id)
      return id
    },
    closeFree: id => {
      void api.termClose(id).catch(() => {})
      const rest = freeRef.current.filter(f => f.id !== id)
      setFreeSessions(() => rest)
      setTermActivity(a => { const n = { ...a }; delete n[id]; return n })
      if (viewRef.current.activeFree === id) setActiveFree(rest.at(-1)?.id ?? null)
    },
    renameFree: (id, name) => setFreeSessions(fs => fs.map(f => f.id === id ? { ...f, name: name.trim() || f.name } : f)),
    createDraft, openCreate: d => setCreateDraft(d ?? {}), closeCreate: () => setCreateDraft(null),
    connection, github, live, projects, sprints, versions, createDefaults, timeTracking, customColumns,
    updateFields: async (key, p) => {
      try {
        putIssue(await api.updateFields(key, p))
        return true
      } catch (err) {
        reportError('warn', err, 'update fields')
        pushToast({ tone: 'error', title: `Could not update ${key}`, detail: String(err) })
        return false
      }
    },
    refreshIssue: async key => {
      try { putIssue(await api.issue(key)) } catch (err) { reportError('warn', err, 'refresh issue') }
    },
    closeTerminal: session => {
      api.termClose(session)
      const rest = sessionsRef.current.filter(s => s !== session)
      sessionsRef.current = rest
      setTermSessions(rest)
      setTermActivity(a => { const n = { ...a }; delete n[session]; return n })
      if (viewRef.current.activeSession === session) {
        const sameTicket = rest.filter(s => sessionTicket(s) === sessionTicket(session))
        setActiveSession(sameTicket[sameTicket.length - 1] ?? rest[rest.length - 1] ?? null)
      }
    },

    transition: guard(async (key, to) => {
      const before = issues[key]?.status
      if (before === to) return
      putIssue(await api.transition(key, to))
      setActivity(await api.activity())
      pushToast({ tone: 'jira', title: `${key} → ${to}`, detail: `Moved from ${before}`, issueKey: key })
    }),
    startWork: guard(async key => applyStep(await api.startWork(key))),
    nextEvent: guard(async key => applyStep(await api.nextEvent(key))),
    resetFlow: guard(async key => applyStep(await api.resetFlow(key))),
    comment: guard(async (key, body) => putIssue(await api.comment(key, body))),
    createIssue: async input => {
      try {
        const is = await api.createIssue(input)
        // Refresh so the parent's childKeys and the new flow come back from the backend.
        const s = await api.snapshot()
        setIssues(Object.fromEntries(s.issues.map(i => [i.key, i])))
        setFlows(Object.fromEntries(s.flows.map(f => [f.issueKey, f])))
        setActivity(s.activity)
        pushToast({ tone: 'jira', title: `Created ${is.key}`, detail: is.summary, issueKey: is.key })
        return is
      } catch (err) {
        reportError('warn', err, 'create issue')
        pushToast({ tone: 'error', title: 'Could not create issue', detail: String(err) })
        return null
      }
    },
    link: guard(async (from, type, to) => { putIssues(await api.link(from, type, to)); setActivity(await api.activity()) }),
    unlink: guard(async (from, type, to) => putIssues(await api.unlink(from, type, to))),
    toggleRule: guard(async id => setRules(await api.toggleRule(id))),
    setRuleStatus: guard(async (id, status) => setRules(await api.setRuleStatus(id, status))),
    copy: (text, label) => { api.copy(text); pushToast({ tone: 'dev', title: 'Copied to clipboard', detail: label ?? text }) },
    dismissToast: id => setToasts(ts => ts.filter(t => t.id !== id)),
  }), [ready, me, issues, flows, statuses, rules, activity, toasts, users, sprint, childTypes, linkTypes, createDraft, connection, github, live, projects, sprints, versions, createDefaults, timeTracking, customColumns, view, focusKey, jiraKey, paletteOpen, termOpen, termSessions, activeSession, termActivity, chirpMuted, freeSessions, activeFree, setFreeSessions, guard, applyStep, pushToast])

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export const sessionTicket = (session: string) => session.split('#')[0]

const FREE = 'free:'
const FREE_KEY = 'maestro.freestyle'

/** Freestyle tabs and their names are remembered per machine; shells restart with the app. */
function loadFree(): FreeSession[] {
  try {
    const v = JSON.parse(localStorage.getItem(FREE_KEY) ?? '[]')
    return Array.isArray(v) ? v.filter(f => typeof f?.id === 'string' && f.id.startsWith(FREE) && typeof f.name === 'string') : []
  } catch { return [] }
}

function saveFree(fs: FreeSession[]) {
  try { localStorage.setItem(FREE_KEY, JSON.stringify(fs)) } catch { /* per-viewer convenience only */ }
}

function nextFree(fs: FreeSession[]) {
  let n = 1
  while (fs.some(f => f.id === `${FREE}${n}`)) n++
  return n
}

function nextIndex(sessions: string[]) {
  let n = 1
  for (const s of sessions) n = Math.max(n, Number(s.split('#')[1] ?? 1))
  return n + 1
}

/** The pane may mount on this render, so ask for focus on the next tick. */
function focusTerminalSoon(session: string) {
  setTimeout(() => window.dispatchEvent(new CustomEvent('maestro:focus-terminal', { detail: session })), 60)
}

function formatSeconds(s: number) {
  if (s < 60) return `${s}s`
  if (s < 3600) return `${Math.floor(s / 60)}m ${s % 60}s`
  return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`
}

/** The most important terminal state of a ticket, across its sessions. */
export function ticketLight(ticket: string, sessions: string[], activity: Record<string, TermLightState>): TermLightState | null {
  const rank = { waiting: 4, running: 3, finished: 2, idle: 1 } as const
  let best: TermLightState | null = null
  for (const s of sessions) {
    if (sessionTicket(s) !== ticket) continue
    const a = activity[s] ?? { state: 'idle' as const }
    if (!best || rank[a.state] > rank[best.state]) best = a
  }
  return best
}
