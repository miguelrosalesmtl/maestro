import { useCallback, useEffect, useRef, useState } from 'react'
import { Plus, SquareDashedBottomCode, X } from 'lucide-react'
import { useStore } from '../store'
import { useAction } from '../keys'
import { cx } from '../lib/util'
import { TermLight, TermPane } from './TerminalDock'

/**
 * Freestyle: terminals that belong to no ticket, for investigations and
 * anything else. Shells start in the home folder, get no ticket variables,
 * and their git/gh commands never move a ticket.
 */
export function FreestyleView({ shown }: { shown: boolean }) {
  const { freeSessions, activeFree, selectFree, newFree, closeFree, renameFree, termActivity } = useStore()
  const [editing, setEditing] = useState<string | null>(null)
  const active = activeFree && freeSessions.some(f => f.id === activeFree) ? activeFree : freeSessions.at(-1)?.id ?? null

  const names = useRef(freeSessions)
  names.current = freeSessions
  const banner = useCallback((session: string) => {
    const name = names.current.find(f => f.id === session)?.name ?? 'Freestyle'
    return [
      `\x1b[38;2;34;211;238m●\x1b[0m \x1b[1m${name}\x1b[0m  \x1b[2mfreestyle · not linked to a ticket\x1b[0m`,
      `\x1b[2m  commands here never change a ticket\x1b[0m`, '', '',
    ].join('\r\n')
  }, [])

  // Arriving with no terminal opens one: this view is only terminals.
  // (Deferred and cancelled on unmount, so a double mount opens only one.)
  useEffect(() => {
    if (!shown || freeSessions.length) return
    const t = setTimeout(() => newFree(), 0)
    return () => clearTimeout(t)
  }, [shown]) // eslint-disable-line react-hooks/exhaustive-deps

  const cycle = (d: number) => {
    if (!freeSessions.length) return
    const i = Math.max(0, freeSessions.findIndex(f => f.id === active))
    selectFree(freeSessions[(i + d + freeSessions.length) % freeSessions.length].id)
  }
  const focusActive = () => { if (active) selectFree(active); else newFree() }
  // Hidden (another view is open): the ticket dock owns the terminal keys.
  useAction('terminal.new', () => { newFree() }, shown)
  useAction('terminal.next', () => cycle(1), shown)
  useAction('terminal.prev', () => cycle(-1), shown)
  useAction('terminal.close', () => { if (active) closeFree(active) }, shown)
  useAction('terminal.focus', focusActive, shown)
  useAction('terminal.toggle', () => {
    if (document.activeElement?.closest('.xterm')) (document.activeElement as HTMLElement).blur()
    else focusActive()
  }, shown)

  return (
    <div className="freestyle">
      <aside className="free-list">
        <div className="free-head">
          <span><SquareDashedBottomCode size={14} /> Freestyle</span>
          <button className="btn primary small" title="New terminal (Ctrl+Shift+T)" onClick={() => newFree()}><Plus size={13} /> New</button>
        </div>
        <p className="free-note muted">Terminals for anything that is not a ticket. They start in your home folder and never move a ticket.</p>
        <div className="free-items">
          {freeSessions.map(f => {
            const act = termActivity[f.id]
            return (
              <div key={f.id} className={cx('free-item', f.id === active && 'on')} onClick={() => selectFree(f.id)}
                onDoubleClick={() => setEditing(f.id)} title="Double-click to rename">
                <TermLight light={act ?? { state: 'idle' }} />
                {editing === f.id ? (
                  <input autoFocus defaultValue={f.name} className="free-rename" onClick={e => e.stopPropagation()}
                    onBlur={e => { renameFree(f.id, e.currentTarget.value); setEditing(null) }}
                    onKeyDown={e => {
                      if (e.key === 'Enter') e.currentTarget.blur()
                      if (e.key === 'Escape') { e.currentTarget.value = f.name; e.currentTarget.blur() }
                      e.stopPropagation()
                    }} />
                ) : (
                  <span className="free-name">{f.name}</span>
                )}
                {act && act.state !== 'idle' && act.command && <span className="tab-cmd">{act.command}</span>}
                <button className="icon-btn" title="Close terminal (Ctrl+Shift+W)" onClick={e => { e.stopPropagation(); closeFree(f.id) }}><X size={12} /></button>
              </div>
            )
          })}
        </div>
        <div className="free-keys muted small">
          <span>Ctrl+Shift+T new</span><span>Ctrl+PgUp/PgDn switch</span><span>Ctrl+Shift+W close</span><span>Ctrl+` focus / back</span>
        </div>
      </aside>
      <div className="free-body">
        {freeSessions.map(f => <TermPane key={f.id} session={f.id} active={f.id === active} banner={banner} />)}
        {!freeSessions.length && (
          <div className="dock-empty">
            <span>No freestyle terminal open.</span>
            <button className="btn primary small" onClick={() => newFree()}><Plus size={13} /> New terminal</button>
          </div>
        )}
      </div>
    </div>
  )
}
