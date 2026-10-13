import { useEffect, useMemo, useRef, useState } from 'react'
import { ChevronRight, Crosshair, Maximize2, Pin, PinOff, Plus, Search, SquareTerminal, Users, X } from 'lucide-react'
import { useStore } from '../store'
import type { Issue, User } from '../types'
import { cx, isWork, stateOf } from '../lib/util'
import { Avatar, PriorityIcon, StatusMenu, TypeIcon } from './bits'
import { useAction } from '../keys'
import { TicketTermLight } from './TerminalDock'
import { IssueBadges, severityTone } from './Badges'

const ALL = '*'
// Flagged issues count as blocked without a status change (custom field role blockedFlag).
const st = (i: Issue) => (i.flagged && i.statusCategory !== 'done' ? 'blocked' : i.state || stateOf(i.status))

const GROUPS: { id: string; title: string; match: (i: Issue) => boolean }[] = [
  { id: 'progress', title: 'In progress', match: i => st(i) === 'inProgress' || st(i) === 'blocked' },
  { id: 'review', title: 'In review', match: i => st(i) === 'inReview' },
  { id: 'todo', title: 'To do', match: i => st(i) === 'todo' },
  { id: 'done', title: 'Done', match: i => st(i) === 'done' },
]

/** Everything assigned to you — or switch to anyone else / the whole team. */
export function MyWorkView() {
  const { issues, flows, me, users, sprints, setFocusKey, setView, openJira, openTerminal, openCreate } = useStore()
  const [who, setWho] = useState(me?.id ?? ALL)
  // "Current sprint" = any active sprint: with several teams there are
  // several boards, each with its own active sprint.
  const activeSprints = useMemo(() => new Set(sprints.filter(s => s.state === 'active').map(s => s.name)), [sprints])
  const sprintLabel = activeSprints.size === 1 ? [...activeSprints][0] : 'Current sprints'
  const [sprintPref, setSprintPref] = useState(loadSprintOnly)
  const sprintOnly = sprintPref && activeSprints.size > 0
  const setSprintOnly = (f: boolean | ((b: boolean) => boolean)) => setSprintPref(b => { const n = typeof f === 'function' ? f(b) : f; saveSprintOnly(n); return n })
  const [q, setQ] = useState('')
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({ done: true })

  // The bar shows a subset: me, pinned people, whoever is being viewed, Everyone.
  // Anyone else is one search away (Find person, w).
  const [pinned, setPinned] = useState<string[]>(loadPinned)
  const [finding, setFinding] = useState(false)
  const togglePin = (id: string) => setPinned(p => { const n = p.includes(id) ? p.filter(x => x !== id) : [...p, id]; savePinned(n); return n })
  const byId = useMemo(() => new Map(users.map(u => [u.id, u])), [users])
  const bar = useMemo(() => {
    const ids = [me?.id, ...pinned].filter((id): id is string => !!id && byId.has(id))
    const list = [...new Set(ids)]
    if (who !== ALL && !list.includes(who) && byId.has(who)) list.push(who)
    return list.map(id => byId.get(id)!)
  }, [me?.id, pinned, who, byId])
  // Open tickets per person, counted once (not per person per render).
  const openCount = useMemo(() => {
    const n = new Map<string, number>()
    for (const i of Object.values(issues)) {
      if (isWork(i) && i.type !== 'subtask' && i.assignee && i.statusCategory !== 'done') n.set(i.assignee.id, (n.get(i.assignee.id) ?? 0) + 1)
    }
    return n
  }, [issues])

  const order = [...bar.map(u => u.id), ALL]
  const step = (d: number) => setWho(order[(order.indexOf(who) + d + order.length) % order.length])
  useAction('mine.me', () => setWho(me?.id ?? ALL))
  useAction('mine.everyone', () => setWho(ALL))
  useAction('mine.findPerson', () => setFinding(true))
  useAction('mine.nextPerson', () => step(1))
  useAction('mine.prevPerson', () => step(-1))
  useAction('mine.sprint', () => setSprintOnly(s => !s))

  const person = users.find(u => u.id === who)
  const mine = who === me?.id
  const query = q.trim().toLowerCase()

  const items = useMemo(() => Object.values(issues).filter(i => {
    if (!isWork(i)) return false
    if (who !== ALL && i.assignee?.id !== who) return false
    // A sub-task shows on its own only when its parent isn't already listed for this person.
    if (i.type === 'subtask' && (who === ALL || issues[i.parentKey]?.assignee?.id === who)) return false
    if (sprintOnly && !activeSprints.has(i.sprint)) return false
    if (query && !`${i.key} ${i.summary}`.toLowerCase().includes(query)) return false
    return true
  }).sort((a, b) => prio(a) - prio(b)), [issues, who, sprintOnly, activeSprints, query])

  const pts = (f: (i: Issue) => boolean) => items.filter(f).reduce((n, i) => n + i.storyPoints, 0)
  const stats = [
    { label: 'In progress', value: items.filter(GROUPS[0].match).length, tone: 'progress' },
    { label: 'In review', value: items.filter(GROUPS[1].match).length, tone: 'review' },
    { label: 'To do', value: items.filter(GROUPS[2].match).length, tone: 'todo' },
    { label: 'Points done', value: `${pts(i => i.statusCategory === 'done')}/${pts(() => true)}`, tone: 'done' },
  ]

  const focus = (k: string) => { setFocusKey(k); setView('focus') }
  const hour = new Date().getHours()
  const greeting = hour < 12 ? 'Good morning' : hour < 18 ? 'Good afternoon' : 'Good evening'

  return (
    <div className="mywork scroll">
      <div className="view-head">
        <div>
          <h1>{mine ? `${greeting}, ${me?.name.split(' ')[0]}` : person ? `${person.name.split(' ')[0]}'s work` : 'Everyone’s work'}</h1>
          <p className="muted">{mine ? 'Everything assigned to you, wherever it lives in the hierarchy.' : 'You’re looking at someone else’s tickets — you can still open, move and comment on them.'}</p>
        </div>
        <button className="btn primary" onClick={() => openCreate({ assigneeId: who === ALL ? me?.id : who })}><Plus size={14} /> Create</button>
      </div>

      <div className="people">
        {bar.map(u => {
          const isMe = u.id === me?.id
          const isPinned = pinned.includes(u.id)
          return (
            <div key={u.id} className={cx('person', who === u.id && 'on', !isMe && !isPinned && 'temp')} onClick={() => setWho(u.id)}
              role="button" tabIndex={0} title={u.name}>
              <Avatar user={u} size={22} /> {isMe ? 'Me' : u.name.split(' ')[0]}
              <span className="person-count">{openCount.get(u.id) ?? 0}</span>
              {!isMe && (
                <button className="person-pin" title={isPinned ? `Unpin ${u.name}` : `Pin ${u.name} to this bar`}
                  onClick={e => { e.stopPropagation(); togglePin(u.id) }}>
                  {isPinned ? <X size={11} /> : <Pin size={11} />}
                </button>
              )}
            </div>
          )
        })}
        <button className={cx('person', who === ALL && 'on')} onClick={() => setWho(ALL)}><Users size={15} /> Everyone</button>
        <div className="find-person">
          <button className={cx('person', 'find', finding && 'on')} onClick={() => setFinding(f => !f)} title="Find a person (w)">
            <Search size={14} /> Find person <span className="person-count">{users.length}</span>
          </button>
          {finding && (
            <PeoplePicker users={users} me={me?.id} pinned={pinned} counts={openCount} onClose={() => setFinding(false)}
              onView={id => { setWho(id); setFinding(false) }} onTogglePin={togglePin} />
          )}
        </div>
        <span className="spacer" />
        <input data-search className="input slim" value={q} onChange={e => setQ(e.target.value)} placeholder="Filter…" />
        <div className="seg compact" title={activeSprints.size ? `Only tickets in ${activeSprints.size === 1 ? 'the active sprint' : `an active sprint (${[...activeSprints].join(', ')})`} · s` : 'No active sprint on your boards'}>
          <button className={cx(!sprintOnly && 'on')} onClick={() => setSprintOnly(false)}>All</button>
          <button className={cx(sprintOnly && 'on')} disabled={!activeSprints.size} onClick={() => setSprintOnly(true)}>{activeSprints.size ? sprintLabel : 'No active sprint'}</button>
        </div>
      </div>

      {!mine && <div className="viewing-banner"><Avatar user={person ?? null} size={18} /> Viewing {person ? person.name : 'the whole team'} · <button className="link-btn" onClick={() => setWho(me?.id ?? ALL)}>back to my work</button></div>}

      <div className="stats">
        {stats.map(s => (
          <div key={s.label} className={cx('stat card', `tone-${s.tone}`)}>
            <div className="stat-value">{s.value}</div>
            <div className="stat-label">{s.label}</div>
          </div>
        ))}
      </div>

      {GROUPS.map(g => {
        const rows = items.filter(g.match)
        if (!rows.length) return null
        const open = !collapsed[g.id]
        return (
          <section key={g.id} className="card work-group">
            <button className="group-head" onClick={() => setCollapsed(c => ({ ...c, [g.id]: open }))}>
              <ChevronRight size={14} className={cx('chev', open && 'open')} />
              <span className={cx('col-dot', `tone-${g.id}`)} />
              {g.title} <span className="muted">{rows.length}</span>
            </button>
            {open && rows.map(i => {
              const parent = issues[i.parentKey]
              const subs = i.childKeys.map(k => issues[k]).filter(Boolean)
              const f = flows[i.key]
              return (
                <div key={i.key} data-issue={i.key} className="work-row" onClick={() => focus(i.key)}>
                  <TypeIcon type={i.type} size={12} />
                  <span className="mono key">{i.key}</span>
                  <TicketTermLight ticket={i.key} />
                  <div className="work-main">
                    <div className="work-sum">{i.summary}</div>
                    <div className="work-sub">
                      <IssueBadges issue={i} place="column" versions />
                      {parent && <span className="parent-chip" onClick={e => { e.stopPropagation(); openJira(parent.key) }}><TypeIcon type={parent.type} size={9} /> {parent.summary}</span>}
                      {subs.length > 0 && <span className="muted">{subs.filter(s => s.statusCategory === 'done').length}/{subs.length} sub-tasks</span>}
                      {i.links.some(l => l.type === 'is blocked by') && <span className="blocked-chip">blocked by {i.links.filter(l => l.type === 'is blocked by').map(l => l.key).join(', ')}</span>}
                      {f?.started && (
                        <span className="mini-flow" title="Dev flow">{f.stages.map(s => <i key={s.id} className={`seg-${s.state}`} />)}</span>
                      )}
                    </div>
                  </div>
                  <PriorityIcon priority={i.priority} />
                  {i.storyPoints > 0 && <span className="pts">{i.storyPoints}</span>}
                  {who === ALL && <Avatar user={i.assignee} size={22} />}
                  <div onClick={e => e.stopPropagation()}><StatusMenu issueKey={i.key} /></div>
                  <div className="row-actions" onClick={e => e.stopPropagation()}>
                    <button className="icon-btn" title="Focus" onClick={() => focus(i.key)}><Crosshair size={14} /></button>
                    <button className="icon-btn" title="Terminal" onClick={() => openTerminal(i.key)}><SquareTerminal size={14} /></button>
                    <button className="icon-btn" title="Jira view" onClick={() => openJira(i.key)}><Maximize2 size={13} /></button>
                  </div>
                </div>
              )
            })}
          </section>
        )
      })}
      {!items.length && <div className="card empty-card">Nothing here. <button className="link-btn" onClick={() => openCreate({ assigneeId: who === ALL ? me?.id : who })}>Create a ticket</button></div>}
    </div>
  )
}

