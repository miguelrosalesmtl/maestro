import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { branchName, defaultNaming, prBody, prCreateCommand, prTitle, shellQuote } from '../src/lib/naming.ts'

// The same cases run against internal/flow/naming.go (TestNamingCases).
const cases = JSON.parse(readFileSync(new URL('../../internal/flow/testdata/naming.json', import.meta.url), 'utf8'))

for (const c of cases) {
  test(`naming: ${c.name}`, () => {
    const n = c.naming ?? defaultNaming
    const branch = branchName(n, c.issue)
    assert.equal(branch, c.branch)
    assert.equal(prTitle(n, c.issue, branch), c.title)
    assert.equal(prBody(n, c.issue, branch), c.body)
  })
}

test('shell quoting keeps titles intact', () => {
  assert.equal(shellQuote(`it's "fine" $HOME`), `'it'\\''s "fine" $HOME'`)
  assert.equal(shellQuote('a\nb\'c'), `$'a\\nb\\'c'`)
  const cmd = prCreateCommand(defaultNaming, { key: 'A-1', summary: "Don't break", type: 'story', url: 'u' }, 'b')
  assert.equal(cmd, `gh pr create --title '[A-1] Don'\\''t break' --body 'Jira: u'`)
})
