import { useCallback, useMemo, useState } from 'react'
import { ChevronRight, Columns3, Crosshair, Flag, GitPullRequest, RefreshCw, SquareDashedBottomCode, Inbox, Keyboard, Layers, Lock, Plus, Search, Settings as SettingsIcon, Zap } from 'lucide-react'
import { combosOf, compactCombo, formatCombo, useAction, useKeys } from '../keys'
import { api } from '../api'
import { useStore } from '../store'
import type { Issue, View } from '../types'
import { activeStage, cx, isContainer, isWork, statusTone, timeAgo } from '../lib/util'
import { visibleKeys } from '../lib/tree'
import { Avatar, Kbd, TypeIcon, modKey } from './bits'
import { TicketTermLight } from './TerminalDock'
import { workspaceName } from '../lib/workspace'

const NAV: { id: View; label: string; short: string; icon: typeof Crosshair; action: string }[] = [
  { id: 'mine', label: 'My work', short: 'Mine', icon: Inbox, action: 'nav.mine' },
  { id: 'focus', label: 'Focus', short: 'Focus', icon: Crosshair, action: 'nav.focus' },
  { id: 'board', label: 'Board', short: 'Board', icon: Columns3, action: 'nav.board' },
  { id: 'roadmap', label: 'Roadmap', short: 'Roadmap', icon: Layers, action: 'nav.roadmap' },
  { id: 'automations', label: 'Automations', short: 'Auto', icon: Zap, action: 'nav.automations' },
  { id: 'freestyle', label: 'Freestyle terminals (no ticket)', short: 'Free', icon: SquareDashedBottomCode, action: 'nav.freestyle' },
]

export function Rail() {
  const { view, setView, me } = useStore()
  const { bindings } = useKeys()
  // Badge: the first key; tooltip: every way to reach it.
  const keyOf = (action: string) => compactCombo(bindings[action])
  const keysOf = (action: string) => combosOf(bindings[action]).map(c => formatCombo(c).filter(p => p !== 'then').join(' ')).join(' or ')
  return (
    <nav className="rail">
      <div className="logo" title="Maestro">
        <svg viewBox="0 0 32 32" width="22" height="22"><path d="M5 25V8l7 9 4-6 4 6 7-9v17" fill="none" stroke="white" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" /></svg>
      </div>
      <div className="rail-items">
        {NAV.map(n => {
          const k = keyOf(n.action)
          return (
            <button key={n.id} className={cx('rail-btn', view === n.id && 'active')} onClick={() => setView(n.id)}
              data-tip={k ? `${n.label}  ·  ${keysOf(n.action)}` : n.label} aria-label={n.label}>
              <n.icon size={19} />
              <span className="rail-label">{n.short}</span>
              {k && <span className="rail-key">{k}</span>}
            </button>
          )
        })}
      </div>
      <div className="rail-bottom">
        <button className={cx('rail-btn', view === 'settings' && 'active')} data-tip={`Settings  ·  ${keysOf('nav.settings')}`} aria-label="Settings"
          onClick={() => setView('settings')}>
          <SettingsIcon size={19} />
          <span className="rail-label">Settings</span>
        </button>
        <button className="rail-btn" data-tip={`Keyboard shortcuts  ·  ${keyOf('help')}`} aria-label="Keyboard shortcuts"
          onClick={() => window.dispatchEvent(new Event('maestro:open-help'))}>
          <Keyboard size={19} />
          <span className="rail-label">Keys</span>
        </button>
        <Avatar user={me} size={30} />
      </div>
    </nav>
  )
}

// ── Resizable width ─────────────────────────────────────────────
// The width lives in a CSS variable on <html>: dragging changes only that
// variable (no React render per mouse move), and it is saved on release.
const WIDTH_KEY = 'maestro.sidebarWidth'
const DEFAULT_WIDTH = 296
const clampWidth = (w: number) => Math.round(Math.min(Math.max(200, w), Math.max(320, Math.min(720, window.innerWidth * 0.5))))
const applyWidth = (w: number) => document.documentElement.style.setProperty('--sidebar-w', `${w}px`)

