import { memo, useCallback, useEffect, useRef, useState } from 'react'
import { Terminal as XTerm } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'
import { ChevronDown, ChevronUp, GitBranch, Plus, SquareTerminal, X } from 'lucide-react'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { api } from '../api'
import { sessionTicket, ticketLight, useStore } from '../store'
import type { TermLightState } from '../types'
import { cx, isWork, statusTone } from '../lib/util'
import { branchName, prCreateCommand, prMergeCommand } from '../lib/naming'
import { StatusMenu, TypeIcon } from './bits'
import { isAppShortcut, useAction } from '../keys'
import { reportError } from '../lib/errors'

// One global listener fans PTY output out to the pane that owns the ticket.
// end is the session's output length after the chunk (see TermPane).
const writers = new Map<string, (b: Uint8Array, end: number) => void>()
const decode = (b64: string) => {
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  return bytes
}
EventsOn('term:data', (key: string, b64: string, end: number) => writers.get(key)?.(decode(b64), end))
const focusers = new Map<string, () => void>()

/**
 * Types a command into a ticket's terminal without pressing Enter, so the
 * user stays in control. Waits for the pane to mount and the shell to start.
 */
export async function typeInTerminal(key: string, cmd: string) {
  for (let i = 0; i < 40 && !writers.has(key); i++) await new Promise(r => setTimeout(r, 75))
  await new Promise(r => setTimeout(r, 250))
  await api.termWrite(key, '\x15' + cmd) // ctrl-u clears a half-typed line
  focusers.get(key)?.()
}
// Focus requests can arrive before a new pane has mounted; wait for it (≤ 2 s).
window.addEventListener('maestro:focus-terminal', async e => {
  const session = (e as CustomEvent<string>).detail
  for (let i = 0; i < 40 && !focusers.has(session); i++) await new Promise(r => setTimeout(r, 50))
  focusers.get(session)?.()
})

const THEME = {
  background: '#0b0b11', foreground: '#d8d9e6', cursor: '#22d3ee', cursorAccent: '#0b0b11',
  selectionBackground: '#7c5cff55',
  black: '#1b1b26', red: '#ff5c7a', green: '#34d399', yellow: '#f5b83d', blue: '#4c9aff', magenta: '#a78bfa', cyan: '#22d3ee', white: '#d8d9e6',
  brightBlack: '#6c6e82', brightRed: '#ff8299', brightGreen: '#6ee7b7', brightYellow: '#fcd34d', brightBlue: '#7cb7ff', brightMagenta: '#c4b5fd', brightCyan: '#67e8f9', brightWhite: '#ffffff',
}

