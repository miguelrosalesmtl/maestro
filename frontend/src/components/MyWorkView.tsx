import { useMemo, useState } from 'react'
import { ChevronRight, Crosshair, Maximize2, Plus, SquareTerminal, Users } from 'lucide-react'
import { useStore } from '../store'
import type { Issue } from '../types'
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
  const { issues, flows, me, users, sprint, setFocusKey, setView, openJira, openTerminal, openCreate } = useStore()
  const [who, setWho] = useState(me?.id ?? ALL)
  const [sprintOnly, setSprintOnly] = useState(false)
  const [q, setQ] = useState('')
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({ done: true })

  const order = [...users.map(u => u.id), ALL]
  const step = (d: number) => setWho(order[(order.indexOf(who) + d + order.length) % order.length])
  useAction('mine.me', () => setWho(me?.id ?? ALL))
  useAction('mine.everyone', () => setWho(ALL))
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
    if (sprintOnly && i.sprint !== sprint) return false
    if (query && !`${i.key} ${i.summary}`.toLowerCase().includes(query)) return false
    return true
  }).sort((a, b) => prio(a) - prio(b)), [issues, who, sprintOnly, sprint, query])

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
        {users.map(u => (
          <button key={u.id} className={cx('person', who === u.id && 'on')} onClick={() => setWho(u.id)}>
            <Avatar user={u} size={22} /> {u.id === me?.id ? 'Me' : u.name.split(' ')[0]}
            <span className="person-count">{Object.values(issues).filter(i => isWork(i) && i.type !== 'subtask' && i.assignee?.id === u.id && i.statusCategory !== 'done').length}</span>
          </button>
        ))}
        <button className={cx('person', who === ALL && 'on')} onClick={() => setWho(ALL)}><Users size={15} /> Everyone</button>
        <span className="spacer" />
        <input data-search className="input slim" value={q} onChange={e => setQ(e.target.value)} placeholder="Filter…" />
        <div className="seg compact">
          <button className={cx(!sprintOnly && 'on')} onClick={() => setSprintOnly(false)}>All</button>
          <button className={cx(sprintOnly && 'on')} onClick={() => setSprintOnly(true)}>{sprint}</button>
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
// Sort: blocked first, then severity (bugs with the severity role), then priority.
const SEV: Record<string, number> = { 'sev-critical': 0, 'sev-major': 1, 'sev-minor': 2, 'sev-low': 3 }
const prio = (i: Issue) => {
  const sev = i.custom.find(c => c.role === 'severity')
  const sevRank = sev ? SEV[severityTone(sev.text)] : 3
  return (st(i) === 'blocked' ? -100 : 0) + sevRank * 10 + (P[i.priority] ?? 2)
}
