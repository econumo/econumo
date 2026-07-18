import { describe, expect, it } from 'vitest'
import { METRICS } from './metrics'

// Every metric in the catalogue must be fired somewhere in the app, or be
// explicitly listed here with the reason it can't be. This keeps "defined but
// never tracked" events (e.g. USER_REGISTRATION before it was wired) from
// accumulating silently. The converse (a feature shipping without an event)
// can't be caught mechanically — every new user-facing feature must add and
// fire a METRICS entry (see the frontend architecture notes in CLAUDE.md).
const NOT_WIRED: Record<string, string> = {}

const sources = import.meta.glob('/src/**/*.{ts,tsx}', {
  eager: true,
  query: '?raw',
  import: 'default',
}) as Record<string, string>

function referencedKeys(): Set<string> {
  const found = new Set<string>()
  for (const [path, content] of Object.entries(sources)) {
    if (path.endsWith('/metrics.ts') || /\.test\.tsx?$/.test(path)) {
      continue
    }
    for (const match of content.matchAll(/METRICS\.([A-Z_0-9]+)/g)) {
      found.add(match[1])
    }
  }
  return found
}

describe('metrics catalogue coverage', () => {
  const referenced = referencedKeys()

  it('every metric is tracked somewhere in the app (or documented in NOT_WIRED)', () => {
    const untracked = Object.keys(METRICS).filter((key) => !referenced.has(key) && !(key in NOT_WIRED))
    expect(untracked).toEqual([])
  })

  it('NOT_WIRED only lists metrics that are actually untracked', () => {
    const stale = Object.keys(NOT_WIRED).filter((key) => referenced.has(key))
    expect(stale).toEqual([])
  })

  it('NOT_WIRED only lists metrics that exist in the catalogue', () => {
    const unknown = Object.keys(NOT_WIRED).filter((key) => !(key in METRICS))
    expect(unknown).toEqual([])
  })
})