const P: Record<string, number> = { Highest: 0, High: 1, Medium: 2, Low: 3 }
const SPRINT_KEY = 'maestro.myWorkSprintOnly'

function loadSprintOnly(): boolean {
  try { return localStorage.getItem(SPRINT_KEY) === '1' } catch { return false }
}

function saveSprintOnly(on: boolean) {
  try { localStorage.setItem(SPRINT_KEY, on ? '1' : '0') } catch { /* per-viewer convenience only */ }
}

const PINNED_KEY = 'maestro.pinnedPeople'

/** Pinned people are remembered on this machine (a per-viewer convenience). */
function loadPinned(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(PINNED_KEY) ?? '[]')
    return Array.isArray(v) ? v.filter(x => typeof x === 'string') : []
  } catch { return [] }
}

function savePinned(ids: string[]) {
  try { localStorage.setItem(PINNED_KEY, JSON.stringify(ids)) } catch { /* per-viewer convenience only */ }
}

/** Search the whole org: Enter or click views a person, the pin keeps them in the bar. */
function PeoplePicker({ users, me, pinned, counts, onView, onTogglePin, onClose }: {
  users: User[]; me?: string; pinned: string[]; counts: Map<string, number>
  onView: (id: string) => void; onTogglePin: (id: string) => void; onClose: () => void
}) {
  const [q, setQ] = useState('')
  const [idx, setIdx] = useState(0)
  const box = useRef<HTMLDivElement>(null)
  const query = q.trim().toLowerCase()
  // Pinned first, then people with open work, then by name.
  const list = useMemo(() => users
    .filter(u => u.id !== me && (!query || u.name.toLowerCase().includes(query)))
    .sort((a, b) => Number(pinned.includes(b.id)) - Number(pinned.includes(a.id))
      || (counts.get(b.id) ?? 0) - (counts.get(a.id) ?? 0) || a.name.localeCompare(b.name)), [users, me, query, pinned, counts])
  const shown = list.slice(0, 60)
  useEffect(() => setIdx(0), [query])
  useEffect(() => {
    const away = (e: MouseEvent) => { if (box.current && !box.current.closest('.find-person')?.contains(e.target as Node)) onClose() }
    window.addEventListener('mousedown', away)
    return () => window.removeEventListener('mousedown', away)
  }, [onClose])
  useEffect(() => { box.current?.querySelector('.pp-row.sel')?.scrollIntoView({ block: 'nearest' }) }, [idx])

  return (
    <div className="people-picker" ref={box} onKeyDown={e => {
      e.stopPropagation() // typing here must not trigger view shortcuts
      if (e.key === 'Escape') onClose()
      else if (e.key === 'ArrowDown') { e.preventDefault(); setIdx(i => Math.min(i + 1, shown.length - 1)) }
      else if (e.key === 'ArrowUp') { e.preventDefault(); setIdx(i => Math.max(i - 1, 0)) }
      else if (e.key === 'Enter' && shown[idx]) { e.preventDefault(); if (e.ctrlKey || e.metaKey) onTogglePin(shown[idx].id); else onView(shown[idx].id) }
    }}>
      <div className="pp-search"><Search size={13} /><input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder={`Search ${users.length} people…`} /></div>
      <div className="pp-list">
        {shown.map((u, i) => {
          const isPinned = pinned.includes(u.id)
          return (
            <div key={u.id} className={cx('pp-row', i === idx && 'sel')} onMouseEnter={() => setIdx(i)} onClick={() => onView(u.id)}>
              <Avatar user={u} size={20} />
              <span className="pp-name">{u.name}</span>
              <span className="person-count" title="Open tickets">{counts.get(u.id) ?? 0}</span>
              <button className={cx('pp-pin', isPinned && 'on')} title={isPinned ? 'Unpin from the bar' : 'Pin to the bar'}
                onClick={e => { e.stopPropagation(); onTogglePin(u.id) }}>
                {isPinned ? <PinOff size={13} /> : <Pin size={13} />}
              </button>
            </div>
          )
        })}
        {!shown.length && <div className="pp-empty muted">No one matches “{q}”.</div>}
        {list.length > shown.length && <div className="pp-empty muted">{list.length - shown.length} more — keep typing to narrow it down.</div>}
      </div>
      <div className="pp-foot muted">↑↓ move · Enter view · Ctrl+Enter pin · Esc close</div>
    </div>
  )
}

// Sort: blocked first, then severity (bugs with the severity role), then priority.
const SEV: Record<string, number> = { 'sev-critical': 0, 'sev-major': 1, 'sev-minor': 2, 'sev-low': 3 }
const prio = (i: Issue) => {
  const sev = i.custom.find(c => c.role === 'severity')
  const sevRank = sev ? SEV[severityTone(sev.text)] : 3
  return (st(i) === 'blocked' ? -100 : 0) + sevRank * 10 + (P[i.priority] ?? 2)
}
