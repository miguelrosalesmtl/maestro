import { useEffect, useMemo, useRef, useState } from 'react'
import { X } from 'lucide-react'
import { useStore } from '../store'
import type { FieldSpec, IssueType, NewIssue } from '../types'
import { ancestors, cx, typeMeta } from '../lib/util'
import { Avatar, Kbd, PriorityIcon, TypeIcon, modKey } from './bits'
import { Combobox } from './Combobox'
import { FieldEditor } from './Fields'
import { api } from '../api'

const ALL_TYPES: IssueType[] = ['initiative', 'epic', 'story', 'task', 'bug', 'subtask']
const PRIORITIES = ['Highest', 'High', 'Medium', 'Low']

/** Jira-style "Create issue" dialog. Opened with defaults via store.openCreate(). */
export function CreateIssueModal() {
  const { createDraft, closeCreate, issues, users, me, childTypes, createIssue, openJira, projects, sprints, versions, createDefaults } = useStore()
  const [form, setForm] = useState<NewIssue>(blank)
  const [labels, setLabels] = useState('')
  const [another, setAnother] = useState(false)
  const [busy, setBusy] = useState(false)
  const [specs, setSpecs] = useState<FieldSpec[]>([])
  const [specError, setSpecError] = useState('')
  const [custom, setCustom] = useState<Record<string, string[]>>({})
  const summary = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!createDraft) return
    const parent = createDraft.parentKey ? issues[createDraft.parentKey] : undefined
    const type = createDraft.type ?? (parent ? childTypes[parent.type]?.[0] : 'story') ?? 'story'
    setForm({ ...blank(), assigneeId: me?.id ?? '', ...createDraft, type })
    setLabels((createDraft.labels ?? []).join(', '))
    setTimeout(() => summary.current?.focus())
  }, [createDraft]) // eslint-disable-line react-hooks/exhaustive-deps

  // Which types can own the selected type.
  const parentTypes = useMemo(
    () => (Object.keys(childTypes) as IssueType[]).filter(p => childTypes[p].includes(form.type)),
    [childTypes, form.type],
  )
  const parentOptions = useMemo(() => {
    const opts = Object.values(issues)
      .filter(i => parentTypes.includes(i.type))
      .sort((a, b) => a.key.localeCompare(b.key, undefined, { numeric: true }))
      .map(i => ({
        value: i.key,
        text: `${i.key} ${i.summary}`,
        label: <span className="opt"><TypeIcon type={i.type} size={11} /><span className="mono key">{i.key}</span><span className="opt-sum">{i.summary}</span></span>,
      }))
    const optional = form.type !== 'subtask' && form.type !== 'initiative'
    return optional ? [{ value: '', text: 'none', label: <span className="muted">No parent</span> }, ...opts] : opts
  }, [issues, parentTypes, form.type])

  // The project the issue lands in: explicit, else the parent's (initiatives
  // live in their own project), else the first project in scope.
  const parentIssue = issues[form.parentKey]
  const projKey = form.project || (parentIssue && parentIssue.type !== 'initiative' ? parentIssue.key.split('-')[0] : '') || projects[0] || ''
  const projSprints = sprints.filter(s => s.project === projKey)
  const projVersions = (versions[projKey] ?? []).filter(v => !v.released)

  // Defaults from config whenever project or type changes.
  useEffect(() => {
    if (!createDraft) return
    const pickSprint = (d: string) => d === 'active' ? projSprints.find(s => s.state === 'active')?.name ?? ''
      : d === 'next' ? projSprints.find(s => s.state === 'future')?.name ?? '' : ''
    const pickVersion = (d: string) => d === 'none' ? [] : d === 'next' ? projVersions.slice(0, 1).map(v => v.name)
      : projVersions.some(v => v.name === d) ? [d] : []
    setForm(f => ({
      ...f,
      sprint: createDraft.sprint ?? pickSprint(createDefaults.sprint),
      fixVersions: createDraft.fixVersions ?? pickVersion(createDefaults.fixVersion),
      affectsVersions: f.type === 'bug' ? pickVersion(createDefaults.affectsVersion) : [],
    }))
  }, [createDraft, projKey, form.type]) // eslint-disable-line react-hooks/exhaustive-deps

  // Extra fields from Jira's create screen and fields.custom (FLD-11).
  useEffect(() => {
    if (!createDraft || !projKey) return
    let alive = true
    setSpecError('')
    api.createFieldSpecs(projKey, form.type).then(list => {
      if (!alive) return
      setSpecs(list)
      setCustom(Object.fromEntries(list.filter(s => s.default).map(s => [s.id, [s.default!]])))
    }).catch(err => { if (alive) { setSpecs([]); setSpecError(String(err).replace(/^Error: /, '')) } })
    return () => { alive = false }
  }, [createDraft, projKey, form.type])

  if (!createDraft) return null

  const set = <K extends keyof NewIssue>(k: K, v: NewIssue[K]) => setForm(f => ({ ...f, [k]: v }))
  const setType = (t: IssueType) => setForm(f => {
    const p = issues[f.parentKey]
    const keep = p && childTypes[p.type]?.includes(t)
    return { ...f, type: t, parentKey: keep ? f.parentKey : '' }
  })

  const parent = issues[form.parentKey]
  const path = parent ? [...ancestors(parent, issues), parent] : []
  const isPlanning = form.type === 'initiative' || form.type === 'epic'
  const needsParent = form.type === 'subtask' && !form.parentKey
  const missingRequired = specs.filter(s => s.required && !(custom[s.id] ?? []).some(v => v.trim()))
  const valid = form.summary.trim() !== '' && !needsParent && missingRequired.length === 0 && !specError

  const submit = async () => {
    if (!valid || busy) return
    setBusy(true)
    const is = await createIssue({
      ...form,
      labels: labels.split(',').map(l => l.trim()).filter(Boolean),
      sprint: isPlanning || form.type === 'subtask' ? '' : form.sprint,
      affectsVersions: form.type === 'bug' ? form.affectsVersions : [],
      custom,
      storyPoints: isPlanning || form.type === 'subtask' ? 0 : form.storyPoints,
    })
    setBusy(false)
    if (!is) return
    if (another) {
      setForm(f => ({ ...f, summary: '', description: '' }))
      summary.current?.focus()
    } else {
      closeCreate()
      if (isPlanning) openJira(is.key)
    }
  }

  return (
    <div className="modal-backdrop fade-in" onMouseDown={closeCreate}
      onKeyDown={e => {
        if (e.key === 'Escape') closeCreate()
        if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submit()
      }}>
      <div className="create pop-in" onMouseDown={e => e.stopPropagation()}>
        <header className="create-head">
          <div>
            <h2>Create {typeMeta[form.type].label.toLowerCase()}</h2>
            <div className="create-path">
              {path.length ? path.map(p => (
                <span key={p.key}><TypeIcon type={p.type} size={10} /> {p.key}</span>
              )) : <span className="muted">Top level</span>}
              <span className="create-path-new"><TypeIcon type={form.type} size={10} /> new</span>
            </div>
          </div>
          <button className="icon-btn" onClick={closeCreate}><X size={16} /></button>
        </header>

        <div className="create-body scroll">
          <label className="field">
            <span>Issue type</span>
            <div className="type-picker">
              {ALL_TYPES.map(t => (
                <button key={t} type="button" className={cx('type-opt', form.type === t && 'on')} onClick={() => setType(t)}>
                  <TypeIcon type={t} size={12} /> {typeMeta[t].label}
                </button>
              ))}
            </div>
          </label>

          {form.type !== 'initiative' && (
            <label className="field">
              <span>Parent {form.type === 'subtask' && <b className="req">*</b>}
                <em>{parentTypes.map(t => typeMeta[t].label).join(' or ')}</em></span>
              <Combobox items={parentOptions} value={form.parentKey} onChange={v => set('parentKey', v)}
                placeholder={`Choose a ${parentTypes.map(t => typeMeta[t].label.toLowerCase()).join(' / ')}…`} />
            </label>
          )}
          {form.type === 'epic' && !form.parentKey && (
            <label className="field">
              <span>Project</span>
              <select className="input" value={form.project || projects[0] || ''} onChange={e => set('project', e.target.value)}>
                {projects.map(p => <option key={p} value={p}>{p}</option>)}
              </select>
            </label>
          )}

          <label className="field">
            <span>Summary <b className="req">*</b></span>
            <input ref={summary} className="input big" value={form.summary} onChange={e => set('summary', e.target.value)}
              placeholder={placeholderFor(form.type)} />
          </label>

          <label className="field">
            <span>Description</span>
            <textarea className="input" rows={4} value={form.description} onChange={e => set('description', e.target.value)}
              placeholder="Context, acceptance criteria (- [ ] item), notes… Markdown works." />
          </label>

          <div className="field-row">
            <label className="field">
              <span>Assignee</span>
              <div className="assignee-row">
                <Avatar user={users.find(u => u.id === form.assigneeId) ?? null} size={26} />
                <select className="input" value={form.assigneeId} onChange={e => set('assigneeId', e.target.value)}>
                  <option value="">Unassigned</option>
                  {users.map(u => <option key={u.id} value={u.id}>{u.name}{u.id === me?.id ? ' (me)' : ''}</option>)}
                </select>
                {form.assigneeId !== me?.id && <button type="button" className="link-btn small" onClick={() => set('assigneeId', me?.id ?? '')}>Assign to me</button>}
              </div>
            </label>
            <label className="field">
              <span>Priority</span>
              <div className="assignee-row">
                <PriorityIcon priority={form.priority} />
                <select className="input" value={form.priority} onChange={e => set('priority', e.target.value)}>
                  {PRIORITIES.map(p => <option key={p}>{p}</option>)}
                </select>
              </div>
            </label>
          </div>

          <div className="field-row">
            {!isPlanning && form.type !== 'subtask' && (
              <label className="field narrow">
                <span>Story points</span>
                <input className="input" type="number" min={0} max={40} value={form.storyPoints || ''} placeholder="–"
                  onChange={e => set('storyPoints', Number(e.target.value) || 0)} />
              </label>
            )}
            <label className="field">
              <span>Labels</span>
              <input className="input" value={labels} onChange={e => setLabels(e.target.value)} placeholder="backend, sre" />
            </label>
          </div>

          {!isPlanning && form.type !== 'subtask' && (projSprints.length > 0 || projVersions.length > 0) && (
            <div className="field-row">
              {projSprints.length > 0 && (
                <label className="field">
                  <span>Sprint</span>
                  <select className="input" value={form.sprint} onChange={e => set('sprint', e.target.value)}>
                    <option value="">Backlog</option>
                    {projSprints.map(s => <option key={s.id} value={s.name}>{s.name}{s.state === 'future' ? ' (planned)' : ''}</option>)}
                  </select>
                </label>
              )}
              {projVersions.length > 0 && (
                <label className="field">
                  <span>Fix version</span>
                  <select className="input" value={form.fixVersions[0] ?? ''} onChange={e => set('fixVersions', e.target.value ? [e.target.value] : [])}>
                    <option value="">None</option>
                    {projVersions.map(v => <option key={v.id} value={v.name}>{v.name}{v.releaseDate ? ` · ${v.releaseDate}` : ''}</option>)}
                  </select>
                </label>
              )}
              {form.type === 'bug' && (versions[projKey] ?? []).length > 0 && (
                <label className="field">
                  <span>Affects version</span>
                  <select className="input" value={form.affectsVersions[0] ?? ''} onChange={e => set('affectsVersions', e.target.value ? [e.target.value] : [])}>
                    <option value="">None</option>
                    {(versions[projKey] ?? []).map(v => <option key={v.id} value={v.name}>{v.name}{v.released ? ' (released)' : ''}</option>)}
                  </select>
                </label>
              )}
            </div>
          )}
          {form.type === 'subtask' && <div className="muted small">Sub-tasks follow their parent's sprint.</div>}

          {specError && <div className="create-error">{specError}</div>}
          {specs.map(s => (
            <label key={s.id} className="field">
              <span>{s.label} {s.required && <b className="req">*</b>}
                {s.requiredBy === 'team' && <em>required by your team</em>}</span>
              <FieldEditor spec={s} values={custom[s.id] ?? []} autoFocus={false} onSave={v => setCustom(c => ({ ...c, [s.id]: v }))} />
            </label>
          ))}
        </div>

        <footer className="create-foot">
          <label className="another">
            <input type="checkbox" checked={another} onChange={e => setAnother(e.target.checked)} /> Create another
          </label>
          <span className="spacer" />
          <span className="muted small"><Kbd>{modKey}</Kbd> <Kbd>↵</Kbd></span>
          <button className="btn ghost" onClick={closeCreate}>Cancel</button>
          <button className="btn primary" disabled={!valid || busy} onClick={submit}>{busy ? 'Creating…' : 'Create'}</button>
        </footer>
      </div>
    </div>
  )
}

function blank(): NewIssue {
  return { type: 'story', summary: '', description: '', parentKey: '', project: '', priority: 'Medium', assigneeId: '', storyPoints: 0,
    labels: [], sprint: '', fixVersions: [], affectsVersions: [], custom: {} }
}

function placeholderFor(t: IssueType) {
  return {
    initiative: 'e.g. Cut cloud spend 20% in 2027',
    epic: 'e.g. Canary releases',
    story: 'e.g. As an on-call engineer I can see canary health at a glance',
    task: 'e.g. Add canary dashboards to Grafana',
    bug: 'e.g. Rollback leaves stale DNS records',
    subtask: 'e.g. Write integration test',
  }[t]
}
