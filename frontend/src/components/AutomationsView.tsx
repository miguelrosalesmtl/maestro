import { useState } from 'react'
import { ArrowRight, Eye, Filter, GitBranch, GitMerge, GitPullRequest, Plus, RotateCcw, ShieldX, Trash2, Zap } from 'lucide-react'
import { useStore } from '../store'
import type { Rule } from '../types'
import { cx, statusTone } from '../lib/util'
import { Timeline } from './FocusView'

const TRIGGER_ICON: Record<string, typeof GitBranch> = {
  branch_created: GitBranch, pr_opened: GitPullRequest, changes_requested: RotateCcw, review_approved: Eye, pr_merged: GitMerge, ci_failed: ShieldX,
}

const TRIGGERS: Record<string, { label: string; hint: string }> = {
  branch_created: { label: 'Branch created', hint: 'git checkout -b feature/KEY-…' },
  pr_opened: { label: 'Pull request opened', hint: 'gh pr create' },
  changes_requested: { label: 'Changes requested', hint: 'reviewer requests changes' },
  review_approved: { label: 'Review approved', hint: 'reviewer approves' },
  pr_merged: { label: 'Pull request merged', hint: 'PR merged into main' },
  ci_failed: { label: 'CI failed', hint: 'any required check fails' },
}

const ISSUE_TYPES = ['story', 'task', 'bug', 'subtask', 'epic', 'initiative']

const scopeText = (r: Rule) => {
  const p = r.projects?.length ? r.projects.join(', ') : 'all projects'
  const t = r.issueTypes?.length ? r.issueTypes.join(', ') : 'all types'
  return `${p} · ${t}`
}

