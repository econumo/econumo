import type { BudgetBuckets } from './budgetMath'
import type { BudgetDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'

export interface ElementMoveItem {
  id: Id
  folderId: Id | null
  // position drives the local preview only; the server orders by afterId.
  position: number
  afterId: Id | null
}

// Only the moved element is sent, with its target folder and the neighbour it
// landed after; the server derives its sort key from that and writes one row.
export function computeElementMove(buckets: BudgetBuckets, activeId: string, overId: string): ElementMoveItem | null {
  const base = arrangementFromBuckets(buckets)
  const moved = moveElementInArrangement(base, activeId, overId)
  const item = arrangementItem(moved, activeId)
  const before = arrangementItem(base, activeId)
  // Dropping an element back onto its own slot is not a move.
  if (!item || (before && before.folderId === item.folderId && before.position === item.position)) {
    return null
  }
  return item
}

// --- live drag preview -------------------------------------------------------
// The page keeps a lightweight arrangement (container -> ordered element ids)
// while a drag is in flight, so the table renders the preview instead of the
// server order and never snaps back on drop.

export interface ElementContainer {
  folderId: Id | null
  ids: string[]
}

export function arrangementFromBuckets(buckets: BudgetBuckets): ElementContainer[] {
  return [
    ...buckets.withFolder.map((b) => ({ folderId: (b.folder?.id ?? null) as Id | null, ids: b.elements.map((e) => e.id) })),
    { folderId: null, ids: buckets.withoutFolder.elements.map((e) => e.id) },
  ]
}

// `overId` is another element id or a container id `bfolder:<id|null>`
export function moveElementInArrangement(arrangement: ElementContainer[], activeId: string, overId: string): ElementContainer[] {
  const next = arrangement.map((c) => ({ ...c, ids: [...c.ids] }))
  const source = next.find((c) => c.ids.includes(activeId))
  if (!source) {
    return arrangement
  }
  let target: ElementContainer | undefined
  let insertAt: number
  if (overId.startsWith('bfolder:')) {
    const folderId = overId.slice('bfolder:'.length)
    target = next.find((c) => (folderId === 'null' ? c.folderId === null : String(c.folderId) === folderId))
    insertAt = target ? target.ids.length : 0
  } else {
    target = next.find((c) => c.ids.includes(overId))
    // An empty folder has no rows, so the pointer can only land on the section
    // itself — which dnd-kit reports as the folder's own sortable id (the bare
    // folder id, registered for folder reordering), never `bfolder:<id>`.
    // Treat that as "append to this folder" instead of dropping the move.
    if (!target) {
      target = next.find((c) => c.folderId !== null && String(c.folderId) === overId)
      insertAt = target ? target.ids.length : 0
    } else {
      insertAt = target.ids.indexOf(overId)
    }
  }
  if (!target) {
    return arrangement
  }
  const fromIndex = source.ids.indexOf(activeId)
  source.ids.splice(fromIndex, 1)
  if (target === source && fromIndex < insertAt) {
    insertAt = Math.min(insertAt, target.ids.length)
  }
  target.ids.splice(insertAt, 0, activeId)
  // a no-op move returns the SAME reference so state setters bail out — the
  // drag-over → reorder → re-measure → drag-over feedback loop never spins up
  const unchanged = next.every(
    (c, i) => c.ids.length === arrangement[i].ids.length && c.ids.every((id, j) => id === arrangement[i].ids[j]),
  )
  return unchanged ? arrangement : next
}

export function arrangementItem(arrangement: ElementContainer[], activeId: string): ElementMoveItem | null {
  for (const container of arrangement) {
    const index = container.ids.indexOf(activeId)
    if (index !== -1) {
      return {
        id: activeId,
        folderId: container.folderId,
        position: index,
        // The server places relative to a neighbour, so report the element this
        // one now sits after (null = first in its container).
        afterId: index > 0 ? container.ids[index - 1] : null,
      }
    }
  }
  return null
}

// Patch element folderId + position to match the arrangement so bucketElements
// (incl. per-folder stats) reproduces the preview; archived elements untouched.
/** The arrangement carries only folderId + position, which both the budget and the
 *  plan element shapes have — so the same optimistic re-placement serves both views. */
export function placeElements<T extends { id: Id; folderId: Id | null; position: number }>(
  elements: T[],
  arrangement: ElementContainer[],
): T[] {
  const placement = new Map<string, { folderId: Id | null; position: number }>()
  let position = 0
  for (const container of arrangement) {
    for (const id of container.ids) {
      placement.set(id, { folderId: container.folderId, position })
      position++
    }
  }
  return elements.map((el) => {
    const placed = placement.get(el.id)
    return placed ? { ...el, folderId: placed.folderId, position: placed.position } : el
  })
}

export function applyArrangement(budget: BudgetDto, arrangement: ElementContainer[]): BudgetDto {
  return {
    ...budget,
    structure: { ...budget.structure, elements: placeElements(budget.structure.elements, arrangement) },
  }
}
