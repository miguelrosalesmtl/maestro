// Run with: npm test (node --test, TypeScript types stripped by Node).
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { visibleKeys, type TreeFilter } from '../src/lib/tree.ts'
import type { Issue } from '../src/types.ts'

const me = 'me'
function issue(key: string, type: string, assignee: string | null, parentKey = '', sprint = ''): Issue {
  return { key, type, summary: `${key} summary`, assignee: assignee ? { id: assignee, name: assignee } : null, parentKey, sprint, childKeys: [] } as unknown as Issue
}
function tree(...list: Issue[]) {
  const byKey = Object.fromEntries(list.map(i => [i.key, i]))
  for (const i of list) if (byKey[i.parentKey]) byKey[i.parentKey].childKeys.push(i.key)
  return { byKey, roots: list.filter(i => !byKey[i.parentKey]) }
}
const filter = (p: Partial<TreeFilter>): TreeFilter => {
  const f = { mine: false, me, query: '', sprintOk: () => true, ...p }
  return { ...f, filtered: f.mine || !!f.query || p.sprintOk !== undefined }
}

// Issue #2: an epic assigned to me with an unassigned child shows in My work.
test('My work shows an epic assigned to me even when no child is mine', () => {
  const { byKey, roots } = tree(issue('E-1', 'epic', me), issue('S-1', 'spike', null, 'E-1'))
  const shown = visibleKeys(byKey, roots, filter({ mine: true }))
  assert.ok(shown.has('E-1'))
  assert.ok(!shown.has('S-1'))
})

test('My work shows an epic of mine with no children', () => {
  const { byKey, roots } = tree(issue('E-2', 'epic', me))
  assert.ok(visibleKeys(byKey, roots, filter({ mine: true })).has('E-2'))
})

test('My work keeps someone else\'s epic as context for my story', () => {
  const { byKey, roots } = tree(issue('E-3', 'epic', 'ana'), issue('S-3', 'story', me, 'E-3'), issue('S-4', 'story', 'ana', 'E-3'))
  const shown = visibleKeys(byKey, roots, filter({ mine: true }))
  assert.deepEqual([...shown].sort(), ['E-3', 'S-3'])
})

test('the text filter finds an epic by key and shows its subtree', () => {
  const { byKey, roots } = tree(issue('I-1', 'initiative', 'ana'), issue('E-4', 'epic', 'ana', 'I-1'), issue('S-5', 'story', 'bo', 'E-4'), issue('E-5', 'epic', 'ana', 'I-1'))
  const shown = visibleKeys(byKey, roots, filter({ query: 'e-4' }))
  assert.deepEqual([...shown].sort(), ['E-4', 'I-1', 'S-5'])
})

test('the sprint filter checks an epic\'s own sprint', () => {
  const { byKey, roots } = tree(issue('E-6', 'epic', me, '', 'Sprint 42'), issue('E-7', 'epic', me, '', 'Sprint 43'))
  const shown = visibleKeys(byKey, roots, filter({ mine: true, sprintOk: i => i.sprint === 'Sprint 42' }))
  assert.deepEqual([...shown], ['E-6'])
})

test('without filters everything shows, including empty epics', () => {
  const { byKey, roots } = tree(issue('E-8', 'epic', 'ana'), issue('S-8', 'story', 'bo'))
  assert.equal(visibleKeys(byKey, roots, filter({})).size, 2)
})