export function AutomationsView() {
  const { rules, statuses, projects, live, toggleRule, setRuleStatus, saveRules, policy, savePolicy, activity } = useStore()
  const log = activity.filter(e => e.kind === 'automation')
  const [editing, setEditing] = useState<string | null>(null)
  const [adding, setAdding] = useState<{ trigger: string; toStatus: string } | null>(null)

  const update = (id: string, patch: Partial<Rule>) => saveRules(rules.map(r => r.id === id ? { ...r, ...patch } : r))
  const toggleIn = (list: string[] | undefined, v: string) => (list ?? []).includes(v) ? (list ?? []).filter(x => x !== v) : [...(list ?? []), v]
  const add = async () => {
    if (!adding) return
    const rule: Rule = { id: '', trigger: adding.trigger, label: TRIGGERS[adding.trigger]?.label ?? adding.trigger, toStatus: adding.toStatus, enabled: true, projects: [], issueTypes: [] }
    if (await saveRules([...rules, rule])) setAdding(null)
  }
  const projectChoices = (r: Rule) => [...new Set([...projects, ...(r.projects ?? [])])].sort()

  return (
    <div className="automations scroll">
      <div className="view-head">
        <div>
          <h1>Automations</h1>
          <p className="muted">When something happens in code, Maestro moves the ticket in Jira for you. No tab-switching.</p>
          <p className="muted small">{live ? 'Changes are saved to your config.yaml.' : 'Demo data: changes last until Maestro quits.'}</p>
        </div>
        {!adding && (
          <button className="btn primary" onClick={() => setAdding({ trigger: 'branch_created', toStatus: statuses[0] ?? '' })}><Plus size={14} /> Rule</button>
        )}
      </div>

      <div className="card rule-policy">
        <div className="policy-row">
          <div>
            <div className="rule-label">Only tickets assigned to me</div>
            <div className="muted small">Teammates' tickets are never moved. Done tickets are never moved by a rule either.</div>
          </div>
          <button className={cx('switch', !policy.anyAssignee && 'on')} aria-label="Only tickets assigned to me"
            onClick={() => void savePolicy({ ...policy, anyAssignee: !policy.anyAssignee })}><i /></button>
        </div>
        <div className="policy-row">
          <div>
            <div className="rule-label">Only my branches and pull requests</div>
            <div className="muted small">A pull request someone else opened for your ticket shows on the timeline but fires nothing.</div>
          </div>
          <button className={cx('switch', !policy.anyAuthor && 'on')} aria-label="Only my branches and pull requests"
            onClick={() => void savePolicy({ ...policy, anyAuthor: !policy.anyAuthor })}><i /></button>
        </div>
        <div className="policy-row">
          <div>
            <div className="rule-label">Dry run</div>
            <div className="muted small">Log what each rule would do in the automation log below, and leave Jira alone.</div>
          </div>
          <button className={cx('switch', policy.dryRun && 'on')} aria-label="Dry run"
            onClick={() => void savePolicy({ ...policy, dryRun: !policy.dryRun })}><i /></button>
        </div>
      </div>

      <div className="rules">
        {rules.map(r => {
          const Icon = TRIGGER_ICON[r.trigger] ?? Zap
          const scoped = !!(r.projects?.length || r.issueTypes?.length)
          return (
            <div key={r.id} className={cx('rule card', !r.enabled && 'off')}>
              <div className="rule-when">
                <span className="rule-tag">When</span>
                <span className="rule-icon"><Icon size={16} /></span>
                <div>
                  <div className="rule-label">{r.label}</div>
                  <div className="mono muted small">{TRIGGERS[r.trigger]?.hint}</div>
                </div>
              </div>
              <ArrowRight className="rule-arrow" size={18} />
              <div className="rule-then">
                <span className="rule-tag">Then move to</span>
                <select className={cx('status-select', `tone-${statusTone(r.toStatus)}`)} value={r.toStatus} onChange={e => setRuleStatus(r.id, e.target.value)}>
                  {[...new Set([...statuses, r.toStatus])].map(s => <option key={s} value={s}>{s}</option>)}
                </select>
              </div>
              <div className="rule-actions">
                <button className={cx('scope-btn', scoped && 'on')} title="Limit to projects or issue types" onClick={() => setEditing(e => e === r.id ? null : r.id)}>
                  <Filter size={12} /> {scopeText(r)}
                </button>
                <button className="icon-btn" title="Delete rule" onClick={() => void saveRules(rules.filter(x => x.id !== r.id))}><Trash2 size={13} /></button>
                <button className={cx('switch', r.enabled && 'on')} onClick={() => toggleRule(r.id)} aria-label="Toggle rule"><i /></button>
              </div>
              {editing === r.id && (
                <div className="rule-scope">
                  <div>
                    <span className="rule-tag">Projects</span>
                    <button className={cx('scope-chip', !r.projects?.length && 'on')} onClick={() => update(r.id, { projects: [] })}>All</button>
                    {projectChoices(r).map(p => (
                      <button key={p} className={cx('scope-chip', r.projects?.includes(p) && 'on')} onClick={() => update(r.id, { projects: toggleIn(r.projects, p) })}>{p}</button>
                    ))}
                  </div>
                  <div>
                    <span className="rule-tag">Types</span>
                    <button className={cx('scope-chip', !r.issueTypes?.length && 'on')} onClick={() => update(r.id, { issueTypes: [] })}>All</button>
                    {ISSUE_TYPES.map(t => (
                      <button key={t} className={cx('scope-chip', r.issueTypes?.includes(t) && 'on')} onClick={() => update(r.id, { issueTypes: toggleIn(r.issueTypes, t) })}>{t}</button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )
        })}

        {adding && (
          <div className="rule card rule-new">
            <div className="rule-when">
              <span className="rule-tag">When</span>
              <select className="status-select" value={adding.trigger} onChange={e => setAdding({ ...adding, trigger: e.target.value })}>
                {Object.entries(TRIGGERS).map(([id, t]) => <option key={id} value={id}>{t.label}</option>)}
              </select>
            </div>
            <ArrowRight className="rule-arrow" size={18} />
            <div className="rule-then">
              <span className="rule-tag">Then move to</span>
              <select className={cx('status-select', `tone-${statusTone(adding.toStatus)}`)} value={adding.toStatus} onChange={e => setAdding({ ...adding, toStatus: e.target.value })}>
                {statuses.map(s => <option key={s} value={s}>{s}</option>)}
              </select>
            </div>
            <div className="rule-actions">
              <button className="btn ghost small" onClick={() => setAdding(null)}>Cancel</button>
              <button className="btn primary small" disabled={!adding.toStatus} onClick={() => void add()}>Add rule</button>
            </div>
          </div>
        )}
      </div>

      <section className="card">
        <div className="card-head"><h2><Zap size={15} /> Automation log</h2></div>
        <Timeline events={log} showKey />
      </section>
    </div>
  )
}
