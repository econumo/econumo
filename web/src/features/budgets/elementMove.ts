import { pointerWithin, rectIntersection } from '@dnd-kit/core'
import type { CollisionDetection } from '@dnd-kit/core'
import type { SortingStrategy } from '@dnd-kit/sortable'
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

/** the budget without one element, wherever it sits (top level or in an envelope):
 *  a category on its way into or out of an envelope hides until the refetch shows
 *  it in its new place */
export function withoutElement(budget: BudgetDto, id: string): BudgetDto {
  const elements = budget.structure.elements
    .filter((el) => el.id !== id)
    .map((el) => (el.children.some((c) => c.id === id) ? { ...el, children: el.children.filter((c) => c.id !== id) } : el))
  return { ...budget, structure: { ...budget.structure, elements } }
}

/** the drop zone of an unfolded envelope's category list: `benv:<envelope id>` */
export const ENVELOPE_DROP = 'benv:'
/** the drop zone of a folded envelope: the middle band of its row */
export const ENVELOPE_HEAD_DROP = 'benvh:'

/** the envelope a drop target puts the dragged category into, or null */
export function envelopeOfDrop(overId: string): string | null {
  for (const prefix of [ENVELOPE_DROP, ENVELOPE_HEAD_DROP]) {
    if (overId.startsWith(prefix)) {
      return overId.slice(prefix.length)
    }
  }
  return null
}

// Rows are nested inside their section droppable, and the dragged row itself
// travels under the pointer (its own rect always wins a pointer test) — so:
// ignore the active row, prefer an envelope's category list the pointer is in
// (when the dragged item may go into it), then whatever OTHER row the pointer is
// inside, and fall back to sections (empty folders, gaps between rows).
export function envelopeCollisions(canEnter: (activeId: string, envelopeId: string) => boolean): CollisionDetection {
  return (args) => {
    const activeId = String(args.active.id)
    const within = pointerWithin(args).filter((c) => c.id !== args.active.id)
    const envelope = within.find((c) => {
      const envelopeId = envelopeOfDrop(String(c.id))
      return envelopeId !== null && canEnter(activeId, envelopeId)
    })
    if (envelope) {
      return [envelope]
    }
    const candidates = (within.length > 0 ? within : rectIntersection(args)).filter(
      (c) => c.id !== args.active.id && envelopeOfDrop(String(c.id)) === null,
    )
    const row = candidates.find((c) => !String(c.id).startsWith('bfolder:'))
    return row ? [row] : candidates
  }
}

export const preferRowCollisions: CollisionDetection = envelopeCollisions(() => false)

// where the dragged item would land: the moved arrangement and the wire item. A
// category coming out of an envelope is in no container yet, so it starts in a
// stand-in one.
const OUT_OF_ENVELOPE = '__envelope__' as Id
function landing(
  base: ElementContainer[],
  activeId: string,
  overId: string,
  fromEnvelope: boolean,
): { final: ElementContainer[]; item: ElementMoveItem } | null {
  if (fromEnvelope) {
    const final = moveElementInArrangement([...base, { folderId: OUT_OF_ENVELOPE, ids: [activeId] }], activeId, overId)
    const item = arrangementItem(final, activeId)
    return item && item.folderId !== OUT_OF_ENVELOPE ? { final, item } : null
  }
  const final = moveElementInArrangement(base, activeId, overId)
  const item = arrangementItem(final, activeId)
  const before = arrangementItem(base, activeId)
  if (!item || (before && before.folderId === item.folderId && before.position === item.position)) {
    return null
  }
  return { final, item }
}

/** Where a category dragged out of an envelope lands: the same placement a row
 *  dropped on `overId` gets. null when it lands nowhere. */
export function placeFromEnvelope(base: ElementContainer[], activeId: string, overId: string): ElementMoveItem | null {
  return landing(base, activeId, overId, true)?.item ?? null
}

/** The insertion line while dragging: before or after a row (top level), under a
 *  folder's header (an empty or folded folder), or under an envelope's categories
 *  (the drop puts the category into it). */
export type DropIndicator =
  | { kind: 'row'; id: string; edge: 'before' | 'after' }
  | { kind: 'folder'; folderId: Id | null }
  | { kind: 'envelope'; envelopeId: string }

export function dropIndicatorFor(
  base: ElementContainer[],
  activeId: string,
  overId: string,
  { fromEnvelope, isFolded }: { fromEnvelope: boolean; isFolded: (folderId: Id | null) => boolean },
): DropIndicator | null {
  const envelopeId = envelopeOfDrop(overId)
  if (envelopeId !== null) {
    return { kind: 'envelope', envelopeId }
  }
  const landed = landing(base, activeId, overId, fromEnvelope)
  if (!landed) {
    return null
  }
  const { final, item } = landed
  if (isFolded(item.folderId)) {
    return { kind: 'folder', folderId: item.folderId }
  }
  if (item.afterId) {
    return { kind: 'row', id: item.afterId, edge: 'after' }
  }
  const next = final.find((c) => c.folderId === item.folderId)?.ids.find((id) => id !== activeId)
  return next ? { kind: 'row', id: next, edge: 'before' } : { kind: 'folder', folderId: item.folderId }
}

/** Rows stay put while a row is dragged: a floating copy follows the pointer and
 *  the insertion line shows where it lands, inside a folder and across folders
 *  alike. */
export const noShift: SortingStrategy = () => null
