import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Bookmark, Bug, CheckSquare, ChevronDown, Crown, Zap, ArrowUp, ArrowDown, ChevronsUp, Equal } from 'lucide-react'
import type { IssueType, User } from '../types'
import { cx, statusTone } from '../lib/util'
import { useStore } from '../store'
import { api } from '../api'

export function TypeIcon({ type, size = 14 }: { type: IssueType; size?: number }) {
  const Icon = { initiative: Crown, epic: Zap, story: Bookmark, task: CheckSquare, bug: Bug, subtask: CheckSquare }[type]
  return (
    <span className={cx('type-icon', `type-${type}`)} style={{ width: size + 6, height: size + 6 }}>
      <Icon size={size - 2} strokeWidth={2.5} />
    </span>
  )
}

export function PriorityIcon({ priority }: { priority: string }) {
  const map: Record<string, [typeof ArrowUp, string]> = {
    Highest: [ChevronsUp, 'var(--red)'],
    High: [ArrowUp, 'var(--orange)'],
    Medium: [Equal, 'var(--amber)'],
    Low: [ArrowDown, 'var(--blue)'],
  }
  const [Icon, color] = map[priority] ?? map.Medium
  return <Icon size={14} color={color} strokeWidth={2.5} aria-label={priority} />
}

export function Avatar({ user, size = 24 }: { user: User | null; size?: number }) {
  if (!user) return <span className="avatar avatar-empty" style={{ width: size, height: size }} />
  return (
    <span className="avatar" title={user.name}
      style={{ width: size, height: size, fontSize: size * 0.4, background: `linear-gradient(135deg, ${user.color}, ${user.color}99)` }}>
      {user.initials}
    </span>
  )
}

export function StatusPill({ status, small }: { status: string; small?: boolean }) {
  return <span className={cx('status-pill', `tone-${statusTone(status)}`, small && 'small')}><i />{status}</span>
}

/** Click-to-change status dropdown, wired to the tracker. */
export function StatusMenu({ issueKey, variant = 'pill' }: { issueKey: string; variant?: 'pill' | 'jira' }) {
  const { issues, statuses, transition } = useStore()
  const issue = issues[issueKey]
  const [open, setOpen] = useState(false)
  const [targets, setTargets] = useState<string[] | null>(null) // null = loading
  const ref = useRef<HTMLDivElement>(null)
  useOutside(ref, () => setOpen(false))
  useEffect(() => {
    if (!open) return
    setTargets(null)
    let alive = true
    api.transitions(issueKey)
      .then(ts => { if (alive) setTargets([...new Set(ts.map(t => t.to))]) })
      .catch(() => { if (alive) setTargets(statuses.filter(s => s !== issue?.status)) })
    return () => { alive = false }
  }, [open, issueKey]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!issue) return null
  return (
    <div className="status-menu" ref={ref}>
      <button className={cx(variant === 'jira' ? 'jira-status-btn' : 'status-pill as-button', `tone-${statusTone(issue.status)}`)}
        onClick={e => { e.stopPropagation(); setOpen(o => !o) }}>
        {variant === 'pill' && <i />}{issue.status}<ChevronDown size={13} />
      </button>
      {open && (
        <div className="menu pop-in" onClick={e => e.stopPropagation()}>
          <div className="menu-label">Transition to</div>
          {targets === null && <div className="menu-label">Loading…</div>}
          {targets?.length === 0 && <div className="menu-label">No transitions available</div>}
          {(targets ?? []).filter(s => s !== issue.status).map(s => (
            <button key={s} className="menu-item" onClick={() => { transition(issueKey, s); setOpen(false) }}>
              <StatusPill status={s} small />
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

export function useOutside(ref: React.RefObject<HTMLElement | null>, cb: () => void) {
  useEffect(() => {
    const h = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) cb() }
    document.addEventListener('mousedown', h)
    return () => document.removeEventListener('mousedown', h)
  }, [ref, cb])
}

export function Kbd({ children }: { children: ReactNode }) {
  return <kbd className="kbd">{children}</kbd>
}

export const isMac = typeof navigator !== 'undefined' && /Mac/.test(navigator.platform)
export const modKey = isMac ? '⌘' : 'Ctrl'

/** Minimal, safe markdown: headings, lists, checkboxes, bold, inline code. */
export function Markdown({ text }: { text: string }) {
  const blocks: ReactNode[] = []
  let list: { ordered: boolean; items: ReactNode[] } | null = null
  const flush = () => {
    if (!list) return
    blocks.push(list.ordered ? <ol key={blocks.length}>{list.items}</ol> : <ul key={blocks.length}>{list.items}</ul>)
    list = null
  }
  text.split('\n').forEach((raw, i) => {
    const line = raw.trimEnd()
    let m
    if ((m = line.match(/^(#{1,3})\s+(.*)/))) {
      flush()
      const H = (`h${m[1].length + 2}`) as 'h3' | 'h4' | 'h5'
      blocks.push(<H key={i}>{inline(m[2])}</H>)
    } else if ((m = line.match(/^-\s+\[( |x)\]\s+(.*)/))) {
      if (!list || list.ordered) { flush(); list = { ordered: false, items: [] } }
      list.items.push(<li key={i} className={cx('md-check', m[1] === 'x' && 'checked')}><span className="checkbox" />{inline(m[2])}</li>)
    } else if ((m = line.match(/^(-|\d+\.)\s+(.*)/))) {
      const ordered = m[1] !== '-'
      if (!list || list.ordered !== ordered) { flush(); list = { ordered, items: [] } }
      list.items.push(<li key={i}>{inline(m[2])}</li>)
    } else if (line === '') {
      flush()
    } else {
      flush()
      blocks.push(<p key={i}>{inline(line)}</p>)
    }
  })
  flush()
  return <div className="md">{blocks}</div>
}

function inline(s: string): ReactNode[] {
  return s.split(/(\*\*[^*]+\*\*|`[^`]+`)/g).map((part, i) => {
    if (part.startsWith('**')) return <strong key={i}>{part.slice(2, -2)}</strong>
    if (part.startsWith('`')) return <code key={i}>{part.slice(1, -1)}</code>
    return part
  })
}
