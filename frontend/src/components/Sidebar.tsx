import { useMemo, useState } from 'react'
import { ChevronRight, Columns3, Crosshair, Flag, Inbox, Keyboard, Layers, Lock, Plus, Search, Zap } from 'lucide-react'
import { combosOf, compactCombo, formatCombo, useKeys } from '../keys'
import { useStore } from '../store'
import type { Issue, View } from '../types'
import { activeStage, cx, isContainer, isWork, statusTone, timeAgo } from '../lib/util'
import { Avatar, Kbd, TypeIcon, modKey } from './bits'
import { TicketTermLight } from './TerminalDock'

const NAV: { id: View; label: string; short: string; icon: typeof Crosshair; action: string }[] = [
  { id: 'mine', label: 'My work', short: 'Mine', icon: Inbox, action: 'nav.mine' },
  { id: 'focus', label: 'Focus', short: 'Focus', icon: Crosshair, action: 'nav.focus' },
  { id: 'board', label: 'Board', short: 'Board', icon: Columns3, action: 'nav.board' },
  { id: 'roadmap', label: 'Roadmap', short: 'Roadmap', icon: Layers, action: 'nav.roadmap' },
  { id: 'automations', label: 'Automations', short: 'Auto', icon: Zap, action: 'nav.automations' },
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

export function Sidebar() {
  const { issues, flows, me, focusKey, setFocusKey, setView, setPaletteOpen, openCreate, openJira, childTypes } = useStore()
  const [mine, setMine] = useState(true)
  const conn = useConnection()
  const { live } = useStore()
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

  const visible = (i: Issue): boolean => {
    const kids = i.childKeys.map(k => issues[k]).filter(Boolean)
    if (kids.some(visible)) return true
    if (isContainer(i)) return !mine && !query && !kids.length // show empty initiatives/epics in "Everything"
    const okMine = !mine || i.assignee?.id === me?.id
    const okQ = !query || i.key.toLowerCase().includes(query) || i.summary.toLowerCase().includes(query)
    return okMine && okQ
  }

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
      <div className="sidebar-head drag">
        <div className="workspace">
          <div className="ws-name">Acme Engineering</div>
          <div className={cx('ws-sub', `conn-${conn.state}`)} title={conn.hint}>
            <span className="conn-dot" /> {conn.label}
            {(conn.state === 'none' || (conn.state === 'error' && !live)) && <span className="mock-tag">demo data</span>}
          </div>
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
      </div>

      <div className="tree scroll">
        {roots.map(r => renderNode(r, 0))}
      </div>
    </aside>
  )
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
  return { state: 'none', label: 'Jira not connected', hint: 'Showing sample tickets. Connect with: go run ./cmd/maestro-jira login' }
}
