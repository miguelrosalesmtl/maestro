import { useCallback, useEffect, useMemo, useState } from 'react'
import { Activity, Command, Keyboard, Maximize2, Minimize2 } from 'lucide-react'
import { WindowFullscreen, WindowToggleMaximise, WindowUnfullscreen } from '../wailsjs/runtime/runtime'
import { KeysProvider, combosOf, formatCombo, useAction, useKeys, type Scope } from './keys'
import { Combo, LinkHints, PendingKeys, ShortcutsOverlay } from './components/Shortcuts'
import { StoreProvider, useStore } from './store'
import type { Issue, NewIssue, View } from './types'
import { Rail, Sidebar } from './components/Sidebar'
import { FocusView } from './components/FocusView'
import { BoardView } from './components/BoardView'
import { RoadmapView } from './components/RoadmapView'
import { AutomationsView } from './components/AutomationsView'
import { FreestyleView } from './components/FreestyleView'
import { SettingsView } from './components/SettingsView'
import { JiraIssueView } from './components/JiraIssueView'
import { CommandPalette } from './components/CommandPalette'
import { Toasts } from './components/Toasts'
import { TerminalDock } from './components/TerminalDock'
import { MyWorkView } from './components/MyWorkView'
import { CreateIssueModal } from './components/CreateIssueModal'
import { isMac } from './components/bits'
import { timeAgo } from './lib/util'

if (isMac) document.documentElement.classList.add('mac')

const TITLES: Record<View, string> = { mine: 'My work', focus: 'Focus', board: 'Board', roadmap: 'Roadmap', automations: 'Automations', freestyle: 'Freestyle', settings: 'Settings' }

function Shell() {
  const { ready, view, jiraKey, paletteOpen, createDraft, connection } = useStore()
  const [help, setHelp] = useState(false)
  const [hints, setHints] = useState<null | 'click' | 'ticket'>(null)

  // Where the keyboard is "pointed": the Jira view if open, else the current view.
  const scopes = useMemo<Scope[]>(() => jiraKey ? ['global', 'jira'] : ['global', `view:${view}`], [jiraKey, view])
  const blocked = paletteOpen || !!createDraft || help || !!hints

  if (!ready) return (
    <div className="boot"><div className="boot-logo" />
      <span>{connection?.mode === 'connecting' && connection.site ? `Connecting to ${connection.site}…` : 'Tuning the orchestra…'}</span>
    </div>
  )
  return (
    <KeysProvider scopes={scopes} blocked={blocked}>
      <Workspace help={help} setHelp={setHelp} hints={hints} setHints={setHints} />
    </KeysProvider>
  )
}