// Restore the saved width before the first paint.
try {
  const saved = Number(localStorage.getItem(WIDTH_KEY))
  if (saved > 0) applyWidth(clampWidth(saved))
} catch { /* per-viewer convenience only */ }

function SidebarGrip() {
  const start = (e: React.MouseEvent) => {
    e.preventDefault()
    const side = (e.currentTarget.parentElement as HTMLElement)
    const x0 = e.clientX
    const w0 = side.getBoundingClientRect().width
    let w = w0
    document.body.classList.add('resizing-x')
    const move = (ev: MouseEvent) => { w = clampWidth(w0 + ev.clientX - x0); applyWidth(w) }
    const up = () => {
      window.removeEventListener('mousemove', move)
      window.removeEventListener('mouseup', up)
      document.body.classList.remove('resizing-x')
      try { localStorage.setItem(WIDTH_KEY, String(w)) } catch { /* ignore */ }
    }
    window.addEventListener('mousemove', move)
    window.addEventListener('mouseup', up)
  }
  const reset = () => {
    applyWidth(DEFAULT_WIDTH)
    try { localStorage.removeItem(WIDTH_KEY) } catch { /* ignore */ }
  }
  return <div className="sidebar-grip" onMouseDown={start} onDoubleClick={reset} title="Drag to resize · double-click to reset" />
}

const SPRINT_SEL_KEY = 'maestro.treeSprint'
function loadSprintSel(): string {
  try { return localStorage.getItem(SPRINT_SEL_KEY) || 'all' } catch { return 'all' }
}
function saveSprintSel(v: string) {
  try { localStorage.setItem(SPRINT_SEL_KEY, v) } catch { /* per-viewer convenience only */ }
}

