import type { BudgetBuckets } from './budgetMath'
import type { BudgetDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'

export interface ElementMoveItem {
  id: Id
  folderId: Id | null
  position: number
}

// Vue sends only the moved element with its target folder + index; the backend
// renumbers each folder contiguously (restoreElementsOrder).
export function computeElementMove(buckets: BudgetBuckets, activeId: string, overId: string): ElementMoveItem | null {
  const containers: { folderId: Id | null; ids: string[] }[] = [
    ...buckets.withFolder.map((b) => ({ folderId: b.folder!.id as Id | null, ids: b.elements.map((e) => e.id) })),
    { folderId: null, ids: buckets.withoutFolder.elements.map((e) => e.id) },
  ]
  const source = containers.find((c) => c.ids.includes(activeId))
  if (!source) {
    return null
  }
  let target: { folderId: Id | null; ids: string[] } | undefined
  let position: number
  if (overId.startsWith('bfolder:')) {
    const folderId = overId.slice('bfolder:'.length)
    target = containers.find((c) => String(c.folderId) === folderId || (folderId === 'null' && c.folderId === null))
    position = target ? target.ids.length : 0
  } else {
    target = containers.find((c) => c.ids.includes(overId))
    position = target ? target.ids.indexOf(overId) : 0
  }
  if (!target) {
    return null
  }
  if (target === source && source.ids.indexOf(activeId) === position) {
    return null
  }
  return { id: activeId, folderId: target.folderId, position }
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
    insertAt = target ? target.ids.indexOf(overId) : 0
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
  return next
}

export function arrangementItem(arrangement: ElementContainer[], activeId: string): ElementMoveItem | null {
  for (const container of arrangement) {
    const index = container.ids.indexOf(activeId)
    if (index !== -1) {
      return { id: activeId, folderId: container.folderId, position: index }
    }
  }
  return null
}

// Patch element folderId + position to match the arrangement so bucketElements
// (incl. per-folder stats) reproduces the preview; archived elements untouched.
export function applyArrangement(budget: BudgetDto, arrangement: ElementContainer[]): BudgetDto {
  const placement = new Map<string, { folderId: Id | null; position: number }>()
  let position = 0
  for (const container of arrangement) {
    for (const id of container.ids) {
      placement.set(id, { folderId: container.folderId, position })
      position++
    }
  }
  return {
    ...budget,
    structure: {
      ...budget.structure,
      elements: budget.structure.elements.map((el) => {
        const placed = placement.get(el.id)
        return placed ? { ...el, folderId: placed.folderId, position: placed.position } : el
      }),
    },
  }
}
