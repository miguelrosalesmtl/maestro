import { Check, Eye, GitBranch, GitCommitHorizontal, GitMerge, GitPullRequest, Loader2, RotateCcw, ShieldCheck, Zap } from 'lucide-react'
import { useStore } from '../store'
import type { Flow, Issue } from '../types'
import { cx, stateOf, statusTone } from '../lib/util'

// Horizontal positions (in %) of each node. Jira statuses sit directly above
// the dev stage that moves the ticket into them, so cause → effect reads vertically.
const DEV_X: Record<string, number> = { branch: 22, commits: 36.4, pr: 50.8, ci: 65.2, review: 79.6, merged: 94 }
// One node per Maestro state; the label is the Jira status name for that state.
const STATE_X: Record<string, number> = { todo: 6, inProgress: 22, inReview: 50.8, done: 94 }
const DEFAULT_LABEL: Record<string, string> = { todo: 'To Do', inProgress: 'In Progress', inReview: 'In Review', done: 'Done' }
const JIRA_Y = 26
const DEV_Y = 72

const TRIGGER_STAGE: Record<string, string> = {
  branch_created: 'branch', pr_opened: 'pr', ci_failed: 'ci', review_approved: 'review', changes_requested: 'review', pr_merged: 'merged',
}
// Exception paths that point backwards; listed as chips instead of drawn.
const SIDE_TRIGGERS = new Set(['changes_requested', 'ci_failed'])

const STAGE_ICON: Record<string, typeof GitBranch> = {
  branch: GitBranch, commits: GitCommitHorizontal, pr: GitPullRequest, ci: ShieldCheck, review: Eye, merged: GitMerge,
}

export function FlowBridge({ issue, flow }: { issue: Issue; flow: Flow }) {
  const { rules, statuses } = useStore()
  const issueState = issue.state || stateOf(issue.status)
  const statusX = STATE_X[issueState]
  const blocked = issueState === 'blocked'
  const label = (st: string) => issueState === st ? issue.status : statuses.find(s => stateOf(s) === st) ?? DEFAULT_LABEL[st]
  const targetX = (to: string) => STATE_X[stateOf(to)]
  const doneStages = flow.stages.filter(s => s.state === 'done')
  const devFillTo = doneStages.length ? DEV_X[doneStages[doneStages.length - 1].id] : 22

  const links = rules
    .filter(r => !SIDE_TRIGGERS.has(r.trigger) && targetX(r.toStatus) !== undefined && TRIGGER_STAGE[r.trigger])
    .map(r => {
      const stage = flow.stages.find(s => s.id === TRIGGER_STAGE[r.trigger])!
      const state = !r.enabled ? 'off' : stage.state === 'done' ? 'fired' : stage.state === 'active' ? 'armed' : 'idle'
      return { rule: r, sx: DEV_X[stage.id], tx: targetX(r.toStatus), state }
    })
  const side = rules.filter(r => SIDE_TRIGGERS.has(r.trigger))

  return (
    <div className="bridge">
      <div className="lane-label jira-lane" style={{ top: `${JIRA_Y}%` }}>Jira</div>
      <div className="lane-label code-lane" style={{ top: `${DEV_Y}%` }}>Code</div>

      <div className="bridge-canvas">
        <svg className="bridge-svg" viewBox="0 0 100 100" preserveAspectRatio="none">
          <defs>
            <linearGradient id="jiraFill" x1="0" x2="1"><stop offset="0" stopColor="var(--accent-2)" /><stop offset="1" stopColor="var(--accent)" /></linearGradient>
            <linearGradient id="devFill" x1="0" x2="1"><stop offset="0" stopColor="var(--accent)" /><stop offset="1" stopColor="var(--green)" /></linearGradient>
          </defs>
          <line x1={6} x2={94} y1={JIRA_Y} y2={JIRA_Y} className="track" />
          {statusX !== undefined && <line x1={6} x2={statusX} y1={JIRA_Y} y2={JIRA_Y} className="track-fill" stroke="url(#jiraFill)" />}
          <line x1={22} x2={94} y1={DEV_Y} y2={DEV_Y} className="track" />
          {doneStages.length > 0 && <line x1={22} x2={devFillTo} y1={DEV_Y} y2={DEV_Y} className="track-fill" stroke="url(#devFill)" />}
          {links.map(l => (
            <path key={l.rule.id} className={cx('link', `link-${l.state}`)}
              d={`M ${l.sx} ${DEV_Y - 8} C ${l.sx} ${DEV_Y - 26}, ${l.tx} ${JIRA_Y + 26}, ${l.tx} ${JIRA_Y + 8}`} />
          ))}
        </svg>

        {links.map(l => (
          <div key={l.rule.id} className={cx('link-chip', `link-${l.state}`)} style={{ left: `${(l.sx + l.tx) / 2}%`, top: '49%' }}
            title={`When “${l.rule.label}” → move to ${l.rule.toStatus}`}>
            <Zap size={10} /> {l.state === 'off' ? 'off' : 'auto'}
          </div>
        ))}

        {Object.entries(STATE_X).map(([st, x]) => {
          const current = st === issueState
          const passed = statusX !== undefined && x < statusX
          return (
            <div key={st} className={cx('jira-node', `tone-${statusTone(st)}`, current && 'current', passed && 'passed')}
              style={{ left: `${x}%`, top: `${JIRA_Y}%` }}>
              {passed && <Check size={11} strokeWidth={3} />}{label(st)}
            </div>
          )
        })}
        {blocked && <div className="jira-node tone-blocked current blocked-float" style={{ left: '50%', top: '6%' }}>{issue.status}</div>}

        {flow.stages.map(s => {
          const Icon = STAGE_ICON[s.id]
          return (
            <div key={s.id} className={cx('dev-node', `state-${s.state}`)} style={{ left: `${DEV_X[s.id]}%`, top: `${DEV_Y}%` }}>
              <div className="dev-orb">
                {s.state === 'active' ? <Loader2 size={15} className="spin-slow" /> : <Icon size={15} />}
              </div>
              <div className="dev-label">{s.label}</div>
              <div className="dev-detail">{s.detail || (s.state === 'active' ? 'up next' : '')}</div>
            </div>
          )
        })}

        {!flow.started && (
          <div className="dev-idle" style={{ left: '6%', top: `${DEV_Y}%` }}>
            <span>no branch yet</span>
          </div>
        )}
      </div>

      {side.length > 0 && (
        <div className="side-rules">
          {side.map(r => (
            <span key={r.id} className={cx('side-rule', !r.enabled && 'off')}>
              <RotateCcw size={11} /> {r.label} → <b className={`tone-text-${statusTone(r.toStatus)}`}>{r.toStatus}</b>{!r.enabled && ' (off)'}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