export function Sidebar() {
  const { issues, flows, me, focusKey, setFocusKey, setView, setPaletteOpen, openCreate, openJira, childTypes, sprints } = useStore()
  const [mine, setMine] = useState(true)
  const conn = useConnection()
  const gh = useGitHub()
  const { live, connection } = useStore()
  const { pushToast } = useStore()
  const [refreshing, setRefreshing] = useState(false)
  const refresh = useCallback(async () => {
    if (refreshing) return
    if (!live) { pushToast({ tone: 'dev', title: 'Demo data', detail: 'Connect Jira to refresh real tickets.' }); return }
    setRefreshing(true)
    try {
      await api.refreshJira()
      pushToast({ tone: 'dev', title: 'Refreshed from Jira', detail: 'Tickets are up to date.' })
    } catch (err) {
      pushToast({ tone: 'error', title: 'Refresh failed', detail: String(err) })
    } finally { setRefreshing(false) }
  }, [refreshing, live, pushToast])
  useAction('jira.refresh', () => void refresh())
  const [q, setQ] = useState('')
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})

  // Roots are issues without a known parent: initiatives first, then epics, then
  // loose work items. Sites without an initiative level start at epics.
  const roots = useMemo(() => {
    const rank: Record<string, number> = { initiative: 0, epic: 1 }
    return Object.values(issues)
      .filter(i => !issues[i.parentKey])
      .sort((a, b) => (rank[a.type] ?? 2) - (rank[b.type] ?? 2) || a.key.localeCompare(b.key, undefined, { numeric: true }))
  }, [issues])
  const query = q.trim().toLowerCase()

  // Sprint filter: all | current (any active sprint) | backlog | s:<name>.
  const [sprintSel, setSprintSelState] = useState(loadSprintSel)
  const setSprintSel = (v: string) => { setSprintSelState(v); saveSprintSel(v) }
  const active = useMemo(() => new Set(sprints.filter(s => s.state === 'active').map(s => s.name)), [sprints])
  const sprintNames = useMemo(() => {
    // Sprints known from the boards, plus any sprint named on a ticket.
    const names = new Map(sprints.map(s => [s.name, s.state as string]))
    for (const i of Object.values(issues)) if (i.sprint && !names.has(i.sprint)) names.set(i.sprint, '')
    return [...names].sort((a, b) => (a[1] === 'active' ? 0 : 1) - (b[1] === 'active' ? 0 : 1) || a[0].localeCompare(b[0], undefined, { numeric: true }))
  }, [sprints, issues])
  const sprintOk = useCallback((i: Issue) => {
    if (sprintSel === 'all') return true
    // A sub-task is in its parent's sprint.
    const sp = i.sprint || (i.type === 'subtask' ? issues[i.parentKey]?.sprint ?? '' : '')
    if (sprintSel === 'current') return active.has(sp)
    if (sprintSel === 'backlog') return !sp
    return sp === sprintSel.slice(2)
  }, [sprintSel, active, issues])

  // Which tickets show, computed once per change (not per row per render).
  const shown = useMemo(() => visibleKeys(issues, roots, { mine, me: me?.id, query, sprintOk, filtered: mine || !!query || sprintSel !== 'all' }),
    [issues, roots, mine, query, me?.id, sprintSel, sprintOk])
  const visible = (i: Issue) => shown.has(i.key)

  const select = (k: string) => { setFocusKey(k); setView('focus') }

  const myActive = Object.values(issues).filter(i => i.assignee?.id === me?.id && i.statusCategory === 'inprogress' && isWork(i) && i.type !== 'subtask').length

  const renderNode = (i: Issue, depth: number) => {
    if (!visible(i)) return null
    const kids = i.childKeys.map(k => issues[k]).filter(Boolean)
    const group = isContainer(i)
    const hasKids = kids.length > 0
    // Sub-task lists start folded; initiatives and epics start open.
    const open = expanded[i.key] ?? group
    const f = flows[i.key]
    const live = f?.started && f.pr?.state !== 'merged'
    const canAdd = (childTypes[i.type] ?? []).length > 0
    const toggle = () => setExpanded(c => ({ ...c, [i.key]: !open }))
    return (
      <div key={i.key} className="tree-node">
        <div data-issue={i.key} className={cx('tree-row', group && 'group', `depth-${depth}`, focusKey === i.key && 'selected')}
          style={{ paddingLeft: 8 + depth * 14 }}
          onClick={() => group ? toggle() : select(i.key)}
          onDoubleClick={() => group && openJira(i.key)}>
          {hasKids
            ? <ChevronRight size={13} className={cx('chev', open && 'open')} onClick={e => { e.stopPropagation(); toggle() }} />
            : group ? <span className="chev-space" /> : <span className={cx('dot', `tone-${statusTone(i.status)}`, live && 'live')} />}
          <TypeIcon type={i.type} size={12} />
          <span className="tree-key">{i.key}</span>
          {i.security && <Lock size={11} className="tree-lock" aria-label={`Secured: ${i.security}`} />}
          {i.flagged && <Flag size={11} className="tree-flag" aria-label="Flagged" />}
          <span className="tree-summary">{i.summary}</span>
          <TicketTermLight ticket={i.key} />
          {live && activeStage(f) && <span className="tree-stage">{activeStage(f)!.label}</span>}
          {!group && hasKids && <span className="tree-count">{kids.filter(k => k.statusCategory === 'done').length}/{kids.length}</span>}
          {canAdd && (
            <button className="tree-add" title={`Add ${childTypes[i.type].join(' / ')} under ${i.key}`}
              onClick={e => { e.stopPropagation(); setExpanded(c => ({ ...c, [i.key]: true })); openCreate({ parentKey: i.key }) }}>
              <Plus size={12} />
            </button>
          )}
        </div>
        {hasKids && open && <div className="tree-children">{kids.map(k => renderNode(k, depth + 1))}</div>}
      </div>
    )
  }

  return (
    <aside className="sidebar">
      <SidebarGrip />
      <div className="sidebar-head drag">
        <div className="workspace">
          <div className="ws-name" title={connection?.site || undefined}>{workspaceName(connection)}</div>
          <div className={cx('ws-sub', `conn-${conn.state}`)} title={conn.hint}>
            <span className="conn-dot" /> {conn.label}
            {(conn.state === 'none' || (conn.state === 'error' && !live)) && <span className="mock-tag">demo data</span>}
            {live && (
              <button className={cx('refresh-btn', 'no-drag', refreshing && 'busy')} title="Refresh from Jira now (Shift+R)" onClick={() => void refresh()}>
                <RefreshCw size={11} />
              </button>
            )}
          </div>
          {gh && (
            <div className={cx('ws-sub', 'gh-sub', `conn-${gh.state}`)} title={gh.hint}>
              <GitPullRequest size={11} /> {gh.label}
            </div>
          )}
        </div>
      </div>

      <div className="side-actions">
        <button className="search-trigger" onClick={() => setPaletteOpen(true)}>
          <Search size={14} /> <span>Jump to ticket…</span> <Kbd>{modKey}K</Kbd>
        </button>
        <button className="create-btn" title="Create issue (C)" onClick={() => openCreate()}><Plus size={16} /></button>
      </div>

      <div className="seg">
        <button className={cx(mine && 'on')} onClick={() => setMine(true)}>My work <b>{myActive}</b></button>
        <button className={cx(!mine && 'on')} onClick={() => setMine(false)}>Everything</button>
      </div>

      <div className="filter">
        <input data-search value={q} onChange={e => setQ(e.target.value)} placeholder="Filter…  /" />
        <select className={cx('sprint-filter', sprintSel !== 'all' && 'on')} value={sprintSel} onChange={e => setSprintSel(e.target.value)} title="Show tickets of one sprint">
          <option value="all">All sprints</option>
          <option value="current" disabled={!active.size}>{active.size === 1 ? `Current · ${[...active][0]}` : active.size ? 'Current sprints' : 'Current (none active)'}</option>
          {sprintNames.length > 0 && (
            <optgroup label="Sprint">
              {sprintNames.map(([n, state]) => <option key={n} value={`s:${n}`}>{n}{state === 'future' ? ' (upcoming)' : ''}</option>)}
            </optgroup>
          )}
          <option value="backlog">Backlog (no sprint)</option>
        </select>
      </div>

      <div className="tree scroll">
        {roots.map(r => renderNode(r, 0))}
      </div>
    </aside>
  )
}

