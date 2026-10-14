import { useEffect, useMemo, useState } from 'react'
import { AlertTriangle, Check, GitBranch, GitPullRequest, Plug, Plus, RotateCcw, Save, SquareTerminal, Trash2 } from 'lucide-react'
import { api } from '../api'
import { useStore } from '../store'
import type { Issue, Settings } from '../types'
import { cx, isWork } from '../lib/util'
import { PLACEHOLDERS, branchName, prBody, prCreateCommand, prMergeCommand, prTitle } from '../lib/naming'
import { useAction } from '../keys'

const PREFIX_TYPES = ['default', 'story', 'task', 'bug', 'subtask', 'epic']
type Pair = { k: string; v: string }
const toPairs = (m: Record<string, string> | undefined): Pair[] => Object.entries(m ?? {}).map(([k, v]) => ({ k, v }))
const fromPairs = (p: Pair[]) => Object.fromEntries(p.filter(x => x.k.trim()).map(x => [x.k.trim(), x.v]))

/** The editable form: env maps as rows, startup commands as text. */
interface Form {
  s: Settings
  env: Pair[]
  startup: string
  projects: Record<string, { env: Pair[]; startup: string }>
}

const toForm = (s: Settings): Form => ({
  s, env: toPairs(s.terminal.env), startup: (s.terminal.startup ?? []).join('\n'),
  projects: Object.fromEntries(Object.entries(s.terminal.projects ?? {}).map(([k, p]) => [k, { env: toPairs(p.env), startup: (p.startup ?? []).join('\n') }])),
})

const fromForm = (f: Form): Settings => ({
  ...f.s,
  terminal: {
    ...f.s.terminal,
    env: fromPairs(f.env),
    startup: f.startup.split('\n'),
    projects: Object.fromEntries(Object.entries(f.projects).map(([k, p]) => [k, { env: fromPairs(p.env), startup: p.startup.split('\n') }])),
  },
})

