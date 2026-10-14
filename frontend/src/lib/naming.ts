// Branch names and pull request text from the configured templates.
// Mirrors internal/flow/naming.go; both pass internal/flow/testdata/naming.json.
import type { Naming } from '../types'

export interface NamingIssue { key: string; summary: string; type: string; security?: string; url: string }

export const defaultNaming: Naming = {
  branchTemplate: '{prefix}/{key}-{slug}', prefix: { bug: 'fix', default: 'feature' }, slugWords: 5,
  prTitle: '[{key}] {summary}', prBody: 'Jira: {url}', mergeMethod: 'squash',
}

/** Placeholders the templates understand, for the Settings help. */
export const PLACEHOLDERS: Record<string, string> = {
  key: 'PLAT-142', project: 'PLAT', type: 'story', summary: 'the ticket title', slug: 'words-from-the-title',
  prefix: 'feature / fix…', url: 'link to the ticket', branch: 'the branch (PR only)',
}

export const project = (key: string) => { const i = key.lastIndexOf('-'); return i > 0 ? key.slice(0, i) : key }

function vars(n: Naming, is: NamingIssue): Record<string, string> {
  const summary = is.security ? '' : is.summary
  let slug = summary.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
  if (slug) slug = slug.split('-').slice(0, Math.max(n.slugWords, 1)).join('-')
  const prefix = n.prefix?.[is.type] || n.prefix?.default || ''
  return { key: is.key, project: project(is.key), type: is.type, summary, slug, prefix, url: is.url }
}

const render = (tmpl: string, v: Record<string, string>) =>
  tmpl.replace(/\{[a-z]+\}/g, p => v[p.slice(1, -1)] ?? p)

export function branchName(n: Naming, is: NamingIssue): string {
  const b = render(n.branchTemplate || defaultNaming.branchTemplate, vars(n, is))
    .replace(/[\s~^:?*[\\]+|\.\.+|@\{/g, '-')
    .replace(/([-_/.])[-_/.]+/g, '$1')
    .replace(/^[-_/.]+|[-_/.]+$/g, '')
  return b.endsWith('.lock') ? b.slice(0, -5) : b
}

export function prTitle(n: Naming, is: NamingIssue, branch: string): string {
  return render(n.prTitle, { ...vars(n, is), branch }).split(/\s+/).filter(Boolean).join(' ').replace(/[ :\-|]+$/, '')
}

export function prBody(n: Naming, is: NamingIssue, branch: string): string {
  return render(n.prBody, { ...vars(n, is), branch }).replace(/[ \t]+(\n|$)/g, '$1').trim()
}

/** Quotes a value for the shell (bash/zsh). Multi-line text uses $'…'. */
export function shellQuote(s: string): string {
  if (!s.includes('\n')) return `'${s.replace(/'/g, `'\\''`)}'`
  return `$'${s.replace(/\\/g, '\\\\').replace(/'/g, "\\'").replace(/\n/g, '\\n')}'`
}

/** The `gh pr create` command for a ticket. */
export function prCreateCommand(n: Naming, is: NamingIssue, branch: string): string {
  const title = prTitle(n, is, branch), body = prBody(n, is, branch)
  return `gh pr create --title ${shellQuote(title)} --body ${shellQuote(body)}`
}

export const prMergeCommand = (n: Naming) => `gh pr merge --${['merge', 'squash', 'rebase'].includes(n.mergeMethod) ? n.mergeMethod : 'squash'} --delete-branch`
