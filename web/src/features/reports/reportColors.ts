import { CHART_OTHER, CHART_PALETTE } from '@/components/ui/chartPalette'
import type { Id } from '@/api/types'

export const OTHER_ID = '__other'

function hash(id: string): number {
  let h = 2166136261
  for (let i = 0; i < id.length; i++) {
    h ^= id.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

// A row's colour comes from its id, so it is the same in every chart and does
// not move when the ranking changes. Two rows on one chart that hash to the
// same slot are separated by probing to the next free slot, in id order, so
// the outcome depends only on the set shown.
export function seriesColors(ids: Id[]): Record<Id, string> {
  const out: Record<Id, string> = {}
  const taken = new Set<number>()
  const rows = ids.filter((id) => id !== OTHER_ID).sort()
  for (const id of rows) {
    let slot = hash(id) % CHART_PALETTE.length
    for (let i = 0; i < CHART_PALETTE.length && taken.has(slot); i++) slot = (slot + 1) % CHART_PALETTE.length
    taken.add(slot)
    out[id] = CHART_PALETTE[slot]
  }
  if (ids.includes(OTHER_ID)) out[OTHER_ID] = CHART_OTHER
  return out
}