// A pane does not read the store: it would re-render on every app change.
// banner() is read once, when the shell starts.
export const TermPane = memo(function TermPane({ session, active, banner }: { session: string; active: boolean; banner: (session: string) => string }) {
  const host = useRef<HTMLDivElement>(null)
  const fitRef = useRef<(() => void) | null>(null)

  useEffect(() => {
    const term = new XTerm({
      theme: THEME, fontFamily: "'JetBrains Mono Variable', ui-monospace, Menlo, monospace", fontSize: 12.5, lineHeight: 1.25,
      cursorBlink: true, cursorStyle: 'bar', allowProposedApi: true, scrollback: 5000, macOptionIsMeta: true,
    })
    // App shortcuts (Ctrl+`, Ctrl+Shift+T…) go to Maestro, not to the shell.
    term.attachCustomKeyEventHandler(e => !isAppShortcut(e))
    const fit = new FitAddon()
    // xterm cannot measure a hidden element (inactive tab, collapsed dock): fitting
    // then throws inside its renderer. Fit only when the pane is laid out.
    const safeFit = () => {
      const el = host.current
      if (!el || !el.offsetParent || el.clientWidth === 0 || el.clientHeight === 0) return
      try { fit.fit() } catch (err) { reportError('warn', err, 'terminal fit') }
    }
    fitRef.current = safeFit
    term.loadAddon(fit)
    term.loadAddon(new WebLinksAddon((_, uri) => api.openURL(uri)))
    term.open(host.current!)
    safeFit()

    // Output is numbered by its end offset. A pane recreated while its shell
    // kept running (a reload, an error recovery) refills from the backend's
    // history; output that arrives meanwhile waits, and nothing is written twice.
    let shown = 0, ready = false
    const queued: [Uint8Array, number][] = []
    const write = (b: Uint8Array, end: number) => {
      if (end <= shown) return
      const start = end - b.length
      term.write(start < shown ? b.subarray(shown - start) : b)
      shown = end
    }
    const flush = () => { ready = true; for (const [b, e] of queued.splice(0)) write(b, e) }
    writers.set(session, (b, end) => { if (ready) write(b, end); else queued.push([b, end]) })
    focusers.set(session, () => term.focus())
    api.termOpen(session, term.cols, term.rows).then(async created => {
      if (created) term.write(banner(session))
      else {
        // Never type into the shell here: a program waiting for input would take it.
        const h = await api.termHistory(session).catch(() => null)
        if (h?.data) write(decode(h.data), h.end)
      }
    }).finally(flush)
    const d1 = term.onData(d => api.termWrite(session, d))
    const d2 = term.onResize(({ cols, rows }) => api.termResize(session, cols, rows))
    const ro = new ResizeObserver(() => safeFit())
    ro.observe(host.current!)

    return () => {
      ro.disconnect(); d1.dispose(); d2.dispose()
      writers.delete(session); focusers.delete(session)
      // xterm queues viewport work right after open(); disposing synchronously
      // (React StrictMode remount, fast tab close) makes that work throw. Let it run first.
      requestAnimationFrame(() => setTimeout(() => term.dispose(), 0))
    }
    // Panes live as long as the session tab; ticket data is only read for the banner.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session])

  useEffect(() => {
    // Fit only: the terminal takes keyboard focus when you ask for it, never on its own.
    if (active) requestAnimationFrame(() => fitRef.current?.())
  }, [active, session])

  return <div ref={host} className={cx('term-pane', active && 'active')} />
})

const SHARE_KEY = 'maestro.terminalShare'
const DEFAULT_SHARE = 0.33

function loadShare(): number {
  try {
    const v = Number(localStorage.getItem(SHARE_KEY))
    return v > 0.1 && v < 0.9 ? v : DEFAULT_SHARE
  } catch { return DEFAULT_SHARE }
}

function saveShare(v: number) {
  try { localStorage.setItem(SHARE_KEY, v.toFixed(3)) } catch { /* per-viewer convenience only */ }
}

/** At least 140 px of terminal, and at least 180 px left for the view above. */
function clampHeight(h: number, viewport: number) {
  return Math.round(Math.min(Math.max(140, viewport - 180), Math.max(140, h)))
}

const terminalHasFocus = () => !!document.activeElement?.closest('.xterm')

export function TerminalDock() {
  const { termOpen, setTermOpen, termSessions, activeSession, selectSession, openTerminal, closeTerminal, focusKey, issues, flows, termActivity, view, naming, commands } = useStore()
  // Freestyle has its own full-size terminals and Settings needs the room:
  // the ticket dock steps aside (its shells keep running).
  const away = view === 'freestyle' || view === 'settings'
  // The panel's height is a share of the window, so it scales with the screen;
  // dragging the edge changes the share, which is remembered.
  const [share, setShare] = useState(loadShare)
  const [viewport, setViewport] = useState(window.innerHeight)
  useEffect(() => {
    const onResize = () => setViewport(window.innerHeight)
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])
  const height = clampHeight(share * viewport, viewport)
  const drag = useRef<{ y: number; h: number } | null>(null)
  const focused = issues[focusKey]

  // The dock shows the selected ticket's terminals only. Selecting a ticket
  // never starts a shell; the user opens one when they want it.
  const ticketKey = focused && isWork(focused) ? focusKey : activeSession ? sessionTicket(activeSession) : undefined
  const mine = ticketKey ? termSessions.filter(s => sessionTicket(s) === ticketKey) : []
  const active = activeSession && mine.includes(activeSession) ? activeSession : mine[mine.length - 1] ?? null
  const issue = ticketKey ? issues[ticketKey] : undefined
  const flow = ticketKey ? flows[ticketKey] : undefined
  // Terminals of other tickets: one entry per ticket, with its light.
  const others = [...new Set(termSessions.map(sessionTicket))].filter(t => t !== ticketKey)
  const [othersOpen, setOthersOpen] = useState(false)

  const workTicket = ticketKey
  const cycle = (d: number) => {
    if (!mine.length) return
    const i = active ? mine.indexOf(active) : -1
    selectSession(mine[(i + d + mine.length) % mine.length])
  }

  // Ctrl+`: hidden → open and focus; typing in it → back to the app; else focus it.
  useAction('terminal.toggle', () => {
    if (away) return
    if (!termOpen) { if (workTicket) openTerminal(workTicket); else setTermOpen(true); return }
    if (terminalHasFocus()) { (document.activeElement as HTMLElement | null)?.blur(); return }
    if (active) selectSession(active)
    else if (workTicket) openTerminal(workTicket)
  })
  useAction('terminal.focus', () => { if (!away && workTicket) openTerminal(workTicket) })
  useAction('terminal.hide', () => {
    if (away) return
    if (termOpen && terminalHasFocus()) (document.activeElement as HTMLElement | null)?.blur()
    setTermOpen(!termOpen)
  })
  useAction('terminal.new', () => { if (!away && workTicket) openTerminal(workTicket, { fresh: true }) })
  useAction('terminal.next', () => { if (!away) cycle(1) })
  useAction('terminal.prev', () => { if (!away) cycle(-1) })
  useAction('terminal.close', () => { if (!away && active) closeTerminal(active) })

  useEffect(() => {
    const move = (e: MouseEvent) => {
      if (!drag.current) return
      const h = clampHeight(drag.current.h + drag.current.y - e.clientY, window.innerHeight)
      setShare(h / window.innerHeight)
    }
    const up = () => {
      if (drag.current) setShare(s => { saveShare(s); return s })
      drag.current = null
      document.body.classList.remove('resizing')
    }
    window.addEventListener('mousemove', move)
    window.addEventListener('mouseup', up)
    return () => { window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up) }
  }, [])

  const insert = (cmd: string) => {
    if (!active) return
    api.termWrite(active, '\x15' + cmd) // ctrl-u clears whatever was half-typed
    focusers.get(active)?.()
  }

  // Names come from Settings (branch and PR templates). A secured issue's
  // summary never goes into a branch or PR (naming.ts leaves it out).
  const branch = flow?.branch || (issue ? branchName(naming, issue) : '')
  const chips: { label: string; cmd: string; show: boolean }[] = issue ? [
    { label: 'create branch', cmd: `git checkout -b ${branch}`, show: !flow?.started },
    { label: 'checkout', cmd: `git checkout ${branch}`, show: !!flow?.started },
    { label: 'commit', cmd: `git commit -am "${issue.key}: "`, show: true },
    { label: 'push', cmd: `git push -u origin ${flow?.branch || '$MAESTRO_BRANCH'}`, show: true },
    { label: 'open PR', cmd: prCreateCommand(naming, issue, branch), show: !flow?.pr },
    { label: 'merge PR', cmd: prMergeCommand(naming), show: !!flow?.pr && flow.pr.state !== 'merged' },
    ...commands.map(c => ({ label: c.label, cmd: c.command, show: true })),
  ] : []

  // Stable for the memoized panes; reads the latest data when called.
  const data = useRef({ issues, flows })
  data.current = { issues, flows }
  const banner = useCallback((session: string) => {
    const ticket = sessionTicket(session)
    const is = data.current.issues[ticket]
    const branch = data.current.flows[ticket]?.branch
    const n = session.split('#')[1]
    const summary = is?.security ? '(secured issue)' : is?.summary ?? ''
    return [
      `\x1b[38;2;124;92;255m●\x1b[0m \x1b[1m${ticket}\x1b[0m${n ? `  \x1b[2mterminal ${n}\x1b[0m` : ''}  ${summary}`,
      `\x1b[2m  status ${is?.status ?? '?'}${branch ? `  ·  branch ${branch}` : ''}  ·  $MAESTRO_TICKET is set\x1b[0m`,
      `\x1b[2m  git/gh commands run here update the ticket automatically\x1b[0m`, '', '',
    ].join('\r\n')
  }, [])

  const ticketSessions = (t: string) => termSessions.filter(s => sessionTicket(s) === t)

  return (
    <section className={cx('dock', !termOpen && 'collapsed')} style={away ? { display: 'none' } : termOpen ? { height } : undefined}>
      {termOpen && <div className="dock-grip" onMouseDown={e => { drag.current = { y: e.clientY, h: height }; document.body.classList.add('resizing') }} />}
      <header className="dock-head">
        <button className="dock-title" onClick={() => setTermOpen(!termOpen)}>
          <SquareTerminal size={14} /> Terminal
        </button>
        <div className="dock-tabs">
          {mine.map(s => {
            const t = sessionTicket(s)
            const n = s.split('#')[1]
            const act = termActivity[s]
            return (
              <div key={s} className={cx('dock-tab', s === active && termOpen && 'on')} onClick={() => selectSession(s)}
                title={act && act.state !== 'idle' ? `${act.command ?? 'command'} — ${act.state}` : `${t}${n ? ` terminal ${n}` : ''}`}>
                <TermLight light={act ?? { state: 'idle' }} fallbackTone={statusTone(issues[t]?.status ?? '')} />
                <span className="mono">{t}</span>
                {n && <span className="tab-n">#{n}</span>}
                {act && act.state !== 'idle' && act.command && <span className="tab-cmd">{act.command}</span>}
                <button className="icon-btn" title="Close terminal" onClick={e => { e.stopPropagation(); closeTerminal(s) }}><X size={11} /></button>
              </div>
            )
          })}
          {workTicket && (
            <button className="dock-tab add" title={`New terminal for ${workTicket}`} onClick={() => openTerminal(workTicket, { fresh: ticketSessions(workTicket).length > 0 })}>
              <Plus size={12} /> {mine.length ? 'new' : workTicket}
            </button>
          )}
        </div>
        {others.length > 0 && (
          <div className="dock-others">
            <button className={cx('dock-tab', 'others-btn', othersOpen && 'on')} title="Terminals of other tickets" onClick={() => setOthersOpen(o => !o)}>
              {others.slice(0, 4).map(t => <TicketDot key={t} ticket={t} />)}
              {others.length} other{others.length === 1 ? '' : 's'} <ChevronDown size={11} />
            </button>
            {othersOpen && (
              <div className="others-menu" onMouseLeave={() => setOthersOpen(false)}>
                {others.map(t => {
                  const ss = termSessions.filter(s => sessionTicket(s) === t)
                  return (
                    <button key={t} onClick={() => { setOthersOpen(false); selectSession(ss[ss.length - 1]) }}>
                      <TicketDot ticket={t} />
                      <span className="mono">{t}</span>
                      <span className="om-sum">{issues[t]?.security ? '🔒 secured' : issues[t]?.summary}</span>
                      {ss.length > 1 && <span className="tab-n">×{ss.length}</span>}
                    </button>
                  )
                })}
              </div>
            )}
          </div>
        )}
        <span className="spacer" />
        <span className="muted small dock-hint">Ctrl+` · Ctrl+Shift+T new</span>
        <button className="icon-btn" onClick={() => setTermOpen(!termOpen)}>{termOpen ? <ChevronDown size={15} /> : <ChevronUp size={15} />}</button>
      </header>

      {termOpen && issue && (
        <div className="dock-context">
          <TypeIcon type={issue.type} size={11} />
          <span className="mono ctx-key">{issue.key}</span>
          <span className="ctx-sum">{issue.security ? '🔒 secured' : issue.summary}</span>
          <StatusMenu issueKey={issue.key} />
          {branch && <span className="ctx-branch mono"><GitBranch size={12} /> {branch}</span>}
          <span className="spacer" />
          <div className="ctx-chips">
            {active && chips.filter(c => c.show).map(c => (
              <button key={c.label} className="ctx-chip" title={c.cmd} onClick={() => insert(c.cmd)}>{c.label}</button>
            ))}
          </div>
        </div>
      )}

      <div className="dock-body" style={{ display: termOpen ? undefined : 'none' }}>
        {termSessions.map(s => <TermPane key={s} session={s} active={s === active} banner={banner} />)}
        {!active && (
          <div className="dock-empty">
            {ticketKey ? (
              <>
                <span>No terminal for <b className="mono">{ticketKey}</b> yet.</span>
                <button className="btn primary small" onClick={() => openTerminal(ticketKey)}><SquareTerminal size={13} /> Open terminal</button>
                <span className="muted small">Ctrl+` or Ctrl+Shift+T</span>
              </>
            ) : 'Pick a ticket to open its terminal.'}
          </div>
        )}
      </div>
    </section>
  )
}

