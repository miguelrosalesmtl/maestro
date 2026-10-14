import { useEffect, useMemo, useRef, useState } from 'react'
import { ArrowRight, Columns3, Crosshair, FastForward, FolderOpen, Inbox, Layers, Maximize2, Play, Plus, Search, SquareTerminal, Volume2, VolumeX, Zap } from 'lucide-react'
import { api } from '../api'
import { useStore } from '../store'
import { cx, isContainer, statusTone, typeMeta } from '../lib/util'
import { typeInTerminal } from './TerminalDock'
import { compactCombo, useKeys } from '../keys'
import { StatusPill, TypeIcon } from './bits'
import type { ReactNode } from 'react'
import { branchName } from '../lib/naming'

interface Item {
  id: string
  group: string
  icon: ReactNode
  label: ReactNode
  text: string
  hint?: ReactNode
  run: () => void
}

export function CommandPalette() {
  const s = useStore()
  const { bindings } = useKeys()
  const { paletteOpen, setPaletteOpen, issues, flows, focusKey, statuses } = s
  const [q, setQ] = useState('')
  const [idx, setIdx] = useState(0)
  const input = useRef<HTMLInputElement>(null)
  const list = useRef<HTMLDivElement>(null)

  useEffect(() => { if (paletteOpen) { setQ(''); setIdx(0); setTimeout(() => input.current?.focus()) } }, [paletteOpen])

  const items = useMemo<Item[]>(() => {
    const focus = issues[focusKey]
    const f = flows[focusKey]
    const out: Item[] = []
    if (focus) {
      if (s.live) out.push({ id: 'start', group: `Actions · ${focus.key}`, icon: <Play size={15} />, label: <>Start work on <b>{focus.key}</b> in the terminal</>, text: `start work terminal ${focus.key}`,
        run: () => { const session = s.openTerminal(focus.key); void typeInTerminal(session, `git checkout -b ${branchName(s.naming, focus)}`) } })
      else if (!f?.started) out.push({ id: 'start', group: `Actions · ${focus.key}`, icon: <Play size={15} />, label: <>Start work on <b>{focus.key}</b></>, text: `start work ${focus.key}`, run: () => s.startWork(focus.key) })
      else if (f.pr?.state !== 'merged') out.push({ id: 'next', group: `Actions · ${focus.key}`, icon: <FastForward size={15} />, label: <>Simulate next dev event</>, text: 'simulate next event', run: () => s.nextEvent(focus.key) })
      out.push({ id: 'jira', group: `Actions · ${focus.key}`, icon: <Maximize2 size={15} />, label: <>Open Jira view</>, text: `open jira view ${focus.key}`, hint: compactCombo(bindings['jira.open']), run: () => s.openJira(focus.key) })
      for (const st of statuses.filter(x => x !== focus.status)) {
        out.push({ id: `mv-${st}`, group: `Actions · ${focus.key}`, icon: <ArrowRight size={15} />, label: <>Move to <StatusPill status={st} small /></>, text: `move status ${st}`, run: () => s.transition(focus.key, st) })
      }
    }
    if (focus) {
      for (const t of s.childTypes[focus.type] ?? []) {
        out.push({ id: `new-${t}`, group: `Create`, icon: <Plus size={15} />, label: <>New {typeMeta[t].label.toLowerCase()} under <b>{focus.key}</b></>, text: `create new ${t} under child ${focus.key}`, run: () => s.openCreate({ type: t, parentKey: focus.key }) })
      }
    }
    for (const t of ['initiative', 'epic', 'story', 'task', 'bug'] as const) {
      out.push({ id: `create-${t}`, group: 'Create', icon: <Plus size={15} />, label: <>Create {typeMeta[t].label.toLowerCase()}…</>, text: `create new ${t}`, hint: t === 'story' ? compactCombo(bindings['create']) : undefined, run: () => s.openCreate({ type: t }) })
    }
    out.push(
      { id: 'v-mine', group: 'Go to', icon: <Inbox size={15} />, label: 'My work', text: 'go my work mine assigned', hint: compactCombo(bindings['nav.mine']), run: () => s.setView('mine') },
      { id: 'v-focus', group: 'Go to', icon: <Crosshair size={15} />, label: 'Focus', text: 'go focus', hint: compactCombo(bindings['nav.focus']), run: () => s.setView('focus') },
      { id: 'v-board', group: 'Go to', icon: <Columns3 size={15} />, label: 'Board', text: 'go board kanban', hint: compactCombo(bindings['nav.board']), run: () => s.setView('board') },
      { id: 'v-road', group: 'Go to', icon: <Layers size={15} />, label: 'Roadmap', text: 'go roadmap initiatives epics', hint: compactCombo(bindings['nav.roadmap']), run: () => s.setView('roadmap') },
      { id: 'v-auto', group: 'Go to', icon: <Zap size={15} />, label: 'Automations', text: 'go automations rules', hint: compactCombo(bindings['nav.automations']), run: () => s.setView('automations') },
    )
    if (focus) out.push({ id: 'term-new', group: `Actions · ${focus.key}`, icon: <SquareTerminal size={15} />, label: <>New terminal for <b>{focus.key}</b></>, text: `new terminal ${focus.key}`, run: () => { s.openTerminal(focus.key, { fresh: true }) } })
    out.push({ id: 'chirp', group: 'Help', icon: s.chirpMuted ? <Volume2 size={15} /> : <VolumeX size={15} />, label: s.chirpMuted ? 'Unmute terminal chirps' : 'Mute terminal chirps', text: 'mute unmute sound chirp notification terminal', run: () => s.setChirpMuted(!s.chirpMuted) })
    out.push({ id: 'logs', group: 'Help', icon: <FolderOpen size={15} />, label: 'Open log folder', text: 'open log folder logs crash debug help', run: () => void api.openLogFolder() })
    for (const i of Object.values(issues)) {
      out.push({
        id: i.key, group: 'Tickets', icon: <TypeIcon type={i.type} size={12} />,
        label: <><span className="mono key">{i.key}</span> {i.summary}</>, text: `${i.key} ${i.summary}`,
        hint: <span className={cx('dot', `tone-${statusTone(i.status)}`)} />,
        run: () => { if (isContainer(i)) s.openJira(i.key); else { s.setFocusKey(i.key); s.setView('focus') } },
      })
    }
    return out
  }, [issues, flows, focusKey, statuses, s, bindings])

  const terms = q.toLowerCase().split(/\s+/).filter(Boolean)
  const filtered = items.filter(it => terms.every(t => it.text.toLowerCase().includes(t))).slice(0, 40)

  useEffect(() => { setIdx(0) }, [q])
  useEffect(() => { list.current?.querySelector('.pal-item.on')?.scrollIntoView({ block: 'nearest' }) }, [idx])

  if (!paletteOpen) return null
  const run = (it?: Item) => { if (!it) return; it.run(); setPaletteOpen(false) }

  let lastGroup = ''
  return (
    <div className="palette-backdrop fade-in" onMouseDown={() => setPaletteOpen(false)}>
      <div className="palette pop-in" onMouseDown={e => e.stopPropagation()}>
        <div className="pal-input">
          <Search size={17} />
          <input ref={input} value={q} onChange={e => setQ(e.target.value)} placeholder="Search tickets or type a command…"
            onKeyDown={e => {
              if (e.key === 'ArrowDown') { e.preventDefault(); setIdx(i => Math.min(i + 1, filtered.length - 1)) }
              if (e.key === 'ArrowUp') { e.preventDefault(); setIdx(i => Math.max(i - 1, 0)) }
              if (e.key === 'Enter') run(filtered[idx])
              if (e.key === 'Escape') setPaletteOpen(false)
            }} />
          <kbd className="kbd">esc</kbd>
        </div>
        <div className="pal-list scroll" ref={list}>
          {filtered.map((it, i) => {
            const head = it.group !== lastGroup ? it.group : null
            lastGroup = it.group
            return (
              <div key={it.id}>
                {head && <div className="pal-group">{head}</div>}
                <button className={cx('pal-item', i === idx && 'on')} onMouseEnter={() => setIdx(i)} onClick={() => run(it)}>
                  <span className="pal-icon">{it.icon}</span>
                  <span className="pal-label">{it.label}</span>
                  {it.hint && <span className="pal-hint">{it.hint}</span>}
                </button>
              </div>
            )
          })}
          {!filtered.length && <div className="pal-empty">No matches.</div>}
        </div>
      </div>
    </div>
  )
}
