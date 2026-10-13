import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Columns3, LayoutGrid, Loader2, Maximize2, Pin, PinOff, RefreshCw, Search, X } from 'lucide-react'
import { useStore } from '../store'
import type { BoardInfo, Issue, JiraBoard, SprintInfo } from '../types'
import { cx, flowProgress, isWork, statusTone } from '../lib/util'
import { reportError } from '../lib/errors'
import { api } from '../api'
import { Avatar, PriorityIcon, TypeIcon } from './bits'
import { useAction } from '../keys'
import { IssueBadges } from './Badges'
import { TicketTermLight } from './TerminalDock'

const HOME = 'home'

/**
 * Board: "Your sprint" (the active sprint of your projects, by Maestro
 * status) or any Jira board you pin or find, shown with its own Jira
 * columns and any of its sprints.
 */
export function BoardView() {
  const { me } = useStore()
  const [mine, setMine] = useState(false)
  const [pinned, setPinned] = useState<BoardInfo[]>(() => load(PINNED_KEY, []))
  const [selected, setSelectedState] = useState<BoardInfo | typeof HOME>(() => load(SELECTED_KEY, HOME))
  const [finding, setFinding] = useState(false)
  const setSelected = (b: BoardInfo | typeof HOME) => { setSelectedState(b); save(SELECTED_KEY, b) }
  const togglePin = (b: BoardInfo) => setPinned(p => {
    const n = p.some(x => x.id === b.id) ? p.filter(x => x.id !== b.id) : [...p, b]
    save(PINNED_KEY, n)
    return n
  })

  // The bar: Your sprint, pinned boards, the board being viewed (if not pinned).
  const bar = useMemo(() => {
    const list = [...pinned]
    if (selected !== HOME && !list.some(b => b.id === selected.id)) list.push(selected)
    return list
  }, [pinned, selected])
  const order: (BoardInfo | typeof HOME)[] = [HOME, ...bar]
  const at = order.findIndex(o => (o === HOME ? selected === HOME : selected !== HOME && o.id === selected.id))
  const step = (d: number) => setSelected(order[(at + d + order.length) % order.length])
  useAction('board.mine', () => setMine(m => !m))
  useAction('board.find', () => setFinding(true))
  useAction('board.next', () => step(1))
  useAction('board.prev', () => step(-1))

  return (
    <div className="board-wrap">
      <div className="boards-bar">
        <button className={cx('person', selected === HOME && 'on')} onClick={() => setSelected(HOME)} title="The active sprint of your projects">
          <LayoutGrid size={14} /> Your sprint
        </button>
        {bar.map(b => {
          const isPinned = pinned.some(p => p.id === b.id)
          return (
            <div key={b.id} role="button" tabIndex={0} className={cx('person', selected !== HOME && selected.id === b.id && 'on', !isPinned && 'temp')}
              onClick={() => setSelected(b)} title={`${b.name}${b.project ? ` · ${b.project}` : ''} · ${b.type}`}>
              <Columns3 size={14} /> {b.name}
              <button className="person-pin" title={isPinned ? `Unpin ${b.name}` : `Pin ${b.name} to this bar`}
                onClick={e => { e.stopPropagation(); togglePin(b) }}>
                {isPinned ? <X size={11} /> : <Pin size={11} />}
              </button>
            </div>
          )
        })}
        <div className="find-person">
          <button className={cx('person', 'find', finding && 'on')} onClick={() => setFinding(f => !f)} title="Find a Jira board (w)">
            <Search size={14} /> Find board
          </button>
          {finding && (
            <BoardPicker pinned={pinned} onClose={() => setFinding(false)} onTogglePin={togglePin}
              onView={b => { setSelected(b); setFinding(false) }} />
          )}
        </div>
        <span className="spacer" />
        <div className="seg compact">
          <button className={cx(!mine && 'on')} onClick={() => setMine(false)}>Team</button>
          <button className={cx(mine && 'on')} onClick={() => setMine(true)}>Only mine</button>
        </div>
      </div>

      {selected === HOME
        ? <HomeBoard mine={mine} />
        : <JiraBoardPanel key={selected.id} board={selected} mine={mine} me={me?.id} />}
    </div>
  )
}

// ── Your sprint (synced issues, Maestro statuses) ───────────────────

