import { CalendarClock, Flag, Lock, Package, RotateCw } from 'lucide-react'
import type { Issue } from '../types'
import { cx, dueClass, dueLabel } from '../lib/util'

/** Severity colours for the "severity" custom field role. */
export function severityTone(v?: string): string {
  const s = (v ?? '').toLowerCase()
  if (/(blocker|critical|sev ?1|p0|highest)/.test(s)) return 'sev-critical'
  if (/(major|high|sev ?2|p1)/.test(s)) return 'sev-major'
  if (/(minor|medium|sev ?3|p2)/.test(s)) return 'sev-minor'
  return 'sev-low'
}

/**
 * Small badges for an issue: security lock, flag, carried over, due date,
 * fix version, and custom fields configured for the place.
 */
export function IssueBadges({ issue, place, versions = false }: { issue: Issue; place: 'column' | 'card'; versions?: boolean }) {
  const custom = issue.custom.filter(c => c.show.includes(place) && (c.text || c.items?.length))
  return (
    <span className="badges">
      {issue.security && <span className="badge b-lock" title={`Security level: ${issue.security}`}><Lock size={10} /></span>}
      {issue.flagged && <span className="badge b-flag" title="Flagged (impediment)"><Flag size={10} /></span>}
      {issue.carriedOver > 0 && <span className="badge b-carry" title={`Carried over from ${issue.carriedOver} earlier sprint${issue.carriedOver > 1 ? 's' : ''}`}><RotateCw size={10} />{issue.carriedOver}</span>}
      {issue.dueDate && issue.statusCategory !== 'done' && (
        <span className={cx('badge', dueClass(issue))} title={`Due ${issue.dueDate}`}><CalendarClock size={10} />{dueLabel(issue.dueDate)}</span>
      )}
      {versions && issue.fixVersions.slice(0, 1).map(v => <span key={v} className="badge b-version" title="Fix version"><Package size={10} />{v}</span>)}
      {custom.map(c => (
        <span key={c.id} className={cx('badge b-custom', c.role === 'severity' && severityTone(c.text))} title={c.label}>
          {c.role === 'severity' ? c.text : `${c.label}: ${c.text || c.items?.join(', ')}`}
        </span>
      ))}
    </span>
  )
}