export function SettingsView() {
  const { issues, focusKey, me, projects, connection, github, applySettings, pushToast } = useStore()
  const [saved, setSaved] = useState<Form | null>(null)
  const [form, setForm] = useState<Form | null>(null)
  const [problems, setProblems] = useState<Settings['problems']>([])
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api.settings().then(s => { const f = toForm(s); setSaved(f); setForm(f) })
      .catch(err => pushToast({ tone: 'error', title: 'Could not load settings', detail: String(err) }))
  }, [pushToast])

  const dirty = useMemo(() => !!form && !!saved && JSON.stringify(fromForm(form)) !== JSON.stringify(fromForm(saved)), [form, saved])

  // Preview on the ticket you're on, else your first open work item.
  const sample: Issue | undefined = useMemo(() => {
    const focus = issues[focusKey]
    if (focus && isWork(focus)) return focus
    return Object.values(issues).find(i => isWork(i) && i.assignee?.id === me?.id) ?? Object.values(issues).find(isWork)
  }, [issues, focusKey, me])

  const save = async () => {
    if (!form) return
    setBusy(true)
    try {
      const res = await api.saveSettings(fromForm(form))
      setProblems(res.problems ?? [])
      if (res.saved) {
        const f = toForm(res)
        setSaved(f); setForm(f); applySettings(res)
        pushToast({ tone: 'dev', title: 'Settings saved', detail: res.path })
      }
    } catch (err) {
      pushToast({ tone: 'error', title: 'Settings not saved', detail: String(err).replace(/^Error: /, '') })
    } finally {
      setBusy(false)
    }
  }
  useAction('settings.save', () => { if (dirty && !busy) void save() })

  if (!form) return <div className="settings scroll"><div className="view-head"><h1>Settings</h1></div></div>

  const s = form.s
  const n = s.naming
  const setNaming = (patch: Partial<typeof n>) => setForm({ ...form, s: { ...s, naming: { ...n, ...patch } } })
  const setTerm = (patch: Partial<typeof s.terminal>) => setForm({ ...form, s: { ...s, terminal: { ...s.terminal, ...patch } } })
  const problem = (path: string) => problems.filter(p => p.path === path || p.path.startsWith(path + '.'))
  const allProjects = [...new Set([...projects, ...Object.keys(form.projects)])].sort()

  const branch = sample ? branchName(n, sample) : ''
  const cfgName = s.path ? s.path.replace(/^.*\/(\.config|Library\/Application Support)\//, '~/$1/') : 'config.yaml'

  return (
    <div className="settings scroll">
      <div className="view-head">
        <div>
          <h1>Settings</h1>
          <p className="muted">
            {s.writable ? <>Saved to <code title={s.path}>{cfgName}</code>. Only what you change is written; the rest comes from the defaults{s.teamPath ? ' and your team file' : ''}.</>
              : 'Your config.yaml could not be read, so settings cannot be saved. See the log.'}
          </p>
        </div>
        <div className="settings-actions">
          <button className="btn ghost" disabled={!dirty || busy} onClick={() => { setForm(saved); setProblems([]) }}><RotateCcw size={13} /> Revert</button>
          <button className="btn primary" disabled={!dirty || busy || !s.writable} onClick={() => void save()} title="Save (Ctrl+S)">
            {dirty ? <><Save size={14} /> Save</> : <><Check size={14} /> Saved</>}
          </button>
        </div>
      </div>

      {problems.length > 0 && (
        <div className="settings-problems card">
          <AlertTriangle size={15} />
          <div>
            <b>Not saved. Fix these first:</b>
            <ul>{problems.map(p => <li key={p.path + p.message}><code>{p.path}</code> {p.message}</li>)}</ul>
          </div>
        </div>
      )}

      <section className="card settings-section">
        <div className="card-head"><h2><GitBranch size={15} /> Branches</h2><span className="muted">Used by “create branch”, Start work and the terminal’s $MAESTRO_BRANCH.</span></div>
        <Field label="Branch name" problems={problem('dev.branch.template')} hint="Must contain {key}, so Maestro can match the branch to its ticket.">
          <input className="mono" value={n.branchTemplate} onChange={e => setNaming({ branchTemplate: e.target.value })} spellCheck={false} />
        </Field>
        <Field label="Prefix by issue type" problems={problem('dev.branch.prefix')} hint="Fills {prefix}. Empty uses the default.">
          <div className="prefix-grid">
            {PREFIX_TYPES.map(t => (
              <label key={t}><span>{t}</span>
                <input className="mono" value={n.prefix?.[t] ?? ''} placeholder={t === 'default' ? '' : n.prefix?.default || '—'}
                  onChange={e => setNaming({ prefix: { ...n.prefix, [t]: e.target.value } })} spellCheck={false} />
              </label>
            ))}
          </div>
        </Field>
        <Field label="Words from the title" problems={problem('dev.branch.slugWords')} hint="How many words of the summary go into {slug} (1–12).">
          <input type="number" min={1} max={12} value={n.slugWords} onChange={e => setNaming({ slugWords: Number(e.target.value) })} className="narrow" />
        </Field>
        <Placeholders />
      </section>

      <section className="card settings-section">
        <div className="card-head"><h2><GitPullRequest size={15} /> Pull requests</h2><span className="muted">Used by the terminal’s “open PR” and “merge PR”.</span></div>
        <Field label="Title" problems={problem('dev.pr.titleTemplate')}>
          <input className="mono" value={n.prTitle} onChange={e => setNaming({ prTitle: e.target.value })} spellCheck={false} />
        </Field>
        <Field label="Description" problems={problem('dev.pr.bodyTemplate')}>
          <textarea className="mono" rows={4} value={n.prBody} onChange={e => setNaming({ prBody: e.target.value })} spellCheck={false} />
        </Field>
        <Field label="Merge with" problems={problem('dev.pr.mergeMethod')}>
          <div className="seg">
            {['squash', 'merge', 'rebase'].map(m => (
              <button key={m} className={cx(n.mergeMethod === m && 'on')} onClick={() => setNaming({ mergeMethod: m })}>{m}</button>
            ))}
          </div>
        </Field>

        {sample && (
          <div className="naming-preview">
            <div className="muted small">Preview on {sample.key}{sample.security ? ' (secured: its title stays out)' : ''}</div>
            <dl>
              <dt>Branch</dt><dd className="mono">{branch || '—'}</dd>
              <dt>PR title</dt><dd>{prTitle(n, sample, branch) || '—'}</dd>
              <dt>PR description</dt><dd className="pre">{prBody(n, sample, branch) || '—'}</dd>
              <dt>Commands</dt><dd className="mono small">{prCreateCommand(n, sample, branch)}<br />{prMergeCommand(n)}</dd>
            </dl>
          </div>
        )}
      </section>

      <section className="card settings-section">
        <div className="card-head"><h2><SquareTerminal size={15} /> Terminal</h2><span className="muted">Applies to terminals opened after saving.</span></div>
        <Field label="Shell" problems={problem('terminal.shell')} hint="A program name or path, e.g. zsh, fish, /bin/bash. Empty uses your login shell.">
          <input className="mono" value={s.terminal.shell} placeholder="$SHELL" onChange={e => setTerm({ shell: e.target.value })} spellCheck={false} />
        </Field>
        <Field label="Environment" problems={problem('terminal.env')} hint="Set in every terminal. MAESTRO_ variables are Maestro’s.">
          <PairRows rows={form.env} onChange={env => setForm({ ...form, env })} keyHint="NAME" valueHint="value" />
        </Field>
        <Field label="Startup commands" problems={problem('terminal.startup')} hint="Typed into every new terminal, one per line (e.g. nvm use).">
          <textarea className="mono" rows={3} value={form.startup} onChange={e => setForm({ ...form, startup: e.target.value })} spellCheck={false} />
        </Field>
        <Field label="One-click commands" problems={problem('terminal.commands')} hint="Extra buttons above the ticket terminal. $MAESTRO_TICKET and $MAESTRO_BRANCH work here.">
          <PairRows rows={(s.terminal.commands ?? []).map(c => ({ k: c.label, v: c.command }))} keyHint="label" valueHint="command"
            onChange={rows => setTerm({ commands: rows.map(r => ({ label: r.k, command: r.v })) })} />
        </Field>

        <div className="sub-head">By project</div>
        <p className="muted small">Added to the settings above for that project’s ticket terminals.</p>
        {allProjects.map(key => {
          const p = form.projects[key] ?? { env: [], startup: '' }
          const set = (patch: Partial<typeof p>) => setForm({ ...form, projects: { ...form.projects, [key]: { ...p, ...patch } } })
          return (
            <details key={key} className="project-term" open={!!(p.env.length || p.startup.trim())}>
              <summary><b>{key}</b> <span className="muted small">{[p.env.length && `${p.env.length} variables`, p.startup.trim() && `${p.startup.trim().split('\n').length} startup commands`].filter(Boolean).join(' · ') || 'nothing extra'}</span></summary>
              <Field label="Environment" problems={problem(`terminal.projects.${key}`)}>
                <PairRows rows={p.env} onChange={env => set({ env })} keyHint="NAME" valueHint="value" />
              </Field>
              <Field label="Startup commands">
                <textarea className="mono" rows={2} value={p.startup} onChange={e => set({ startup: e.target.value })} spellCheck={false} />
              </Field>
            </details>
          )
        })}
      </section>

      <section className="card settings-section">
        <div className="card-head"><h2><Plug size={15} /> Connections</h2><span className="muted">Signing in happens in a terminal, so tokens go straight to your keychain.</span></div>
        <dl className="conn-list">
          <dt>Jira</dt>
          <dd>{connection?.mode === 'live' ? <>Connected to <b>{connection.site}</b></> : connection?.configured ? <>Not connected: {connection.error}</> : 'Not connected (showing sample tickets)'}</dd>
          <dt>GitHub</dt>
          <dd>{github?.state === 'ok' ? <>Signed in as <b>@{github.user}</b> · following {github.repos?.length ?? 0} repositories</>
            : github?.state === 'error' ? <>Not connected: {github.error}</> : github?.state === 'connecting' ? 'Connecting…' : 'Off (add repositories to a project in config.yaml)'}</dd>
        </dl>
        <div className="cmd-list mono small">
          <div><span>maestro-jira login</span><span className="muted">connect or change the Jira site / token</span></div>
          <div><span>maestro-jira check</span><span className="muted">test the token, permissions and config</span></div>
          <div><span>gh auth login</span><span className="muted">or: maestro-jira github-login</span></div>
        </div>
      </section>
    </div>
  )
}

