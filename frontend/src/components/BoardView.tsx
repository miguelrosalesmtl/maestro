import { useState } from 'react'
import { Maximize2 } from 'lucide-react'
import { useStore } from '../store'
import { cx, flowProgress, isWork, statusTone } from '../lib/util'
import { Avatar, PriorityIcon, TypeIcon } from './bits'
import { useAction } from '../keys'
import { IssueBadges } from './Badges'
import { TicketTermLight } from './TerminalDock'

export function BoardView() {
  const { issues, flows, statuses, me, transition, setFocusKey, setView, openJira, sprint, sprints, openCreate } = useStore()
  const [mine, setMine] = useState(false)
  // Several sprints can be active at once: pick one.
  const active = sprints.filter(s => s.state === 'active')
  const [chosen, setChosen] = useState<string | null>(null)
  const current = active.find(s => s.name === chosen) ?? active.find(s => s.name === sprint) ?? active[0]
  const sprintName = current?.name ?? ''
  const daysLeft = current?.end ? Math.ceil((new Date(current.end).getTime() - Date.now()) / 86400000) : null
  const [dragKey, setDragKey] = useState<string | null>(null)
  const [over, setOver] = useState<string | null>(null)
  useAction('board.mine', () => setMine(m => !m))

  const cards = Object.values(issues).filter(i => isWork(i) && i.type !== 'subtask' && (sprintName ? i.sprint === sprintName : i.statusCategory !== 'done') && (!mine || i.assignee?.id === me?.id))

  return (
    <div className="board-wrap">
      <div className="view-head">
        <div>
          <h1>{sprintName || 'Open work'}</h1>
          {current?.goal && <div className="sprint-goal">🎯 {current.goal}</div>}
          <p className="muted">
            {daysLeft !== null && <b className={cx('ends', daysLeft <= 2 && 'soon')}>{daysLeft > 0 ? `Ends in ${daysLeft} day${daysLeft === 1 ? '' : 's'}` : daysLeft === 0 ? 'Ends today' : `Ended ${-daysLeft}d ago`} · </b>}
            Drag a card to change its Jira status.{!sprintName && ' No active sprint, so all open work is shown.'}
          </p>
        </div>
        <div className="head-actions">
          {active.length > 1 && (
            <div className="seg compact" title="Parallel active sprints">
              {active.map(s => <button key={s.id} className={cx(s.name === sprintName && 'on')} onClick={() => setChosen(s.name)}>{s.name}</button>)}
            </div>
          )}
          <button className="btn ghost" onClick={() => openCreate({ type: 'story', sprint })}>+ Create</button>
          <div className="seg compact">
            <button className={cx(!mine && 'on')} onClick={() => setMine(false)}>Team</button>
            <button className={cx(mine && 'on')} onClick={() => setMine(true)}>Only mine</button>
          </div>
        </div>
      </div>

      <div className="board scroll-x">
        {statuses.map(s => {
          const col = cards.filter(c => c.status === s)
          const pts = col.reduce((n, c) => n + c.storyPoints, 0)
          return (
            <div key={s} className={cx('column', `tone-${statusTone(s)}`, over === s && 'drop-over')}
              onDragOver={e => { e.preventDefault(); setOver(s) }}
              onDragLeave={() => setOver(o => (o === s ? null : o))}
              onDrop={() => { if (dragKey) transition(dragKey, s); setDragKey(null); setOver(null) }}>
              <div className="col-head">
                <span className="col-dot" />
                <span className="col-name">{s}</span>
                <span className="col-count">{col.length}</span>
                <span className="col-pts">{pts} pts</span>
              </div>
              <div className="col-body">
                {col.map(c => {
                  const f = flows[c.key]
                  const p = flowProgress(f)
                  return (
                    <div key={c.key} data-issue={c.key} className={cx('kcard', dragKey === c.key && 'dragging')} draggable
                      onDragStart={() => setDragKey(c.key)} onDragEnd={() => { setDragKey(null); setOver(null) }}
                      onClick={() => { setFocusKey(c.key); setView('focus') }}>
                      <div className="kcard-sum">{c.summary}</div>
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
                        <button className="icon-btn" title="Jira view" onClick={e => { e.stopPropagation(); openJira(c.key) }}><Maximize2 size={12} /></button>
                        <Avatar user={c.assignee} size={20} />
                      </div>
                    </div>
                  )
                })}
                {!col.length && <div className="col-empty">Drop here</div>}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
