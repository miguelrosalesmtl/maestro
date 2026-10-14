import { useState } from 'react'
import { ArrowUpRight, Link2, Plus, Copy, ExternalLink, FastForward, SquareTerminal, GitBranch, GitPullRequest, Play, RotateCcw, Sparkles, Terminal, Zap, GitMerge, CircleDot, Check, Loader2, X, GitPullRequestClosed, GitPullRequestDraft } from 'lucide-react'
import { api } from '../api'
import { useStore } from '../store'
import type { FlowEvent, Issue } from '../types'
import { activeStage, ancestors, cx, isWork, linkLabel, statusTone, timeAgo } from '../lib/util'
import { Avatar, PriorityIcon, StatusMenu, StatusPill, TypeIcon } from './bits'
import { FlowBridge } from './FlowBridge'
import { IssueBadges } from './Badges'
import { TicketTermLight, typeInTerminal } from './TerminalDock'
import { useAction } from '../keys'
import { branchName } from '../lib/naming'

export function FocusView() {
  const { issues, flows, focusKey, setFocusKey, me, naming, openJira, startWork, nextEvent, resetFlow, copy, activity, openTerminal, transition, createIssue, statuses, live } = useStore()
  const [newSub, setNewSub] = useState('')

  // ── Shortcuts (registered before any early return) ──
  const cur = issues[focusKey]
  const curFlow = flows[focusKey]
  const moveStatus = (d: number) => {
    if (!cur) return
    const i = statuses.indexOf(cur.status)
    const next = statuses[i + d]
    if (next) transition(cur.key, next)
  }
  const stepQueue = (d: number) => {
    const list = Object.values(issues)
      .filter(i => isWork(i) && i.type !== 'subtask' && i.assignee?.id === me?.id && (i.statusCategory !== 'done' || i.key === focusKey))
      .map(i => i.key).sort((a, b) => a.localeCompare(b, undefined, { numeric: true }))
    if (!list.length) return
    const at = list.indexOf(focusKey)
    setFocusKey(list[(at + d + list.length) % list.length])
  }
  // Live Jira: real work starts in the terminal; typing the command (not
  // simulating it) keeps the user in control of their repository.
  const startLive = (is: Issue) => {
    const session = openTerminal(is.key)
    void typeInTerminal(session, `git checkout -b ${branchName(naming, is)}`)
  }
  useAction('focus.primary', () => {
    if (!cur || !curFlow) return
    if (live) startLive(cur)
    else if (!curFlow.started) startWork(cur.key)
    else if (curFlow.pr?.state !== 'merged') nextEvent(cur.key)
  })
  useAction('focus.statusNext', () => moveStatus(1))
  useAction('focus.statusPrev', () => moveStatus(-1))
  useAction('focus.next', () => stepQueue(1))
  useAction('focus.prev', () => stepQueue(-1))
  useAction('focus.addSubtask', () => document.querySelector<HTMLInputElement>('.subtask-add input')?.focus())
  useAction('focus.copyBranch', () => { if (curFlow?.branch) copy(`git checkout ${curFlow.branch}`) })
  useAction('focus.browser', () => { if (cur) api.openURL(cur.url) })
  const issue = issues[focusKey]
  const flow = flows[focusKey]
  if (!issue || !flow) return <div className="empty">Pick a ticket from the sidebar.</div>

  const chain = ancestors(issue, issues)
  const subtasks = issue.childKeys.map(k => issues[k]).filter(Boolean)
  const next = activeStage(flow)
  const merged = flow.pr?.state === 'merged'
  const events = activity.filter(e => e.issueKey === issue.key)
  const queue = Object.values(issues)
    .filter(i => i.assignee?.id === me?.id && isWork(i) && i.type !== 'subtask' && i.key !== issue.key && i.statusCategory !== 'done')
    .sort((a, b) => (b.statusCategory === 'inprogress' ? 1 : 0) - (a.statusCategory === 'inprogress' ? 1 : 0))

  const acceptance = issue.description.split('\n').map(l => l.match(/^-\s+\[( |x)\]\s+(.*)/)).filter(Boolean) as RegExpMatchArray[]

  return (
    <div className="focus-view scroll">
      {/* ── Hero ─────────────────────────────────────────── */}
      <section className={cx('hero card', `glow-${statusTone(issue.status)}`)}>
        <div className="hero-top">
          <div className="crumbs">
            {chain.map(a => (
              <button key={a.key} className="crumb" onClick={() => openJira(a.key)}>
                <TypeIcon type={a.type} size={11} /> {a.summary}
              </button>
            ))}
          </div>
          <div className="hero-actions">
            <button className="btn ghost" onClick={() => openTerminal(issue.key)}><SquareTerminal size={14} /> Terminal</button>
            <button className="btn ghost" onClick={() => openJira(issue.key)}><Sparkles size={14} /> Jira view <span className="kbd-inline">J</span></button>
            <button className="btn ghost icon" title="Open in browser" onClick={() => api.openURL(issue.url)}><ExternalLink size={14} /></button>
          </div>
        </div>

        <div className="hero-main">
          <div className="hero-title">
            <button className="hero-key" onClick={() => openJira(issue.key)}><TypeIcon type={issue.type} size={14} /> {issue.key}</button>
            <TicketTermLight ticket={issue.key} />
            <IssueBadges issue={issue} place="column" versions />
            <h1>{issue.summary}</h1>
            <div className="hero-meta">
              <StatusMenu issueKey={issue.key} />
              <span className="meta"><PriorityIcon priority={issue.priority} /> {issue.priority}</span>
              {issue.storyPoints > 0 && <span className="meta"><b>{issue.storyPoints}</b> pts</span>}
              {issue.sprint && <span className="meta">{issue.sprint}</span>}
              {issue.labels.map(l => <span key={l} className="label">{l}</span>)}
              <span className="meta"><Avatar user={issue.assignee} size={18} /> {issue.assignee?.name ?? 'Unassigned'}</span>
            </div>
            <div className="hero-links">
              {issue.links.map(l => issues[l.key] && (
                <button key={l.type + l.key} className={cx('hero-link', l.type === 'is blocked by' && issues[l.key].statusCategory !== 'done' && 'blocking')}
                  title={issues[l.key].summary} onClick={() => openJira(l.key)}>
                  <span className="hl-type">{linkLabel(l.type)}</span>
                  <TypeIcon type={issues[l.key].type} size={10} /> <span className="mono">{l.key}</span>
                  <span className={cx('dot', `tone-${statusTone(issues[l.key].status)}`)} />
                </button>
              ))}
              <button className="hero-link add" onClick={() => openJira(issue.key)}><Link2 size={11} /> link issue</button>
            </div>
          </div>

          <div className="hero-cta">
            {live ? (
              <button className="btn primary big" onClick={() => startLive(issue)}>
                {flow.started ? <SquareTerminal size={16} /> : <Play size={16} fill="currentColor" />} {flow.started ? 'Continue in terminal' : 'Start work'}
                <small>{flow.started ? (flow.live ? 'pull requests on GitHub move the ticket' : 'git push · gh pr create move the ticket') : 'types git checkout -b · Enter moves it to In Progress'}</small>
              </button>
            ) : !flow.started ? (
              <button className="btn primary big" onClick={() => startWork(issue.key)}>
                <Play size={16} fill="currentColor" /> Start work
                <small>creates branch · moves to In Progress</small>
              </button>
            ) : merged ? (
              <div className="shipped"><GitMerge size={18} /> Shipped <small>{timeAgo(flow.stages[5].at)}</small></div>
            ) : (
              <button className="btn primary big" onClick={() => nextEvent(issue.key)}>
                <FastForward size={16} /> Simulate: {nextLabel(next?.id)}
                <small>demo — real git events later</small>
              </button>
            )}
            {flow.started && !live && <button className="btn ghost small" onClick={() => resetFlow(issue.key)}><RotateCcw size={12} /> reset demo</button>}
          </div>
        </div>
      </section>

      {/* ── The bridge ─────────────────────────────────────── */}
      <section className="card bridge-card">
        <div className="card-head">
          <h2><Zap size={15} /> Ticket ⇄ Code</h2>
          <span className="muted">Dev events move the ticket automatically. Change it by hand any time.</span>
        </div>
        <FlowBridge issue={issue} flow={flow} />
      </section>

      <div className="focus-grid">
        {/* ── Code ───────────────────────────────────────── */}
        <section className="card">
          <div className="card-head"><h2><Terminal size={15} /> Code</h2><span className="muted mono">{flow.repo}</span></div>
          {flow.started ? (
            <>
              <div className="branch-row">
                <GitBranch size={14} />
                <span className="mono branch-name">{flow.branch}</span>
                <button className="btn ghost icon" title="Copy checkout command" onClick={() => copy(`git checkout ${flow.branch}`, `git checkout ${flow.branch}`)}><Copy size={13} /></button>
              </div>

              {flow.pr && (
                <div className={cx('pr-card', flow.pr.state, flow.pr.draft && flow.pr.state === 'open' && 'draft')} onClick={() => api.openURL(flow.pr!.url)}>
                  <div className="pr-top">
                    {flow.pr.state === 'merged' ? <GitMerge size={16} /> : flow.pr.state === 'closed' ? <GitPullRequestClosed size={16} /> : flow.pr.draft ? <GitPullRequestDraft size={16} /> : <GitPullRequest size={16} />}
                    <span className="pr-title">{flow.pr.title}</span>
                    <span className="pr-num">#{flow.pr.number}</span>
                    <ArrowUpRight size={14} className="muted" />
                  </div>
                  <div className="pr-bottom">
                    <span className={cx('pr-state', flow.pr.draft && flow.pr.state === 'open' ? 'draft' : flow.pr.state)}>{flow.pr.draft && flow.pr.state === 'open' ? 'draft' : flow.pr.state}</span>
                    {flow.pr.checks.map(c => (
                      <span key={c.name} className={cx('check', `state-${c.state}`, c.required && 'required', c.url && 'has-url')}
                        title={`${c.name}${c.required ? ' (required)' : ''}`}
                        onClick={c.url ? e => { e.stopPropagation(); void api.openURL(c.url!) } : undefined}>
                        {c.state === 'done' ? <Check size={11} strokeWidth={3} /> : c.state === 'failed' ? <X size={11} strokeWidth={3} /> : <Loader2 size={11} className="spin" />} {c.name}
                      </span>
                    ))}
                    {flow.pr.live && !flow.pr.checks.length && <span className="check">no checks</span>}
                    <span className="approvals" title={[
                      flow.pr.approvedBy?.length ? `Approved by ${flow.pr.approvedBy.join(', ')}` : '',
                      flow.pr.reviewers.length ? `Reviewers: ${flow.pr.reviewers.join(', ')}` : '',
                    ].filter(Boolean).join('\n')}>
                      {flow.pr.changesRequested?.length
                        ? <b className="changes">changes requested · {flow.pr.changesRequested.join(', ')}</b>
                        : <>{flow.pr.approvals}/{flow.pr.requiredApprovals || flow.pr.reviewers.length} approvals</>}
                    </span>
                  </div>
                  {flow.pr.live && (
                    <div className="pr-meta">
                      <span className="mono">{flow.pr.repo}</span>
                      {flow.pr.head && <span className="mono">{flow.pr.head} → {flow.pr.base}</span>}
                      {flow.pr.author && <span>by @{flow.pr.author}</span>}
                      {flow.pr.updatedAt && <span>updated {timeAgo(flow.pr.mergedAt || flow.pr.updatedAt)}</span>}
                    </div>
                  )}
                </div>
              )}

              <div className="commits">
                {[...flow.commits].reverse().map(c => (
                  <div key={c.sha} className="commit">
                    <CircleDot size={13} className="commit-dot" />
                    <span className="mono sha">{c.sha}</span>
                    <span className="commit-msg">{c.message}</span>
                    {(c.adds > 0 || c.dels > 0) && <span className="diff"><b className="add">+{c.adds}</b> <b className="del">−{c.dels}</b></span>}
                    <span className="muted small">{timeAgo(c.at)}</span>
                  </div>
                ))}
                {!flow.commits.length && <div className="muted small pad">No commits yet — go write some code ✨</div>}
              </div>
            </>
          ) : (
            <div className="code-empty">
              <GitBranch size={28} />
              <p>Starting work will create</p>
              <code>{branchName(naming, issue)}</code>
            </div>
          )}

          {issue.type !== 'subtask' && (
            <>
              <div className="sub-head">Sub-tasks {subtasks.length > 0 && <span>{subtasks.filter(t => t.statusCategory === 'done').length}/{subtasks.length}</span>}</div>
              <div className="subtasks">
                {subtasks.map(t => (
                  <div key={t.key} className={cx('subtask', t.statusCategory === 'done' && 'done')}>
                    <button className="checkbox" title={t.statusCategory === 'done' ? 'Reopen' : 'Mark done'}
                      onClick={() => transition(t.key, t.statusCategory === 'done' ? 'todo' : 'done')} />
                    <button className="mono key" onClick={() => setFocusKey(t.key)}>{t.key}</button>
                    <span className="subtask-sum" onClick={() => openJira(t.key)}>{t.summary}</span>
                    <StatusPill status={t.status} small />
                    <Avatar user={t.assignee} size={18} />
                  </div>
                ))}
                <form className="subtask-add" onSubmit={async e => {
                  e.preventDefault()
                  if (!newSub.trim()) return
                  if (await createIssue({ type: 'subtask', parentKey: issue.key, summary: newSub.trim(), description: '', project: '', priority: 'Medium', assigneeId: me?.id ?? '', storyPoints: 0, labels: [], sprint: '', fixVersions: [], affectsVersions: [], custom: {} })) setNewSub('')
                }}>
                  <Plus size={13} />
                  <input value={newSub} onChange={e => setNewSub(e.target.value)} placeholder="Add a sub-task and press Enter…" />
                </form>
              </div>
            </>
          )}

          {acceptance.length > 0 && (
            <>
              <div className="sub-head">Acceptance criteria</div>
              <ul className="ac">
                {acceptance.map((m, i) => (
                  <li key={i} className={cx(m[1] === 'x' && 'done')}><span className="checkbox" />{m[2].replace(/`/g, '')}</li>
                ))}
              </ul>
            </>
          )}
        </section>

        {/* ── Timeline ───────────────────────────────────── */}
        <section className="card">
          <div className="card-head"><h2><Sparkles size={15} /> Timeline</h2></div>
          <Timeline events={events} />

          <div className="sub-head">Up next for you</div>
          <div className="queue">
            {queue.slice(0, 5).map(q => (
              <button key={q.key} data-issue={q.key} className="queue-row" onClick={() => setFocusKey(q.key)}>
                <TypeIcon type={q.type} size={12} />
                <span className="mono key">{q.key}</span>
                <span className="queue-sum">{q.summary}</span>
                <span className={cx('dot', `tone-${statusTone(q.status)}`)} />
              </button>
            ))}
          </div>
        </section>
      </div>
    </div>
  )
}

export function Timeline({ events, showKey }: { events: FlowEvent[]; showKey?: boolean }) {
  const { setFocusKey, setView } = useStore()
  if (!events.length) return <div className="muted small pad">Nothing yet.</div>
  return (
    <ol className="timeline">
      {events.map(e => (
        <li key={e.id} className={cx('tl', `tl-${e.kind}`)}>
          <span className="tl-dot">{e.kind === 'automation' ? <Zap size={11} /> : e.kind === 'term' ? <SquareTerminal size={11} /> : e.kind === 'dev' ? <GitBranch size={11} /> : <CircleDot size={11} />}</span>
          <div className="tl-body">
            <div className="tl-title">
              {showKey && <button className="mono key link-btn" onClick={() => { setFocusKey(e.issueKey); setView('focus') }}>{e.issueKey}</button>}
              {e.title}
            </div>
            <div className="tl-detail">{e.detail}</div>
          </div>
          <span className="tl-time">{timeAgo(e.at)}</span>
        </li>
      ))}
    </ol>
  )
}

function nextLabel(stage?: string) {
  return ({ commits: 'push commit', pr: 'open PR', ci: 'checks pass', review: 'approve PR', merged: 'merge PR' } as Record<string, string>)[stage ?? ''] ?? 'next event'
}