/** The activity light: waiting (amber, pulsing), running (cyan, spinning), finished (green), idle. */
export function TermLight({ light, fallbackTone, size = 8 }: { light: TermLightState; fallbackTone?: string; size?: number }) {
  if (light.state === 'idle') return <span className={cx('dot', fallbackTone ? `tone-${fallbackTone}` : 'term-idle')} />
  return <span className={cx('term-light', `tl-${light.state}`)} style={{ width: size, height: size }} />
}

/** Small light for a ticket in the "other terminals" menu. */
function TicketDot({ ticket }: { ticket: string }) {
  const { termSessions, termActivity, issues } = useStore()
  const light = ticketLight(ticket, termSessions, termActivity) ?? { state: 'idle' as const }
  return <TermLight light={light} fallbackTone={statusTone(issues[ticket]?.status ?? '')} size={7} />
}

/** Light for a ticket across all its terminals; nothing if it has none. */
export function TicketTermLight({ ticket }: { ticket: string }) {
  const { termSessions, termActivity } = useStore()
  const light = ticketLight(ticket, termSessions, termActivity)
  if (!light) return null
  const tip = light.state === 'idle' ? 'Terminal open'
    : light.state === 'finished' ? `${light.command ?? 'Command'} finished`
    : `${light.command ?? 'Command'} ${light.state}`
  return <span className={cx('ticket-term', `tt-${light.state}`)} title={tip}><SquareTerminal size={11} />{light.state !== 'idle' && <i />}</span>
}