function HomeBoard({ mine }: { mine: boolean }) {
  const { issues, statuses, me, transition, sprint, sprints, openCreate } = useStore()
  // Several sprints can be active at once: pick one.
  const active = sprints.filter(s => s.state === 'active')
  const [chosen, setChosen] = useState<string | null>(null)
  const current = active.find(s => s.name === chosen) ?? active.find(s => s.name === sprint) ?? active[0]
  const sprintName = current?.name ?? ''
  const cards = Object.values(issues).filter(i => isWork(i) && i.type !== 'subtask' && (sprintName ? i.sprint === sprintName : i.statusCategory !== 'done') && (!mine || i.assignee?.id === me?.id))

  return (
    <>
      <div className="view-head">
        <div>
          <h1>{sprintName || 'Open work'}</h1>
          {current?.goal && <div className="sprint-goal">🎯 {current.goal}</div>}
          <p className="muted"><SprintDates sprint={current} />Drag a card to change its Jira status.{!sprintName && ' No active sprint, so all open work is shown.'}</p>
        </div>
        <div className="head-actions">
          {active.length > 1 && (
            <div className="seg compact" title="Parallel active sprints">
              {active.map(s => <button key={s.id} className={cx(s.name === sprintName && 'on')} onClick={() => setChosen(s.name)}>{s.name}</button>)}
            </div>
          )}
          <button className="btn ghost" onClick={() => openCreate({ type: 'story', sprint })}>+ Create</button>
        </div>
      </div>
      <Columns columns={statuses.map(s => ({ name: s, tone: statusTone(s), cards: cards.filter(c => c.status === s), dropTo: s }))}
        onDrop={(key, to) => transition(key, to)} />
    </>
  )
}

// ── Any Jira board ──────────────────────────────────────────────────

function JiraBoardPanel({ board, mine, me }: { board: BoardInfo; mine: boolean; me?: string }) {
  const { transition } = useStore()
  const [sprints, setSprints] = useState<SprintInfo[] | null>(null)
  const [sprintId, setSprintId] = useState<number | null>(() => load(sprintKey(board.id), null))
  const [view, setView] = useState<JiraBoard | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const seq = useRef(0)

  // Sprints of the board; keep the remembered sprint if it still exists, else the active one.
  useEffect(() => {
    api.boardSprints(board.id).then(ss => {
      setSprints(ss)
      setSprintId(id => (id !== null && (id === 0 ? !ss.length : ss.some(s => s.id === id)) ? id : ss.find(s => s.state === 'active')?.id ?? ss[0]?.id ?? 0))
    }).catch(err => { setError(String(err)); setSprints([]) })
  }, [board.id])

  const reload = useCallback(() => {
    if (sprintId === null) return
    const n = ++seq.current
    setLoading(true)
    api.board(board.id, sprintId)
      .then(v => { if (n === seq.current) { setView(v); setError('') } })
      .catch(err => { if (n === seq.current) { setError(String(err)); reportError('warn', err, 'open board') } })
      .finally(() => { if (n === seq.current) setLoading(false) })
  }, [board.id, sprintId])
  useEffect(() => { reload() }, [reload])
  useAction('board.refresh', reload)
  const pick = (id: number) => { setSprintId(id); save(sprintKey(board.id), id) }

  const byKey = useMemo(() => new Map((view?.issues ?? []).map(i => [i.key, i])), [view])
  const sprint = view?.sprint ?? sprints?.find(s => s.id === sprintId) ?? null
  const columns = (view?.columns ?? []).map(c => ({
    name: c.name,
    tone: statusTone(c.statuses[0] ?? ''),
    cards: c.keys.map(k => byKey.get(k)).filter((i): i is Issue => !!i && (!mine || i.assignee?.id === me)),
    dropTo: c.statuses[0],
  }))

  return (
    <>
      <div className="view-head">
        <div>
          <h1>{board.name}{sprint ? <span className="board-sprint"> · {sprint.name}</span> : null}</h1>
          {sprint?.goal && <div className="sprint-goal">🎯 {sprint.goal}</div>}
          <p className="muted">
            <SprintDates sprint={sprint ?? undefined} />
            {board.project && <>{board.project} · </>}{board.type === 'kanban' || sprints?.length === 0 ? 'Open work and anything resolved in the last 14 days.' : 'Drag a card to change its Jira status.'}
            {view?.truncated && <b className="ends soon"> · showing the first {view.issues.length} issues</b>}
          </p>
        </div>
        <div className="head-actions">
          {!!sprints?.length && (
            <select className="input slim sprint-select" value={sprintId ?? ''} onChange={e => pick(Number(e.target.value))} title="Sprint">
              {(['active', 'future', 'closed'] as const).map(state => {
                const group = sprints.filter(s => s.state === state)
                return group.length ? (
                  <optgroup key={state} label={state === 'active' ? 'Active' : state === 'future' ? 'Upcoming' : 'Recently closed'}>
                    {group.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}
                  </optgroup>
                ) : null
              })}
            </select>
          )}
          <button className="btn ghost icon" title="Reload from Jira (r)" onClick={reload}>{loading ? <Loader2 size={14} className="spin" /> : <RefreshCw size={14} />}</button>
        </div>
      </div>
      {error && <div className="board-error">Could not load this board: {error}</div>}
      {!view && !error && <div className="board-loading"><Loader2 size={18} className="spin" /> Loading {board.name}…</div>}
      {view && (
        <Columns columns={columns} onDrop={async (key, to) => { await transition(key, to); reload() }} />
      )}
    </>
  )
}