/** Pull request watching. Hidden in demo mode, where PRs are simulated. */
function useGitHub(): { state: ConnState; label: string; hint: string } | null {
  const { github: g, live } = useStore()
  if (!g || !live) return null
  const repos = (g.repos ?? []).join(', ')
  const host = g.host || 'github.com'
  switch (g.state) {
    case 'ok': return { state: 'live', label: `GitHub · @${g.user}`, hint: `Following ${repos} (${g.prs} pull request${g.prs === 1 ? '' : 's'} linked${g.lastSync ? ` · checked ${timeAgo(g.lastSync)}` : ''})` }
    case 'connecting': return { state: 'syncing', label: `${host} · connecting…`, hint: `Reading pull requests of ${repos}` }
    case 'off': return { state: 'none', label: 'GitHub off', hint: g.error }
  }
  return { state: 'error', label: g.user ? `GitHub · @${g.user} · problem` : 'GitHub not connected', hint: g.error }
}

type ConnState = 'live' | 'syncing' | 'offline' | 'configured' | 'none' | 'error'

/** Where the data comes from, as the backend reports it. Never claims a live connection it doesn't have. */
function useConnection(): { state: ConnState; label: string; hint: string } {
  const { connection: c } = useStore()
  if (!c || c.mode === 'connecting') return { state: 'syncing', label: c?.site ? `Connecting to ${c.site}…` : 'Starting…', hint: 'Loading your Jira issues' }
  if (c.mode === 'live') {
    const when = c.lastSync ? ` · synced ${timeAgo(c.lastSync)}` : ''
    switch (c.state) {
      case 'offline': return { state: 'offline', label: `${c.site} · offline`, hint: c.error ?? 'Cannot reach Jira; retrying. Showing the last synced data.' }
      case 'error': return { state: 'error', label: `${c.site} · sync stopped`, hint: c.error ?? 'Sync failed' }
      case 'syncing': return { state: 'syncing', label: `${c.site} · syncing…`, hint: 'Loading from Jira' }
    }
    return { state: 'live', label: `Jira · ${c.site}`, hint: `Live data from Jira (${c.issues} issues${when})` }
  }
  if (c.configured) return { state: 'error', label: `${c.site} · not connected`, hint: (c.error ?? 'Could not connect') + ' — showing sample tickets.' }
  return { state: 'none', label: 'Jira not connected', hint: 'Showing sample tickets. Connect with: maestro-jira login' }
}
