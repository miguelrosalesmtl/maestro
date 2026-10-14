import { test } from 'node:test'
import assert from 'node:assert/strict'
import { workspaceName } from '../src/lib/workspace.ts'

test('a Jira Cloud site is named by its subdomain', () => {
  assert.equal(workspaceName({ mode: 'live', site: 'acme.atlassian.net' }), 'acme')
  assert.equal(workspaceName({ mode: 'live', site: 'Acme-Eng.jira.com' }), 'Acme-Eng')
})

test('a self-hosted Jira keeps its host', () => {
  assert.equal(workspaceName({ mode: 'live', site: 'jira.example.org' }), 'jira.example.org')
})

test('no site means demo data (or still starting)', () => {
  assert.equal(workspaceName({ mode: 'demo', site: '' }), 'Demo workspace')
  assert.equal(workspaceName(null), 'Demo workspace')
  assert.equal(workspaceName({ mode: 'connecting', site: '' }), 'Maestro')
})