function SprintDates({ sprint }: { sprint?: SprintInfo }) {
  if (!sprint?.end) return null
  const days = Math.ceil((new Date(sprint.end).getTime() - Date.now()) / 86400000)
  if (sprint.state === 'closed') return <b className="ends">Closed {new Date(sprint.end).toLocaleDateString()} · </b>
  if (sprint.state === 'future') return null
  return <b className={cx('ends', days <= 2 && 'soon')}>{days > 0 ? `Ends in ${days} day${days === 1 ? '' : 's'}` : days === 0 ? 'Ends today' : `Ended ${-days}d ago`} · </b>
}

// ── Columns and cards (shared) ──────────────────────────────────────

type Col = { name: string; tone: string; cards: Issue[]; dropTo?: string }

function Columns({ columns, onDrop }: { columns: Col[]; onDrop: (key: string, toStatus: string) => void }) {
  const { issues, flows, setFocusKey, setView, openJira, adoptIssue } = useStore()
  const [dragKey, setDragKey] = useState<string | null>(null)
  const [over, setOver] = useState<string | null>(null)
  // A card from another team's board may not be synced yet: make it known first.
  const open = (c: Issue) => { adoptIssue(c); setFocusKey(c.key); setView('focus') }

  return (
    <div className="board scroll-x" style={{ gridTemplateColumns: `repeat(${Math.max(columns.length, 1)}, minmax(220px, 1fr))` }}>
      {columns.map(col => {
        const pts = col.cards.reduce((n, c) => n + c.storyPoints, 0)
        return (
          <div key={col.name} className={cx('column', `tone-${col.tone}`, over === col.name && 'drop-over')}
            onDragOver={e => { if (col.dropTo) { e.preventDefault(); setOver(col.name) } }}
            onDragLeave={() => setOver(o => (o === col.name ? null : o))}
            onDrop={() => { if (dragKey && col.dropTo) onDrop(dragKey, col.dropTo); setDragKey(null); setOver(null) }}>
            <div className="col-head">
              <span className="col-dot" />
              <span className="col-name">{col.name}</span>
              <span className="col-count">{col.cards.length}</span>
              <span className="col-pts">{pts} pts</span>
            </div>
            <div className="col-body">
              {col.cards.map(c => {
                const f = flows[c.key]
                const p = flowProgress(f)
                return (
                  <div key={c.key} data-issue={c.key} className={cx('kcard', dragKey === c.key && 'dragging')} draggable
                    onDragStart={() => setDragKey(c.key)} onDragEnd={() => { setDragKey(null); setOver(null) }}
                    onClick={() => open(c)}>
                    <div className="kcard-sum">{c.security ? '🔒 secured' : c.summary}</div>
                    <div className="kcard-labels">{c.labels.map(l => <span key={l} className="label tiny">{l}</span>)}</div>
                    <IssueBadges issue={c} place="card" versions />
                    {f?.started && (
                      <div className="kcard-flow" title={`${Math.round(p * 100)}% through dev flow`}>
                        {f.stages.map(st => <i key={st.id} className={`seg-${st.state}`} />)}
                      </div>
                    )}
                    <div className="kcard-foot">
                      <TypeIcon type={c.type} size={11} />
                      <span className="mono key">{c.key}</span>
                      <TicketTermLight ticket={c.key} />
                      <PriorityIcon priority={c.priority} />
                      <span className="spacer" />
                      {c.childKeys.length > 0 && <span className="sub-count" title="Sub-tasks done">{c.childKeys.filter(k => issues[k]?.statusCategory === 'done').length}/{c.childKeys.length}</span>}
                      {c.storyPoints > 0 && <span className="pts">{c.storyPoints}</span>}
                      <button className="icon-btn" title="Jira view" onClick={e => { e.stopPropagation(); adoptIssue(c); openJira(c.key) }}><Maximize2 size={12} /></button>
                      <Avatar user={c.assignee} size={20} />
                    </div>
                  </div>
                )
              })}
              {!col.cards.length && <div className="col-empty">{col.dropTo ? 'Drop here' : 'Empty'}</div>}
            </div>
          </div>
        )
      })}
    </div>
  )
}

