import { useEffect, useMemo, useRef, useState } from 'react'
import { AlertTriangle, Keyboard, RotateCcw, Search, X } from 'lucide-react'
import { ACTIONS, SCOPE_LABEL, comboFromEvent, formatCombo, useKeys, type ActionDef } from '../keys'
import { cx } from '../lib/util'

export function Combo({ combo, dim }: { combo: string; dim?: boolean }) {
  if (!combo) return <span className="muted small">unbound</span>
  return (
    <span className={cx('combo-keys', dim && 'dim')}>
      {formatCombo(combo).map((p, i) => p === 'then' || p === 'or' ? <span key={i} className="then">{p}</span> : <kbd key={i} className="kbd">{p}</kbd>)}
    </span>
  )
}

/** "?" sheet: every shortcut for where you are, click one to rebind it. */
export function ShortcutsOverlay({ onClose }: { onClose: () => void }) {
  const { bindings, overrides, scopes, path, setBinding, resetAll, conflicts } = useKeys()
  const [showAll, setShowAll] = useState(false)
  const [q, setQ] = useState('')
  const [editing, setEditing] = useState<string | null>(null)

  const visible = ACTIONS.filter(a => (showAll || scopes.includes(a.scope)) &&
    (!q || `${a.label} ${a.group} ${bindings[a.id]}`.toLowerCase().includes(q.toLowerCase())))
  const groups = useMemo(() => {
    const g: Record<string, ActionDef[]> = {}
    for (const a of visible) (g[a.group] ??= []).push(a)
    return g
  }, [visible])

  useEffect(() => {
    if (editing) return
    const h = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement
      if (e.key === 'Escape' || (e.key === '?' && !target.closest('input'))) { e.preventDefault(); e.stopPropagation(); onClose() }
    }
    window.addEventListener('keydown', h, true)
    return () => window.removeEventListener('keydown', h, true)
  }, [editing, onClose])

  return (
    <div className="modal-backdrop fade-in" onMouseDown={onClose}>
      <div className="shortcuts pop-in" onMouseDown={e => e.stopPropagation()}>
        <header className="sc-head">
          <h2><Keyboard size={18} /> Keyboard shortcuts</h2>
          <div className="sc-search"><Search size={13} /><input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder="Find a shortcut…" /></div>
          <div className="seg compact">
            <button className={cx(!showAll && 'on')} onClick={() => setShowAll(false)}>This view</button>
            <button className={cx(showAll && 'on')} onClick={() => setShowAll(true)}>All</button>
          </div>
          <button className="icon-btn" onClick={onClose}><X size={16} /></button>
        </header>

        <div className="sc-body scroll">
          {Object.entries(groups).map(([group, acts]) => (
            <section key={group} className="sc-group">
              <h3>{group}</h3>
              {acts.map(a => {
                const clash = conflicts(a.id, bindings[a.id])
                const custom = a.id in overrides
                return (
                  <div key={a.id} className={cx('sc-row', editing === a.id && 'editing')}>
                    <span className="sc-label">
                      {a.label}
                      {showAll && a.scope !== 'global' && <em>{SCOPE_LABEL[a.scope] ?? a.scope}</em>}
                    </span>
                    {clash.length > 0 && <span className="sc-clash" title={`Also bound to: ${clash.map(c => c.label).join(', ')}`}><AlertTriangle size={13} /></span>}
                    {custom && editing !== a.id && (
                      <button className="icon-btn" title={`Reset to default (${a.keys || 'unbound'})`} onClick={() => setBinding(a.id, a.keys)}><RotateCcw size={12} /></button>
                    )}
                    {editing === a.id
                      ? <Recorder action={a} onDone={combo => { if (combo !== null) setBinding(a.id, combo); setEditing(null) }} />
                      : <button className={cx('sc-keys', custom && 'custom')} title="Click to change" onClick={() => setEditing(a.id)}><Combo combo={bindings[a.id]} /></button>}
                  </div>
                )
              })}
            </section>
          ))}
          {!visible.length && <div className="muted pad">No shortcuts match.</div>}
        </div>

        <footer className="sc-foot">
          <span className="muted small">Click a shortcut to change it. Saved to <code>{path || 'keybindings.json'}</code></span>
          <span className="spacer" />
          {Object.keys(overrides).length > 0 && <button className="btn ghost small" onClick={resetAll}><RotateCcw size={12} /> Reset all</button>}
        </footer>
      </div>
    </div>
  )
}

