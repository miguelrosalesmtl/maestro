// Which tickets the sidebar tree shows. Pure (no React, no imports at run
// time) so it can be tested with node --test.
import type { Issue } from '../types'

export interface TreeFilter {
  mine: boolean // "My work": only my tickets
  me?: string // my account ID
  query: string // lower-cased text filter, '' = none
  sprintOk: (i: Issue) => boolean // the sprint selector (always true for "All sprints")
  filtered: boolean // any filter is on (mine, query or sprint)
}

const isContainer = (i: Issue) => i.type === 'initiative' || i.type === 'epic'

/**
 * A ticket shows when it matches the filters itself, or when something under
 * it does (so the hierarchy stays as context). Epics and initiatives match on
 * their own fields too: an epic assigned to me shows in My work even if none
 * of its children are mine. When an epic or initiative matches the text
 * filter, its subtree shows without the text filter (still mine and sprint).
 * Without filters everything shows.
 */
export function visibleKeys(issues: Record<string, Issue>, roots: Issue[], f: TreeFilter): Set<string> {
  const out = new Set<string>()
  const walk = (i: Issue, seen: Set<string>, textFromAbove: boolean): boolean => {
    if (seen.has(i.key)) return false // guards against a cycle in bad data
    seen.add(i.key)
    const textHit = !f.query || i.key.toLowerCase().includes(f.query) || i.summary.toLowerCase().includes(f.query)
    const self = (!f.mine || i.assignee?.id === f.me) && (textHit || textFromAbove) && f.sprintOk(i)
    const passText = textFromAbove || (!!f.query && textHit && isContainer(i))
    let any = false
    for (const k of i.childKeys) {
      const kid = issues[k]
      if (kid && walk(kid, seen, passText)) any = true
    }
    const ok = !f.filtered || any || self
    if (ok) out.add(i.key)
    return ok
  }
  for (const r of roots) walk(r, new Set(), false)
  return out
}