function Field({ label, hint, problems, children }: { label: string; hint?: string; problems?: { message: string }[]; children: React.ReactNode }) {
  return (
    <div className={cx('field-row', !!problems?.length && 'bad')}>
      <label className="field-label">{label}</label>
      <div className="field-body">
        {children}
        {problems?.map(p => <div key={p.message} className="field-problem">{p.message}</div>)}
        {hint && <div className="field-hint muted small">{hint}</div>}
      </div>
    </div>
  )
}

function PairRows({ rows, onChange, keyHint, valueHint }: { rows: Pair[]; onChange: (rows: Pair[]) => void; keyHint: string; valueHint: string }) {
  const set = (i: number, patch: Partial<Pair>) => onChange(rows.map((r, j) => j === i ? { ...r, ...patch } : r))
  return (
    <div className="pair-rows">
      {rows.map((r, i) => (
        <div key={i} className="pair-row">
          <input className="mono" value={r.k} placeholder={keyHint} onChange={e => set(i, { k: e.target.value })} spellCheck={false} />
          <input className="mono" value={r.v} placeholder={valueHint} onChange={e => set(i, { v: e.target.value })} spellCheck={false} />
          <button className="icon-btn" title="Remove" onClick={() => onChange(rows.filter((_, j) => j !== i))}><Trash2 size={13} /></button>
        </div>
      ))}
      <button className="btn ghost small" onClick={() => onChange([...rows, { k: '', v: '' }])}><Plus size={12} /> Add</button>
    </div>
  )
}

function Placeholders() {
  return (
    <div className="placeholders small">
      <span className="muted">Placeholders:</span>
      {Object.entries(PLACEHOLDERS).map(([k, v]) => <span key={k} title={v}><code>{`{${k}}`}</code> <span className="muted">{v}</span></span>)}
    </div>
  )
}
