import type { BudgetBuckets } from './budgetMath'
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
