import { useEffect, useRef, useState } from 'react'
import { Check, Pencil, X } from 'lucide-react'
import type { FieldSpec, FieldValue } from '../types'
import { cx } from '../lib/util'

// Display and edit any field kind. Kinds come from the backend:
// text, url, markdown, number, date, datetime, option, options, cascade, user,
// users, version, versions, labels, group(s), team, unknown.

const MULTI = new Set(['options', 'versions', 'labels', 'users', 'groups'])

/** Shows a value: chips for multi values, a link for URLs, text otherwise. */
export function ValueView({ kind, text, items, empty = 'None' }: { kind: string; text?: string; items?: string[]; empty?: string }) {
  if (MULTI.has(kind)) {
    if (!items?.length) return <span className="muted">{empty}</span>
    return <span className="value-chips">{items.map(i => <span key={i} className="vchip">{i}</span>)}</span>
  }
  if (!text) return <span className="muted">{empty}</span>
  if (kind === 'url') return <a href={text} target="_blank" rel="noreferrer">{text}</a>
  if (kind === 'datetime') return <span>{new Date(text).toLocaleString()}</span>
  if (kind === 'markdown') return <span className="value-md">{text}</span>
  return <span>{text}</span>
}

export const fieldValueOf = (v?: FieldValue) => ({ kind: v?.kind ?? 'text', text: v?.text, items: v?.items })

/**
 * Editor for one field. values: current value(s) as strings.
 * onSave gets the new value(s); an empty array clears the field.
 */
export function FieldEditor({ spec, values, onSave, onCancel, autoFocus = true }: {
  spec: FieldSpec
  values: string[]
  onSave: (v: string[]) => void
  onCancel?: () => void
  autoFocus?: boolean
}) {
  const [draft, setDraft] = useState<string[]>(values)
  useEffect(() => setDraft(values), [values.join('\u0000')]) // eslint-disable-line react-hooks/exhaustive-deps
  const multi = MULTI.has(spec.kind)
  const save = () => onSave(draft.map(s => s.trim()).filter(Boolean))
  // Without save/cancel buttons (forms) every change is reported at once.
  const live = !onCancel
  const first = useRef(true)
  useEffect(() => {
    if (first.current) { first.current = false; return }
    if (live) save()
  }, [draft.join('\u0000')]) // eslint-disable-line react-hooks/exhaustive-deps
  const keys = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey && spec.kind !== 'markdown' && !live) { e.preventDefault(); save() }
    if (e.key === 'Escape') { e.stopPropagation(); onCancel?.() }
  }

  let control
  if ((spec.kind === 'option' || spec.kind === 'cascade' || spec.kind === 'version') && spec.options?.length) {
    control = (
      <select className="input" autoFocus={autoFocus} value={draft[0] ?? ''} onChange={e => setDraft(e.target.value ? [e.target.value] : [])} onKeyDown={keys}>
        <option value="">None</option>
        {spec.options.map(o => <option key={o} value={o}>{o}</option>)}
      </select>
    )
  } else if (multi && spec.options?.length) {
    control = (
      <div className="multi-pick">
        {spec.options.map(o => {
          const on = draft.includes(o)
          return (
            <button type="button" key={o} className={cx('vchip pick', on && 'on')} onClick={() => setDraft(d => on ? d.filter(x => x !== o) : [...d, o])}>
              {on && <Check size={11} />} {o}
            </button>
          )
        })}
      </div>
    )
  } else if (multi) {
    control = <input className="input" autoFocus={autoFocus} value={draft.join(', ')} placeholder="comma separated"
      onChange={e => setDraft(e.target.value.split(',').map(s => s.trim()))} onKeyDown={keys} />
  } else if (spec.kind === 'markdown') {
    control = <textarea className="input" rows={4} autoFocus={autoFocus} value={draft[0] ?? ''} onChange={e => setDraft([e.target.value])} onKeyDown={keys} />
  } else {
    const type = spec.kind === 'number' ? 'number' : spec.kind === 'date' ? 'date' : spec.kind === 'url' ? 'url' : 'text'
    control = <input className="input" type={type} autoFocus={autoFocus} value={draft[0] ?? ''} onChange={e => setDraft([e.target.value])} onKeyDown={keys} />
  }

  return (
    <div className="field-editor">
      {control}
      {onCancel && (
        <div className="fe-actions">
          <button type="button" className="icon-btn" title="Save (Enter)" onClick={save}><Check size={14} /></button>
          <button type="button" className="icon-btn" title="Cancel (Esc)" onClick={onCancel}><X size={14} /></button>
        </div>
      )}
    </div>
  )
}

/** A value with an inline pencil to edit it (Jira view details). */
export function EditableField({ spec, view, values, onSave }: {
  spec?: FieldSpec
  view: React.ReactNode
  values: string[]
  onSave: (v: string[]) => Promise<boolean> | void
}) {
  const [editing, setEditing] = useState(false)
  if (editing && spec) {
    return <FieldEditor spec={spec} values={values} onCancel={() => setEditing(false)}
      onSave={async v => { const ok = await onSave(v); if (ok !== false) setEditing(false) }} />
  }
  return (
    <span className="editable">
      {view}
      {spec && <button type="button" className="icon-btn edit-pen" title={`Edit ${spec.label}`} onClick={() => setEditing(true)}><Pencil size={11} /></button>}
    </span>
  )
}
