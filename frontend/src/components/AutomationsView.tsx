import { ArrowRight, Eye, GitBranch, GitMerge, GitPullRequest, RotateCcw, ShieldX, Zap } from 'lucide-react'
import { useStore } from '../store'
import { cx, statusTone } from '../lib/util'
import { Timeline } from './FocusView'

const TRIGGER_ICON: Record<string, typeof GitBranch> = {
  branch_created: GitBranch, pr_opened: GitPullRequest, changes_requested: RotateCcw, review_approved: Eye, pr_merged: GitMerge, ci_failed: ShieldX,
}

const TRIGGER_HINT: Record<string, string> = {
  branch_created: 'git checkout -b feature/KEY-…',
  pr_opened: 'gh pr create',
  changes_requested: 'reviewer requests changes',
  pr_merged: 'PR merged into main',
  ci_failed: 'any required check fails',
}

export function AutomationsView() {
  const { rules, statuses, toggleRule, setRuleStatus, activity } = useStore()
  const log = activity.filter(e => e.kind === 'automation')

  return (
    <div className="automations scroll">
      <div className="view-head">
        <div>
          <h1>Automations</h1>
          <p className="muted">When something happens in code, Maestro moves the ticket in Jira for you. No tab-switching.</p>
        </div>
      </div>

      <div className="rules">
        {rules.map(r => {
          const Icon = TRIGGER_ICON[r.trigger] ?? Zap
          return (
            <div key={r.id} className={cx('rule card', !r.enabled && 'off')}>
              <div className="rule-when">
                <span className="rule-tag">When</span>
                <span className="rule-icon"><Icon size={16} /></span>
                <div>
                  <div className="rule-label">{r.label}</div>
                  <div className="mono muted small">{TRIGGER_HINT[r.trigger]}</div>
                </div>
              </div>
              <ArrowRight className="rule-arrow" size={18} />
              <div className="rule-then">
                <span className="rule-tag">Then move to</span>
                <select className={cx('status-select', `tone-${statusTone(r.toStatus)}`)} value={r.toStatus} onChange={e => setRuleStatus(r.id, e.target.value)}>
                  {statuses.map(s => <option key={s} value={s}>{s}</option>)}
                </select>
              </div>
              <button className={cx('switch', r.enabled && 'on')} onClick={() => toggleRule(r.id)} aria-label="Toggle rule"><i /></button>
            </div>
          )
        })}
      </div>

      <section className="card">
        <div className="card-head"><h2><Zap size={15} /> Automation log</h2></div>
        <Timeline events={log} showKey />
      </section>
    </div>
  )
}