// ── Find board ──────────────────────────────────────────────────────

function BoardPicker({ pinned, onView, onTogglePin, onClose }: {
  pinned: BoardInfo[]; onView: (b: BoardInfo) => void; onTogglePin: (b: BoardInfo) => void; onClose: () => void
}) {
  const [q, setQ] = useState('')
  const [found, setFound] = useState<BoardInfo[] | null>(null)
  const [error, setError] = useState('')
  const [idx, setIdx] = useState(0)
  const box = useRef<HTMLDivElement>(null)

  // Search Jira as you type (debounced); an empty query lists the first boards.
  useEffect(() => {
    let live = true
    const t = setTimeout(() => {
      api.searchBoards(q.trim())
        .then(bs => { if (live) { setFound(bs); setError(''); setIdx(0) } })
        .catch(err => { if (live) setError(String(err)) })
    }, q ? 250 : 0)
    return () => { live = false; clearTimeout(t) }
  }, [q])
  useEffect(() => {
    const away = (e: MouseEvent) => { if (box.current && !box.current.closest('.find-person')?.contains(e.target as Node)) onClose() }
    window.addEventListener('mousedown', away)
    return () => window.removeEventListener('mousedown', away)
  }, [onClose])

  const isPinned = (b: BoardInfo) => pinned.some(p => p.id === b.id)
  // Pinned boards first.
  const list = [...(found ?? [])].sort((a, b) => Number(isPinned(b)) - Number(isPinned(a)))
  useEffect(() => { box.current?.querySelector('.pp-row.sel')?.scrollIntoView({ block: 'nearest' }) }, [idx])

  return (
    <div className="people-picker board-picker" ref={box} onKeyDown={e => {
      e.stopPropagation()
      if (e.key === 'Escape') onClose()
      else if (e.key === 'ArrowDown') { e.preventDefault(); setIdx(i => Math.min(i + 1, list.length - 1)) }
      else if (e.key === 'ArrowUp') { e.preventDefault(); setIdx(i => Math.max(i - 1, 0)) }
      else if (e.key === 'Enter' && list[idx]) { e.preventDefault(); if (e.ctrlKey || e.metaKey) onTogglePin(list[idx]); else onView(list[idx]) }
    }}>
      <div className="pp-search"><Search size={13} /><input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder="Search Jira boards by name…" /></div>
      <div className="pp-list">
        {found === null && !error && <div className="pp-empty muted"><Loader2 size={13} className="spin" /> Searching…</div>}
        {list.map((b, i) => (
          <div key={b.id} className={cx('pp-row', i === idx && 'sel')} onMouseEnter={() => setIdx(i)} onClick={() => onView(b)}>
            <Columns3 size={15} className="muted" />
            <span className="pp-name">{b.name}</span>
            {b.project && <span className="mono small muted">{b.project}</span>}
            <span className="board-type">{b.type}</span>
            <button className={cx('pp-pin', isPinned(b) && 'on')} title={isPinned(b) ? 'Unpin from the bar' : 'Pin to the bar'}
              onClick={e => { e.stopPropagation(); onTogglePin(b) }}>
              {isPinned(b) ? <PinOff size={13} /> : <Pin size={13} />}
            </button>
          </div>
        ))}
        {found !== null && !list.length && !error && <div className="pp-empty muted">No board matches “{q}”.</div>}
        {error && <div className="pp-empty board-error">{error}</div>}
        {found !== null && found.length >= 50 && <div className="pp-empty muted">Showing 50 — keep typing to narrow it down.</div>}
      </div>
      <div className="pp-foot muted">↑↓ move · Enter view · Ctrl+Enter pin · Esc close</div>
    </div>
  )
}

// ── Remembered on this machine (per-viewer conveniences) ────────────

const PINNED_KEY = 'maestro.pinnedBoards'
const SELECTED_KEY = 'maestro.selectedBoard'
const sprintKey = (boardId: number) => `maestro.boardSprint.${boardId}`

function load<T>(key: string, fallback: T): T {
  try {
    const v = localStorage.getItem(key)
    return v === null ? fallback : (JSON.parse(v) as T)
  } catch { return fallback }
}

function save(key: string, v: unknown) {
  try { localStorage.setItem(key, JSON.stringify(v)) } catch { /* ignore */ }
}