function Workspace({ help, setHelp, hints, setHints }: {
  help: boolean; setHelp: (b: boolean) => void
  hints: null | 'click' | 'ticket'; setHints: (h: null | 'click' | 'ticket') => void
}) {
  const { view, setView, focusKey, openJira, paletteOpen, setPaletteOpen, activity, termOpen, setTermOpen, openCreate, issues } = useStore()
  const { bindings } = useKeys()

  useAction('help', () => setHelp(true))
  useEffect(() => {
    const open = () => setHelp(true)
    window.addEventListener('maestro:open-help', open)
    return () => window.removeEventListener('maestro:open-help', open)
  }, [setHelp])
  useAction('hints', () => setHints('click'))
  useAction('hints.ticket', () => setHints('ticket'))
  useAction('palette', () => setPaletteOpen(!paletteOpen))
  useAction('create', () => openCreate(createContext(issues[focusKey])))
  useAction('jira.open', () => openJira(focusKey))
  useAction('search', () => {
    const input = document.querySelector<HTMLInputElement>('.view [data-search], .sidebar [data-search]')
    if (input) input.focus(); else setPaletteOpen(true)
  })
  useAction('nav.mine', () => setView('mine'))
  useAction('nav.focus', () => setView('focus'))
  useAction('nav.board', () => setView('board'))
  useAction('nav.roadmap', () => setView('roadmap'))
  useAction('nav.automations', () => setView('automations'))
  useAction('nav.freestyle', () => setView('freestyle'))
  useAction('nav.settings', () => setView('settings'))
  const [freeSeen, setFreeSeen] = useState(false)
  useEffect(() => { if (view === 'freestyle') setFreeSeen(true) }, [view])
  const [fullscreen, setFullscreen] = useState(false)
  const toggleFullscreen = useCallback(() => {
    const next = !fullscreen
    try { if (next) WindowFullscreen(); else WindowUnfullscreen() } catch { /* not in the desktop app */ }
    setFullscreen(next)
  }, [fullscreen])
  const toggleMaximize = () => { try { WindowToggleMaximise() } catch { /* not in the desktop app */ } }
  useAction('window.fullscreen', toggleFullscreen)
  useAction('window.maximize', toggleMaximize)

  // Picking a hint is exactly a click: a ticket opens where clicking it would
  // (Focus, the board card…); the Jira view is a step inside the ticket (J).
  const pick = useCallback((el: HTMLElement) => {
    if (el.matches('input, textarea, select')) el.focus()
    else el.click()
  }, [])


  const latest = activity?.[0]
  return (
    <div className="app">
      <div className="aurora" aria-hidden><i /><i /><i /></div>
      <Rail />
      <Sidebar />
      <div className="main">
        <header className="topbar drag" onDoubleClick={e => { if (e.target === e.currentTarget || (e.target as HTMLElement).classList.contains('topbar-title')) toggleMaximize() }}
          title="Double-click to maximize">
          <div className="topbar-title">{TITLES[view]}</div>
          {latest && (
            <div className="ticker no-drag" key={latest.id} onClick={() => openJira(latest.issueKey)}>
              <Activity size={13} />
              <span className="mono">{latest.issueKey}</span>
              <span className="ticker-text">{latest.title}</span>
              <span className="muted">{timeAgo(latest.at)}</span>
            </div>
          )}
          <button className="btn ghost icon no-drag" onClick={toggleFullscreen}
            title={`${fullscreen ? 'Exit full screen' : 'Full screen'}  ·  ${formatCombo(combosOf(bindings['window.fullscreen'])[0] ?? '').join(' ')}`}>
            {fullscreen ? <Minimize2 size={14} /> : <Maximize2 size={14} />}
          </button>
          <button className="btn ghost no-drag" onClick={() => setHelp(true)}><Keyboard size={13} /> Shortcuts <Combo combo={bindings.help} /></button>
          <button className="btn ghost no-drag" onClick={() => setPaletteOpen(true)}><Command size={13} /> Commands <Combo combo={bindings.palette} /></button>
        </header>
        {view !== 'freestyle' && <div className="view" key={view}>
          {view === 'mine' && <MyWorkView />}
          {view === 'focus' && <FocusView />}
          {view === 'board' && <BoardView />}
          {view === 'roadmap' && <RoadmapView />}
          {view === 'automations' && <AutomationsView />}
          {view === 'settings' && <SettingsView />}
        </div>}
        {/* Mounted from the first visit on, so its terminals keep their screens while you are elsewhere. */}
        {freeSeen && <div className="view" hidden={view !== 'freestyle'}><FreestyleView shown={view === 'freestyle'} /></div>}
        <TerminalDock />
      </div>
      <JiraIssueView />
      <CreateIssueModal />
      <CommandPalette />
      <Toasts />
      <PendingKeys />
      {help && <ShortcutsOverlay onClose={() => setHelp(false)} />}
      {hints && <LinkHints mode={hints} onPick={pick} onClose={() => setHints(null)} />}
    </div>
  )
}

/** Sensible defaults for "C": a sibling story under the epic you're working in. */
function createContext(focus?: Issue): Partial<NewIssue> {
  if (!focus) return {}
  if (focus.type === 'subtask') return { type: 'subtask', parentKey: focus.parentKey }
  if (focus.type === 'epic') return { parentKey: focus.key }
  return { type: 'story', parentKey: focus.parentKey }
}

export default function App() {
  return <StoreProvider><Shell /></StoreProvider>
}
