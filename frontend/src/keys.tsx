// Keyboard shortcuts: one registry of actions, user-configurable bindings,
// scope-aware dispatch and two-key sequences ("g b").
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import * as App from '../wailsjs/go/main/App'

export type Scope = 'global' | 'jira' | `view:${string}`

export interface ActionDef {
  id: string
  label: string
  group: string
  scope: Scope
  keys: string // default binding(s), "a | b" for alternatives; "" = unbound
  /** Fires even while the palette/terminal/an input has focus (needs a modifier). */
  anywhere?: boolean
}

export const ACTIONS: ActionDef[] = [
  { id: 'help', label: 'Show keyboard shortcuts', group: 'General', scope: 'global', keys: '?' },
  { id: 'hints', label: 'Click anything (link hints)', group: 'General', scope: 'global', keys: 'f' },
  { id: 'hints.ticket', label: 'Pick a ticket (hints on tickets only)', group: 'General', scope: 'global', keys: 'shift+f' },
  { id: 'palette', label: 'Command palette', group: 'General', scope: 'global', keys: 'mod+k', anywhere: true },
  { id: 'search', label: 'Filter / search', group: 'General', scope: 'global', keys: '/' },
  { id: 'create', label: 'Create issue', group: 'General', scope: 'global', keys: 'c' },
  { id: 'jira.open', label: 'Open current ticket in Jira view', group: 'General', scope: 'global', keys: 'j' },
  { id: 'jira.refresh', label: 'Refresh from Jira now', group: 'General', scope: 'global', keys: 'shift+r' },
  { id: 'terminal.toggle', label: 'Jump to terminal / back to the app', group: 'Terminal', scope: 'global', keys: 'ctrl+`', anywhere: true },
  { id: 'terminal.focus', label: 'Focus terminal', group: 'Terminal', scope: 'global', keys: 't' },
  { id: 'terminal.hide', label: 'Show / hide terminal panel', group: 'Terminal', scope: 'global', keys: 'mod+shift+j', anywhere: true },
  { id: 'terminal.new', label: 'New terminal (this ticket, or Freestyle)', group: 'Terminal', scope: 'global', keys: 'mod+shift+t', anywhere: true },
  { id: 'terminal.next', label: 'Next terminal tab', group: 'Terminal', scope: 'global', keys: 'mod+pagedown', anywhere: true },
  { id: 'terminal.prev', label: 'Previous terminal tab', group: 'Terminal', scope: 'global', keys: 'mod+pageup', anywhere: true },
  { id: 'terminal.close', label: 'Close this terminal', group: 'Terminal', scope: 'global', keys: 'mod+shift+w', anywhere: true },

  // Ctrl/⌘ + digit works everywhere, even while typing in the terminal.
  { id: 'nav.mine', label: 'Go to My work', group: 'Navigation', scope: 'global', keys: 'mod+1', anywhere: true },
  { id: 'nav.focus', label: 'Go to Focus', group: 'Navigation', scope: 'global', keys: 'mod+2', anywhere: true },
  { id: 'nav.board', label: 'Go to Board', group: 'Navigation', scope: 'global', keys: 'mod+3', anywhere: true },
  { id: 'nav.roadmap', label: 'Go to Roadmap', group: 'Navigation', scope: 'global', keys: 'mod+4', anywhere: true },
  { id: 'nav.automations', label: 'Go to Automations', group: 'Navigation', scope: 'global', keys: 'mod+5', anywhere: true },
  { id: 'nav.freestyle', label: 'Go to Freestyle terminals', group: 'Navigation', scope: 'global', keys: 'mod+6', anywhere: true },
  { id: 'nav.settings', label: 'Open Settings', group: 'Navigation', scope: 'global', keys: 'mod+,', anywhere: true },
  { id: 'settings.save', label: 'Save settings', group: 'Settings', scope: 'view:settings', keys: 'mod+s', anywhere: true },
  { id: 'window.fullscreen', label: 'Toggle full screen', group: 'Window', scope: 'global', keys: 'f11', anywhere: true },
  { id: 'window.maximize', label: 'Maximize / restore window', group: 'Window', scope: 'global', keys: 'mod+shift+m', anywhere: true },

  { id: 'focus.primary', label: 'Start work / next dev event', group: 'Focus', scope: 'view:focus', keys: 's' },
  { id: 'focus.statusNext', label: 'Move ticket to next status', group: 'Focus', scope: 'view:focus', keys: 'l' },
  { id: 'focus.statusPrev', label: 'Move ticket to previous status', group: 'Focus', scope: 'view:focus', keys: 'h' },
  { id: 'focus.next', label: 'Next ticket in my queue', group: 'Focus', scope: 'view:focus', keys: 'n' },
  { id: 'focus.prev', label: 'Previous ticket in my queue', group: 'Focus', scope: 'view:focus', keys: 'p' },
  { id: 'focus.addSubtask', label: 'Add sub-task', group: 'Focus', scope: 'view:focus', keys: 'a' },
  { id: 'focus.copyBranch', label: 'Copy checkout command', group: 'Focus', scope: 'view:focus', keys: 'y' },
  { id: 'focus.browser', label: 'Open ticket in browser', group: 'Focus', scope: 'view:focus', keys: 'o' },

  { id: 'mine.me', label: 'Show my tickets', group: 'My work', scope: 'view:mine', keys: 'm' },
  { id: 'mine.everyone', label: 'Show everyone’s tickets', group: 'My work', scope: 'view:mine', keys: 'e' },
  { id: 'mine.findPerson', label: 'Find a person (view or pin them)', group: 'My work', scope: 'view:mine', keys: 'w' },
  { id: 'mine.nextPerson', label: 'Next person in the bar', group: 'My work', scope: 'view:mine', keys: ']' },
  { id: 'mine.prevPerson', label: 'Previous person in the bar', group: 'My work', scope: 'view:mine', keys: '[' },
  { id: 'mine.sprint', label: 'Toggle current sprint only (any active sprint)', group: 'My work', scope: 'view:mine', keys: 's' },

  { id: 'board.mine', label: 'Toggle only my cards', group: 'Board', scope: 'view:board', keys: 'm' },
  { id: 'board.find', label: 'Find a Jira board (view or pin it)', group: 'Board', scope: 'view:board', keys: 'w' },
  { id: 'board.next', label: 'Next board in the bar', group: 'Board', scope: 'view:board', keys: ']' },
  { id: 'board.prev', label: 'Previous board in the bar', group: 'Board', scope: 'view:board', keys: '[' },
  { id: 'board.refresh', label: 'Reload the board from Jira', group: 'Board', scope: 'view:board', keys: 'r' },

  { id: 'jira.close', label: 'Close Jira view', group: 'Jira view', scope: 'jira', keys: 'escape' },
  { id: 'jira.comment', label: 'Add comment', group: 'Jira view', scope: 'jira', keys: 'm' },
  { id: 'jira.link', label: 'Link issue', group: 'Jira view', scope: 'jira', keys: 'l' },
  { id: 'jira.addChild', label: 'Add child / sub-task', group: 'Jira view', scope: 'jira', keys: 'a' },
  { id: 'jira.parent', label: 'Go to parent', group: 'Jira view', scope: 'jira', keys: 'u' },
  { id: 'jira.focusTicket', label: 'Focus this ticket in Maestro', group: 'Jira view', scope: 'jira', keys: 'enter' },
  { id: 'jira.browser', label: 'Open in browser', group: 'Jira view', scope: 'jira', keys: 'o' },
]

