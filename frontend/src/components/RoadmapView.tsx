import { ChevronRight, Plus } from 'lucide-react'
import { useStore } from '../store'
import type { Issue } from '../types'
import { cx, leaves, statusTone } from '../lib/util'
import { Avatar, StatusPill, TypeIcon } from './bits'

function breakdown(items: Issue[]) {
  const total = items.length || 1
  const by = (cat: string) => items.filter(i => i.statusCategory === cat).length
  return { done: by('done') / total, prog: by('inprogress') / total, todo: by('todo') / total, count: items.length, doneCount: by('done') }
}

function Ring({ value }: { value: number }) {
  const r = 26, c = 2 * Math.PI * r
  return (
    <svg width="68" height="68" viewBox="0 0 68 68" className="ring">
      <circle cx="34" cy="34" r={r} className="ring-bg" />
      <circle cx="34" cy="34" r={r} className="ring-fg" strokeDasharray={c} strokeDashoffset={c * (1 - value)} />
      <text x="34" y="38" textAnchor="middle">{Math.round(value * 100)}%</text>
    </svg>
  )
}

export function RoadmapView() {
  const { issues, openJira, setFocusKey, setView, openCreate } = useStore()
  const initiatives = Object.values(issues).filter(i => i.type === 'initiative')
  const orphanEpics = Object.values(issues).filter(i => i.type === 'epic' && !issues[i.parentKey])

  return (
    <div className="roadmap scroll">
      <div className="view-head">
        <div>
          <h1>Roadmap</h1>
          <p className="muted">Initiative → Epic → work. Progress rolls up from the tickets you move.</p>
        </div>
        <button className="btn primary" onClick={() => openCreate({ type: 'initiative' })}><Plus size={14} /> Initiative</button>
      </div>

      {initiatives.map(ini => {
        const all = leaves(ini, issues)
        const b = breakdown(all)
        return (
          <section key={ini.key} className="card initiative">
            <div className="ini-head">
              <Ring value={b.done} />
              <div className="ini-title">
                <button className="hero-key" onClick={() => openJira(ini.key)}><TypeIcon type="initiative" size={13} /> {ini.key}</button>
                <h2>{ini.summary}</h2>
                <div className="muted small">{b.doneCount} of {b.count} tickets done · owned by {ini.assignee?.name}</div>
              </div>
              <button className="btn ghost small" onClick={() => openCreate({ type: 'epic', parentKey: ini.key })}><Plus size={12} /> Epic</button>
              <StatusPill status={ini.status} />
            </div>

            <div className="epics">
              {ini.childKeys.map(k => issues[k]).filter(Boolean).map(ep => <EpicRow key={ep.key} ep={ep} />)}
            </div>
          </section>
        )
      })}

      {orphanEpics.length > 0 && (
        <section className="card initiative">
          <div className="ini-head">
            <div className="ini-title">
              <h2>{initiatives.length ? 'Epics without an initiative' : 'Epics'}</h2>
              <div className="muted small">{orphanEpics.length} epics</div>
            </div>
            <button className="btn ghost small" onClick={() => openCreate({ type: 'epic' })}><Plus size={12} /> Epic</button>
          </div>
          <div className="epics">{orphanEpics.map(ep => <EpicRow key={ep.key} ep={ep} />)}</div>
        </section>
      )}
    </div>
  )
}

function EpicRow({ ep }: { ep: Issue }) {
  const { issues, openJira, setFocusKey, setView, openCreate } = useStore()
  const kids = leaves(ep, issues)
  const eb = breakdown(kids)
  return (
    <div className="epic-row">
      <button data-issue={ep.key} className="epic-name" onClick={() => openJira(ep.key)}>
        <TypeIcon type="epic" size={12} />
        <span className="mono key">{ep.key}</span>
        <span>{ep.summary}</span>
        <ChevronRight size={13} className="muted" />
      </button>
      <div className="epic-bar" title={`${eb.doneCount}/${eb.count} done`}>
        <i className="b-done" style={{ width: `${eb.done * 100}%` }} />
        <i className="b-prog" style={{ width: `${eb.prog * 100}%` }} />
      </div>
      <div className="epic-chips">
        {kids.map(t => (
          <button key={t.key} data-issue={t.key} className={cx('chip', `tone-${statusTone(t.status)}`)} title={`${t.key} · ${t.summary} — ${t.status}`}
            onClick={() => { setFocusKey(t.key); setView('focus') }}>
            {t.key.split('-')[1]}
          </button>
        ))}
      </div>
      <button className="icon-btn" title={`Add story under ${ep.key}`} onClick={() => openCreate({ parentKey: ep.key })}><Plus size={13} /></button>
      <Avatar user={ep.assignee} size={22} />
    </div>
  )
}