/** Captures a new binding: up to two steps ("g b"). Enter saves, Esc cancels, Backspace unbinds. */
function Recorder({ action, onDone }: { action: ActionDef; onDone: (combo: string | null) => void }) {
  const { conflicts } = useKeys()
  const [steps, setSteps] = useState<string[]>([])
  const stepsRef = useRef<string[]>([])
  stepsRef.current = steps

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      e.preventDefault(); e.stopPropagation()
      const combo = comboFromEvent(e)
      if (!combo) return
      const cur = stepsRef.current
      if (combo === 'escape' && !cur.length) return onDone(null)
      if (combo === 'backspace' && !cur.length) return onDone('')
      if (combo === 'enter' && cur.length) return onDone(cur.join(' '))
      if (combo === 'escape') return onDone(cur.join(' ')) // Esc after a key = keep what was typed
      // A modifier combo is complete on its own; plain keys may start a sequence.
      const next = [...cur, combo].slice(-2)
      setSteps(next)
      if (/(mod|ctrl|alt|meta)\+/.test(combo) || next.length === 2) onDone(next.join(' '))
    }
    window.addEventListener('keydown', h, true)
    return () => window.removeEventListener('keydown', h, true)
  }, [onDone])

  const clash = steps.length ? conflicts(action.id, steps.join(' ')) : []
  return (
    <span className="recorder">
      {steps.length ? <Combo combo={steps.join(' ')} /> : <span className="rec-hint">press keys…</span>}
      <span className="rec-help">{steps.length ? '↵ save · or press a 2nd key' : 'Esc cancel · ⌫ unbind'}</span>
      {clash.length > 0 && <span className="sc-clash"><AlertTriangle size={12} /> {clash[0].label}</span>}
    </span>
  )
}

/** Small indicator while a sequence like "g …" is half-typed. */
export function PendingKeys() {
  const { pending } = useKeys()
  if (!pending) return null
  return <div className="pending-keys pop-in"><Combo combo={pending} /> <span className="muted">…</span></div>
}

// ── Link hints (Vimium "f") ───────────────────────────────────────

const CLICKABLE = 'button, a, input, select, textarea, [data-issue], .tree-row, .work-row, .kcard, .queue-row, .pr-card, .crumb'
const ALPHABET = 'sadfjklewcmpgh'

function labels(n: number): string[] {
  let len = 1
  while (ALPHABET.length ** len < n) len++
  const out: string[] = []
  for (let i = 0; out.length < n; i++) {
    let s = '', x = i
    for (let j = 0; j < len; j++) { s = ALPHABET[x % ALPHABET.length] + s; x = Math.floor(x / ALPHABET.length) }
    out.push(s)
  }
  return out
}

interface Hint { el: HTMLElement; label: string; x: number; y: number }

export function LinkHints({ mode, onPick, onClose }: {
  mode: 'click' | 'ticket'
  onPick: (el: HTMLElement) => void
  onClose: () => void
}) {
  const [typed, setTyped] = useState('')
  const hints = useMemo<Hint[]>(() => {
    // Only hint inside the top-most overlay, if one is open.
    const root = document.querySelector('.create, .jira, .palette') ?? document.querySelector('.app')!
    const sel = mode === 'ticket' ? '[data-issue]' : CLICKABLE
    const seen = new Set<Element>()
    const els = Array.from(root.querySelectorAll<HTMLElement>(sel)).filter(el => {
      if (seen.has(el) || el.closest('.hints-layer')) return false
      const r = el.getBoundingClientRect()
      if (r.width < 4 || r.height < 4 || r.bottom < 0 || r.right < 0 || r.top > innerHeight || r.left > innerWidth) return false
      const hit = document.elementFromPoint(Math.min(innerWidth - 1, Math.max(0, r.left + Math.min(r.width / 2, 12))), Math.min(innerHeight - 1, Math.max(0, r.top + r.height / 2)))
      if (!hit || !(el === hit || el.contains(hit) || hit.contains(el))) return false
      seen.add(el)
      return true
    })
    const ls = labels(els.length)
    return els.map((el, i) => {
      const r = el.getBoundingClientRect()
      return { el, label: ls[i], x: Math.max(2, r.left - 4), y: Math.max(2, r.top - 4) }
    })
  }, [mode])

  useEffect(() => {
    if (!hints.length) { onClose(); return }
    const h = (e: KeyboardEvent) => {
      e.preventDefault(); e.stopPropagation()
      if (e.key === 'Escape') return onClose()
      if (e.key === 'Backspace') return setTyped(t => t.slice(0, -1))
      const ch = e.key.toLowerCase()
      if (ch.length !== 1 || !ALPHABET.includes(ch)) return
      setTyped(t => {
        const next = t + ch
        const match = hints.filter(x => x.label.startsWith(next))
        if (match.length === 1 && match[0].label === next) { onClose(); setTimeout(() => onPick(match[0].el)) }
        else if (!match.length) onClose()
        return next
      })
    }
    window.addEventListener('keydown', h, true)
    return () => window.removeEventListener('keydown', h, true)
  }, [hints, onPick, onClose])

  return (
    <div className="hints-layer" onMouseDown={onClose}>
      {hints.filter(h => h.label.startsWith(typed)).map(h => (
        <span key={h.label} className={cx('hint', mode === 'ticket' && 'ticket')} style={{ left: h.x, top: h.y }}>
          <b>{h.label.slice(0, typed.length)}</b>{h.label.slice(typed.length)}
        </span>
      ))}
      <div className="hints-bar">{mode === 'ticket' ? 'Pick a ticket to open in Jira view' : 'Type a hint to click'} · <kbd className="kbd">Esc</kbd> cancel</div>
    </div>
  )
}