export const SCOPE_LABEL: Record<string, string> = {
  global: 'Everywhere', jira: 'Jira view', 'view:focus': 'Focus', 'view:mine': 'My work', 'view:board': 'Board',
}

const isMac = /Mac/.test(navigator.platform)

/** Normalised combo for a key event, e.g. "mod+k", "shift+f", "?", "ctrl+`". */
export function comboFromEvent(e: KeyboardEvent): string | null {
  const k = e.key
  if (['Shift', 'Control', 'Alt', 'Meta', 'CapsLock', 'Dead', 'Unidentified'].includes(k)) return null
  let key = k === ' ' ? 'space' : k.length === 1 ? k.toLowerCase() : k.toLowerCase()
  if (e.code === 'Backquote' && (e.ctrlKey || e.metaKey)) key = '`' // ctrl changes e.key on some layouts
  const parts: string[] = []
  const mod = isMac ? e.metaKey : e.ctrlKey
  if (mod) parts.push('mod')
  if (isMac && e.ctrlKey) parts.push('ctrl')
  if (!isMac && e.metaKey) parts.push('meta')
  if (e.altKey) parts.push('alt')
  // Shift is implied by symbols like "?" — only record it for letters and named keys.
  if (e.shiftKey && (/^[a-z]$/.test(key) || k.length > 1)) parts.push('shift')
  parts.push(key)
  // "ctrl+`" is written literally in defaults; map it to the platform mod on non-mac.
  return parts.join('+').replace(/^mod\+`$/, isMac ? 'mod+`' : 'ctrl+`')
}

/** Human-readable label for a binding, e.g. "⌘ K", "g b", "⇧ F". */
/** The alternatives of a binding ("1 | mod+1" → ["1", "mod+1"]). */
export function combosOf(binding: string | undefined): string[] {
  return (binding ?? '').split('|').map(s => s.trim()).filter(Boolean)
}

/** Display parts of one combo, e.g. "⌘ K", "g then b". Use combosOf for alternatives. */
export function formatCombo(combo: string): string[] {
  if (!combo) return []
  if (combo.includes('|')) return combosOf(combo).flatMap((c, i) => i ? ['or', ...formatCombo(c)] : formatCombo(c))
  return combo.split(' ').map(step => step.split('+').map(p => ({
    mod: isMac ? '⌘' : 'Ctrl', ctrl: isMac ? '⌃' : 'Ctrl', alt: isMac ? '⌥' : 'Alt', shift: '⇧', meta: 'Super',
    escape: 'Esc', enter: '↵', space: 'Space', arrowup: '↑', arrowdown: '↓', arrowleft: '←', arrowright: '→', backspace: '⌫', tab: 'Tab',
  } as Record<string, string>)[p] ?? (p.length === 1 ? p.toUpperCase() : p)).join(' ')).flatMap((s, i) => i ? ['then', s] : [s])
}

// Combos of "anywhere" actions, so the terminal can hand them back to the app
// instead of sending them to the shell (see TerminalDock).
let anywhereCombos = new Set<string>()
export function isAppShortcut(e: KeyboardEvent): boolean {
  const c = comboFromEvent(e)
  return !!c && anywhereCombos.has(c)
}

// Combos safe to handle while typing: they carry a modifier, or are function keys (F11).
const hasModifier = (combo: string) => /(^|\+)(mod|ctrl|alt|meta)\+/.test(combo) || /^f\d{1,2}$/.test(combo)

interface KeysCtx {
  bindings: Record<string, string>
  overrides: Record<string, string>
  path: string
  scopes: Scope[]
  pending: string
  setBinding: (id: string, combo: string) => Promise<void>
  resetAll: () => Promise<void>
  register: (id: string, fn: () => void) => () => void
  conflicts: (id: string, combo: string) => ActionDef[]
}

const Ctx = createContext<KeysCtx | null>(null)
export const useKeys = () => {
  const c = useContext(Ctx)
  if (!c) throw new Error('useKeys outside KeysProvider')
  return c
}

/** Register a handler for an action while the calling component is mounted. */
export function useAction(id: string, fn: () => void, enabled = true) {
  const { register } = useKeys()
  const ref = useRef(fn)
  ref.current = fn
  useEffect(() => (enabled ? register(id, () => ref.current()) : undefined), [id, enabled, register])
}

const scopesOverlap = (a: Scope, b: Scope) => a === b || a === 'global' || b === 'global'

export function KeysProvider({ scopes, blocked, children }: { scopes: Scope[]; blocked: boolean; children: ReactNode }) {
  const [overrides, setOverrides] = useState<Record<string, string>>({})
  const [path, setPath] = useState('')
  const [pending, setPending] = useState('')
  const handlers = useRef(new Map<string, (() => void)[]>())
  const pendingRef = useRef('')
  const timer = useRef<number>(0)

  useEffect(() => {
    // Never let a config problem take the UI down: fall back to the defaults.
    Promise.resolve().then(() => App.GetKeybindings())
      .then(r => { setOverrides(r.overrides ?? {}); setPath(r.path) })
      .catch(err => console.warn('keybindings: using defaults', err))
  }, [])

  const bindings = useMemo(
    () => Object.fromEntries(ACTIONS.map(a => [a.id, a.id in overrides ? overrides[a.id] : a.keys])),
    [overrides],
  )

  useEffect(() => {
    // Only modifier combos pass through the terminal; plain keys stay with the shell.
    anywhereCombos = new Set(ACTIONS.filter(a => a.anywhere).flatMap(a => combosOf(bindings[a.id])).filter(hasModifier))
  }, [bindings])

  const persist = useCallback(async (next: Record<string, string>) => {
    setOverrides(next)
    const r = await App.SaveKeybindings(next)
    setPath(r.path)
  }, [])

  const setBinding = useCallback(async (id: string, combo: string) => {
    const def = ACTIONS.find(a => a.id === id)!
    const next = { ...overrides }
    if (combo === def.keys) delete next[id]
    else next[id] = combo
    await persist(next)
  }, [overrides, persist])

  const register = useCallback((id: string, fn: () => void) => {
    const list = handlers.current.get(id) ?? []
    list.push(fn)
    handlers.current.set(id, list)
    return () => {
      const l = handlers.current.get(id) ?? []
      handlers.current.set(id, l.filter(f => f !== fn))
    }
  }, [])

  const conflicts = useCallback((id: string, combo: string) => {
    const self = ACTIONS.find(a => a.id === id)!
    const mine = combosOf(combo)
    return mine.length ? ACTIONS.filter(a => a.id !== id && scopesOverlap(a.scope, self.scope) &&
      combosOf(bindings[a.id]).some(c => mine.includes(c))) : []
  }, [bindings])

  useEffect(() => {
    const clearPending = () => { pendingRef.current = ''; setPending(''); window.clearTimeout(timer.current) }

    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.isComposing) return
      const combo = comboFromEvent(e)
      if (!combo) return
      const target = e.target as HTMLElement
      const typing = !!target.closest?.('input, textarea, select, [contenteditable="true"], .xterm')

      // Vimium-style: Esc leaves a text field instead of triggering anything.
      if (typing && combo === 'escape' && !target.closest('.xterm')) { target.blur(); return }

      // (action, combo) pairs that may fire now. While typing, only combos with
      // a modifier fire, so plain keys keep reaching the text field or shell.
      const pairs = ACTIONS
        .filter(a => a.anywhere || (!blocked && scopes.includes(a.scope)))
        .flatMap(a => combosOf(bindings[a.id]).map(combo => ({ a, combo })))
        .filter(p => !typing || hasModifier(p.combo))
        .filter(p => !blocked || p.a.anywhere)

      const tryRun = (seq: string) => {
        // Most specific scope wins: jira > view > global.
        const hits = pairs.filter(p => p.combo === seq).map(p => p.a).sort((a, b) => rank(b.scope) - rank(a.scope))
        for (const a of hits) {
          const fns = handlers.current.get(a.id)
          if (fns?.length) { e.preventDefault(); e.stopPropagation(); fns[fns.length - 1](); return true }
        }
        return false
      }

      const seq = pendingRef.current ? `${pendingRef.current} ${combo}` : combo
      if (tryRun(seq)) { clearPending(); return }
      if (pairs.some(p => p.combo.startsWith(seq + ' '))) {
        e.preventDefault()
        pendingRef.current = seq
        setPending(seq)
        window.clearTimeout(timer.current)
        timer.current = window.setTimeout(clearPending, 1200)
        return
      }
      if (pendingRef.current) { clearPending(); tryRun(combo) }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [bindings, scopes, blocked])

  const value = useMemo<KeysCtx>(() => ({
    bindings, overrides, path, scopes, pending, setBinding, register, conflicts,
    resetAll: () => persist({}),
  }), [bindings, overrides, path, scopes, pending, setBinding, register, conflicts, persist])

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

const rank = (s: Scope) => (s === 'jira' ? 2 : s === 'global' ? 0 : 1)

/** Compact form for small badges: "⌃1" (Linux/Windows), "⌘1" (macOS), "⇧F". */
export function compactCombo(binding: string | undefined): string {
  const first = combosOf(binding)[0]
  if (!first) return ''
  return first.split(' ').map(step => step.split('+').map(p => ({
    mod: isMac ? '⌘' : '⌃', ctrl: '⌃', alt: isMac ? '⌥' : 'Alt', shift: '⇧', meta: '◆',
  } as Record<string, string>)[p] ?? (p.length === 1 ? p.toUpperCase() : p.toUpperCase())).join('')).join(' ')
}
