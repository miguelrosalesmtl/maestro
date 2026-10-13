import { Fragment, useEffect, useRef, useState } from 'react'
import { ChevronDown, ChevronRight, ExternalLink, Eye, GitBranch, GitCommitHorizontal, GitPullRequest, Link2, MoreHorizontal, Paperclip, Plus, Share2, Lock, Flag, ThumbsUp, X, Crosshair } from 'lucide-react'
import { api } from '../api'
import { useStore } from '../store'
import { ancestors, cx, formatDate, isWork, statusTone, timeAgo, typeMeta } from '../lib/util'
import { Combobox } from './Combobox'
import { EditableField, ValueView, fieldValueOf } from './Fields'
import type { FieldSpec } from '../types'
import { dueClass, formatBytes, formatDay, timeSince } from '../lib/util'
import { useAction, useKeys } from '../keys'
import { Combo } from './Shortcuts'
import { Avatar, Markdown, PriorityIcon, StatusMenu, StatusPill, TypeIcon } from './bits'

/** Renders a ticket the way Jira does (dark theme), so you never need to leave the app. */
export function JiraIssueView() {
  const { jiraKey, openJira, issues, flows, activity, comment, me, setFocusKey, setView, openCreate, childTypes, linkTypes, link, unlink, refreshIssue, updateFields, timeTracking } = useStore()
  const [linking, setLinking] = useState(false)
  const [linkType, setLinkType] = useState('blocks')
  const [linkTo, setLinkTo] = useState('')
  const [draft, setDraft] = useState('')
  const [tab, setTab] = useState<'comments' | 'history'>('comments')
  const [detailsOpen, setDetailsOpen] = useState(true)
  const editor = useRef<HTMLTextAreaElement>(null)

  useEffect(() => { setDraft(''); setLinking(false); setLinkTo('') }, [jiraKey])
  // Read the issue fresh (with comments) whenever the Jira view opens.
  useEffect(() => { if (jiraKey) void refreshIssue(jiraKey) }, [jiraKey]) // eslint-disable-line react-hooks/exhaustive-deps
  const { bindings } = useKeys()
  // Editable fields of this issue, from Jira's edit screen.
  const [specs, setSpecs] = useState<Record<string, FieldSpec>>({})
  useEffect(() => {
    setSpecs({})
    if (!jiraKey) return
    let alive = true
    api.fieldSpecs(jiraKey).then(list => { if (alive) setSpecs(Object.fromEntries(list.map(s => [s.id, s]))) }).catch(() => {})
    return () => { alive = false }
  }, [jiraKey])
  const cur = jiraKey ? issues[jiraKey] : undefined
  const on = !!cur
  useAction('jira.close', () => openJira(null), on)
  useAction('jira.comment', () => { setTab('comments'); setTimeout(() => editor.current?.focus()) }, on)
  useAction('jira.link', () => setLinking(true), on)
  useAction('jira.addChild', () => { if (cur && childTypes[cur.type]?.length) openCreate({ parentKey: cur.key }) }, on)
  useAction('jira.parent', () => { if (cur?.parentKey) openJira(cur.parentKey) }, on)
  useAction('jira.browser', () => { if (cur) api.openURL(cur.url) }, on)
  useAction('jira.focusTicket', () => { if (cur && isWork(cur)) { setFocusKey(cur.key); setView('focus'); openJira(null) } }, on)

  if (!jiraKey) return null
  const issue = issues[jiraKey]
  if (!issue) return null
  const flow = flows[jiraKey]
  const chain = ancestors(issue, issues)
  const children = issue.childKeys.map(k => issues[k]).filter(Boolean)
  const doneKids = children.filter(c => c.statusCategory === 'done').length
  const history = activity.filter(e => e.issueKey === issue.key)

  const submit = () => {
    if (!draft.trim()) return
    comment(issue.key, draft.trim())
    setDraft('')
  }

  return (
    <div className="jira-backdrop fade-in" onMouseDown={() => openJira(null)}>
      <div className="jira pop-in" onMouseDown={e => e.stopPropagation()}>
        {/* Top bar */}
        <header className="jira-top">
          <nav className="jira-crumbs">
            {chain.map(a => (
              <span key={a.key} className="jc">
                <button onClick={() => openJira(a.key)}><TypeIcon type={a.type} size={11} /> {a.key}</button>
                <span className="sep">/</span>
              </span>
            ))}
            <span className="jc current"><TypeIcon type={issue.type} size={11} /> {issue.key}</span>
          </nav>
          <div className="jira-top-actions">
            <button className="jbtn subtle" title="Watch"><Eye size={15} /> 3</button>
            <button className="jbtn subtle" title="Like"><ThumbsUp size={15} /></button>
            <button className="jbtn subtle" title="Share"><Share2 size={15} /></button>
            <button className="jbtn subtle" title="Open in Jira" onClick={() => api.openURL(issue.url)}><ExternalLink size={15} /></button>
            <button className="jbtn subtle" title="More"><MoreHorizontal size={15} /></button>
            <button className="jbtn subtle" title="Close (Esc)" onClick={() => openJira(null)}><X size={17} /></button>
          </div>
        </header>

        <div className="jira-body">
          {/* Main column */}
          <main className="jira-main scroll">
            <h1 className="jira-title">
              {issue.security && <span className="sec-lock" title={`Security level: ${issue.security}`}><Lock size={16} /> {issue.security}</span>}
              {issue.summary}
            </h1>
            <div className="jira-toolbar">
              <button className="jbtn"><Paperclip size={14} /> Attach</button>
              {(childTypes[issue.type] ?? []).length > 0 && (
                <button className="jbtn" onClick={() => openCreate({ parentKey: issue.key })}>
                  <Plus size={14} /> {issue.type === 'initiative' ? 'Add epic' : issue.type === 'epic' ? 'Add child issue' : 'Create subtask'}
                </button>
              )}
              <button className="jbtn" onClick={() => setLinking(l => !l)}><Link2 size={14} /> Link issue</button>
              {isWork(issue) && (
                <button className="jbtn" onClick={() => { setFocusKey(issue.key); setView('focus'); openJira(null) }}><Crosshair size={14} /> Focus in Maestro</button>
              )}
            </div>

            <h3 className="jira-h">Description</h3>
            <div className="jira-desc"><Markdown text={issue.description || '_Add a description…_'} /></div>

            {issue.environment && (<>
              <h3 className="jira-h">Environment</h3>
              <div className="jira-desc"><Markdown text={issue.environment} /></div>
            </>)}

            {issue.custom.some(c => c.show.includes('view')) && (<>
              <h3 className="jira-h">Fields</h3>
              <dl className="jira-fields jira-custom">
                {issue.custom.filter(c => c.show.includes('view')).map(c => (
                  <Fragment key={c.id}>
                    <dt>{c.label}</dt>
                    <dd className="wrap">
                      <EditableField spec={c.editable ? specs[c.id] : undefined} values={c.items?.length ? c.items : c.text ? [c.text] : []}
                        view={<ValueView {...fieldValueOf(c)} />}
                        onSave={v => updateFields(issue.key, { custom: { [c.id]: v } })} />
                    </dd>
                  </Fragment>
                ))}
              </dl>
            </>)}

            {issue.attachments.length > 0 && (<>
              <h3 className="jira-h">Attachments <span className="jira-muted">{issue.attachments.length}</span></h3>
              <div className="jira-attachments">
                {issue.attachments.map(a => (
                  <button key={a.id} className="jira-attachment" title="Open the issue in Jira to download" onClick={() => api.openURL(issue.url)}>
                    <Paperclip size={14} /> <span className="att-name">{a.name}</span> <span className="jira-muted">{formatBytes(a.size)}</span>
                  </button>
                ))}
              </div>
            </>)}

            {children.length > 0 && (
              <>
                <h3 className="jira-h">
                  {issue.type === 'initiative' ? 'Epics' : issue.type === 'epic' ? 'Child issues' : 'Subtasks'}
                  <span className="jira-progress"><i style={{ width: `${(doneKids / children.length) * 100}%` }} /></span>
                  <span className="jira-muted">{Math.round((doneKids / children.length) * 100)}% done</span>
                </h3>
                <div className="jira-children">
                  {children.map(c => (
                    <button key={c.key} data-issue={c.key} className="jira-child" onClick={() => openJira(c.key)}>
                      <TypeIcon type={c.type} size={12} />
                      <span className="jira-link">{c.key}</span>
                      <span className="jira-child-sum">{c.summary}</span>
                      <PriorityIcon priority={c.priority} />
                      <Avatar user={c.assignee} size={20} />
                      <span className={cx('jira-lozenge', `lz-${c.statusCategory}`)}>{c.status}</span>
                    </button>
                  ))}
                </div>
              </>
            )}

            {(issue.links.length > 0 || linking) && (
              <>
                <h3 className="jira-h">Linked issues
                  <button className="jbtn subtle small-btn" onClick={() => setLinking(true)}><Plus size={14} /></button>
                </h3>
                {linking && (
                  <div className="jira-link-form">
                    <select className="jira-select" value={linkType} onChange={e => setLinkType(e.target.value)}>
                      {Object.keys(linkTypes).map(t => <option key={t} value={t}>{t}</option>)}
                    </select>
                    <Combobox className="jira-combo" value={linkTo} onChange={setLinkTo} placeholder="Search for issues"
                      items={Object.values(issues)
                        .filter(i => i.key !== issue.key && !issue.links.some(l => l.key === i.key && l.type === linkType))
                        .map(i => ({ value: i.key, text: `${i.key} ${i.summary}`,
                          label: <span className="opt"><TypeIcon type={i.type} size={11} /><span className="mono key">{i.key}</span><span className="opt-sum">{i.summary}</span></span> }))} />
                    <button className="jbtn primary" disabled={!linkTo} onClick={() => { link(issue.key, linkType, linkTo); setLinkTo(''); setLinking(false) }}>Link</button>
                    <button className="jbtn subtle" onClick={() => setLinking(false)}>Cancel</button>
                  </div>
                )}
                {Object.entries(groupLinks(issue.links)).map(([type, keys]) => (
                  <div key={type} className="jira-link-group">
                    <div className="jira-link-type">{type}</div>
                    <div className="jira-children">
                      {keys.map(k => issues[k]).filter(Boolean).map(c => (
                        <div key={c.key} data-issue={c.key} className="jira-child" onClick={() => openJira(c.key)}>
                          <TypeIcon type={c.type} size={12} />
                          <span className="jira-link">{c.key}</span>
                          <span className="jira-child-sum">{c.summary}</span>
                          <PriorityIcon priority={c.priority} />
                          <Avatar user={c.assignee} size={20} />
                          <span className={cx('jira-lozenge', `lz-${c.statusCategory}`)}>{c.status}</span>
                          <button className="jira-unlink" title="Remove link" onClick={e => { e.stopPropagation(); unlink(issue.key, type, c.key) }}><X size={13} /></button>
                        </div>
                      ))}
                    </div>
                  </div>
                ))}
              </>
            )}

            <h3 className="jira-h">Activity</h3>
            <div className="jira-tabs">
              <span className="jira-muted">Show:</span>
              <button className={cx(tab === 'comments' && 'on')} onClick={() => setTab('comments')}>Comments</button>
              <button className={cx(tab === 'history' && 'on')} onClick={() => setTab('history')}>History</button>
            </div>

            {tab === 'comments' ? (
              <>
                <div className="jira-comment-box">
                  <Avatar user={me} size={32} />
                  <div className="jira-editor">
                    <textarea ref={editor} value={draft} onChange={e => setDraft(e.target.value)} placeholder="Add a comment…"
                      onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submit() }} />
                    {draft && (
                      <div className="jira-editor-actions">
                        <button className="jbtn primary" onClick={submit}>Save</button>
                        <button className="jbtn subtle" onClick={() => setDraft('')}>Cancel</button>
                      </div>
                    )}
                    {!draft && bindings['jira.comment'] && <div className="jira-tip"><b>Pro tip:</b> press <Combo combo={bindings['jira.comment']} /> to comment</div>}
                  </div>
                </div>
                {[...issue.comments].reverse().map(c => (
                  <div key={c.id} className="jira-comment">
                    <Avatar user={c.author} size={32} />
                    <div>
                      <div className="jira-comment-head"><b>{c.author.name}</b> <span className="jira-muted">{timeAgo(c.created)}</span></div>
                      <div className="jira-comment-body"><Markdown text={c.body} /></div>
                      <div className="jira-comment-actions"><button>Edit</button>·<button>Delete</button>·<button>👍</button></div>
                    </div>
                  </div>
                ))}
              </>
            ) : (
              <div className="jira-history">
                {history.length ? history.map(h => (
                  <div key={h.id} className="jira-hist-row">
                    <span className={cx('jira-hist-kind', h.kind)}>{h.kind === 'automation' ? 'Maestro automation' : h.kind === 'dev' ? 'Development' : 'Status'}</span>
                    <span>{h.title}</span>
                    <span className="jira-muted">{h.detail}</span>
                    <span className="jira-muted right">{timeAgo(h.at)}</span>
                  </div>
                )) : <div className="jira-muted">No history yet.</div>}
              </div>
            )}
          </main>

          {/* Side column */}
          <aside className="jira-side scroll">
            <div className="jira-side-top">
              <StatusMenu issueKey={issue.key} variant="jira" />
              <button className="jbtn"><span className="bolt">⚡</span> Actions <ChevronDown size={13} /></button>
            </div>

            <div className="jira-panel">
              <button className="jira-panel-head" onClick={() => setDetailsOpen(o => !o)}>
                Details {detailsOpen ? <ChevronDown size={15} /> : <ChevronRight size={15} />}
              </button>
              {detailsOpen && (
                <dl className="jira-fields">
                  <dt>Assignee</dt>
                  <dd><Avatar user={issue.assignee} size={22} /> {issue.assignee?.name ?? 'Unassigned'}</dd>
                  <dt>Reporter</dt>
                  <dd><Avatar user={issue.reporter} size={22} /> {issue.reporter.name}</dd>
                  <dt>Development</dt>
                  <dd className="jira-dev">
                    {flow?.started ? (
                      <>
                        <span><GitBranch size={13} /> <a onClick={() => api.openURL(`https://github.com/${flow.repo}/tree/${flow.branch}`)}>1 branch</a></span>
                        <span><GitCommitHorizontal size={13} /> <a>{flow.commits.length} commits</a> <em>{flow.commits.at(-1) && timeAgo(flow.commits.at(-1)!.at)}</em></span>
                        {flow.pr && <span><GitPullRequest size={13} /> <a onClick={() => api.openURL(flow.pr!.url)}>1 pull request</a> <i className={cx('jira-lozenge', flow.pr.state === 'merged' ? 'lz-done' : 'lz-inprogress')}>{flow.pr.state.toUpperCase()}</i></span>}
                      </>
                    ) : <span className="jira-muted">Create branch</span>}
                  </dd>
                  <dt>Labels</dt>
                  <dd className="wrap">{issue.labels.length ? issue.labels.map(l => <span key={l} className="jira-label">{l}</span>) : <span className="jira-muted">None</span>}</dd>
                  {issue.parentKey && (<>
                    <dt>Parent</dt>
                    <dd><button className="jira-parent" onClick={() => openJira(issue.parentKey)}><TypeIcon type={issues[issue.parentKey]?.type ?? 'epic'} size={11} /> {issue.parentKey} {issues[issue.parentKey]?.summary}</button></dd>
                  </>)}
                  {issue.sprint && (<><dt>Sprint</dt><dd><a>{issue.sprint}</a></dd></>)}
                  {issue.storyPoints > 0 && (<><dt>Story point estimate</dt><dd><span className="jira-points">{issue.storyPoints}</span></dd></>)}
                  <dt>Priority</dt>
                  <dd><PriorityIcon priority={issue.priority} /> {issue.priority}</dd>
                  {issue.components.length > 0 && (<><dt>Components</dt><dd>{issue.components.join(', ')}</dd></>)}
                  <dt>Issue type</dt>
                  <dd><TypeIcon type={issue.type} size={12} /> {typeMeta[issue.type].label}</dd>
                  <dt>Fix versions</dt>
                  <dd className="wrap">
                    <EditableField spec={specs.fixVersions} values={issue.fixVersions} view={<ValueView kind="versions" items={issue.fixVersions} />}
                      onSave={v => updateFields(issue.key, { fixVersions: v })} />
                  </dd>
                  {(issue.type === 'bug' || issue.affectsVersions.length > 0) && (<>
                    <dt>Affects versions</dt>
                    <dd className="wrap">
                      <EditableField spec={specs.versions} values={issue.affectsVersions} view={<ValueView kind="versions" items={issue.affectsVersions} />}
                        onSave={v => updateFields(issue.key, { affectsVersions: v })} />
                    </dd>
                  </>)}
                  <dt>Due date</dt>
                  <dd>
                    <EditableField spec={specs.duedate} values={issue.dueDate ? [issue.dueDate] : []}
                      view={issue.dueDate ? <span className={cx(dueClass(issue))}>{formatDay(issue.dueDate)}</span> : <span className="jira-muted">None</span>}
                      onSave={v => updateFields(issue.key, { dueDate: v[0] ?? '' })} />
                  </dd>
                  {issue.startDate && (<><dt>Start date</dt><dd>{formatDay(issue.startDate)}</dd></>)}
                  {issue.flagged && (<><dt>Flagged</dt><dd><span className="flag-chip"><Flag size={12} /> Impediment</span></dd></>)}
                  {issue.carriedOver > 0 && (<><dt>Carried over</dt><dd>{issue.carriedOver} sprint{issue.carriedOver > 1 ? 's' : ''}</dd></>)}
                  {issue.resolution && (<><dt>Resolution</dt><dd>{issue.resolution}{issue.resolvedAt && <span className="jira-muted"> · {timeAgo(issue.resolvedAt)}</span>}</dd></>)}
                  {timeTracking && issue.timeTracking && (<>
                    <dt>Time tracking</dt>
                    <dd className="jira-tt">
                      <span>{issue.timeTracking.spent ?? '0m'} logged</span>
                      <span className="jira-muted">{issue.timeTracking.remaining ?? '—'} remaining · {issue.timeTracking.original ?? '—'} estimated</span>
                    </dd>
                  </>)}
                  {issue.security && (<><dt>Security level</dt><dd><Lock size={12} /> {issue.security}</dd></>)}
                  {issue.creator && issue.creator.id !== issue.reporter.id && (<><dt>Creator</dt><dd><Avatar user={issue.creator} size={22} /> {issue.creator.name}</dd></>)}
                </dl>
              )}
            </div>

            <div className="jira-panel maestro-panel">
              <div className="jira-panel-head static">Maestro</div>
              <div className="jira-maestro">
                <span>Ticket status</span> <StatusPill status={issue.status} small />
                {flow?.started && <><span>Dev flow</span><span className={`tone-text-${statusTone(issue.status)}`}>{flow.stages.filter(s => s.state === 'done').length}/{flow.stages.length} stages</span></>}
              </div>
            </div>

            <div className="jira-dates">
              <div>Created {formatDate(issue.created)}</div>
              <div>Updated {timeAgo(issue.updated)}</div>
              {issue.statusChangedAt && <div>In {issue.status} for {timeSince(issue.statusChangedAt)}</div>}
              {(issue.watchers > 0 || issue.votes > 0) && <div><Eye size={11} /> {issue.watchers} watching{issue.watching ? ' (you)' : ''} · {issue.votes} vote{issue.votes === 1 ? '' : 's'}</div>}
            </div>
          </aside>
        </div>
      </div>
    </div>
  )
}

function groupLinks(links: { type: string; key: string }[]) {
  const out: Record<string, string[]> = {}
  for (const l of links) (out[l.type] ??= []).push(l.key)
  return out
}
