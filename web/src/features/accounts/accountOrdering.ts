import { closestCenter, pointerWithin } from '@dnd-kit/core'
import type { CollisionDetection } from '@dnd-kit/core'
import type { AccountDto } from '@/api/dto/account'

export interface FolderBucket {
  folderId: string | null
  accountIds: string[]
}

export function bucketsFromAccounts(accounts: AccountDto[], folderIds: string[]): FolderBucket[] {
  const ordered = [...accounts].sort((a, b) => a.position - b.position)
  const buckets: FolderBucket[] = folderIds.map((folderId) => ({
    folderId,
    accountIds: ordered.filter((a) => a.folderId === folderId).map((a) => a.id),
  }))
  const folderless = ordered.filter((a) => !a.folderId || !folderIds.includes(a.folderId))
  if (folderless.length > 0) {
    buckets.push({ folderId: null, accountIds: folderless.map((a) => a.id) })
  }
  return buckets
}

// Move an account within/between folder buckets. `overId` is another account id,
// a folder container id of the form `folder:<id>` (empty-folder drop), or a bare
// folder id (the folder's own sortable registers a droppable too).
export function moveAccount(buckets: FolderBucket[], activeId: string, overId: string): FolderBucket[] {
  const next = buckets.map((b) => ({ ...b, accountIds: [...b.accountIds] }))
  const source = next.find((b) => b.accountIds.includes(activeId))
  if (!source) {
    return buckets
  }
  let target: FolderBucket | undefined
  let insertAt: number
  const containerId = overId.startsWith('folder:') ? overId.slice('folder:'.length) : overId
  const container = next.find((b) => b.folderId === containerId)
  if (container) {
    target = container
    insertAt = container.accountIds.length
  } else {
    target = next.find((b) => b.accountIds.includes(overId))
    insertAt = target ? target.accountIds.indexOf(overId) : 0
  }
  if (!target) {
    return buckets
  }
  const fromIndex = source.accountIds.indexOf(activeId)
  source.accountIds.splice(fromIndex, 1)
  if (target === source && fromIndex < insertAt) {
    // account removed before the insertion point shifts it left
    insertAt = Math.min(insertAt, target.accountIds.length)
  }
  target.accountIds.splice(insertAt, 0, activeId)
  return next
}

// Describe a drag as the relative move the server expects: which account moved,
// which folder it landed in, and which account it now sits after (null = first).
// The bucket layout is the source of truth for all three.
export function accountMoveFrom(buckets: FolderBucket[], movedId: string): AccountMove | null {
  for (const bucket of buckets) {
    const index = bucket.accountIds.indexOf(movedId)
    if (index === -1) {
      continue
    }
    return {
      id: movedId,
      folderId: bucket.folderId,
      afterId: index > 0 ? bucket.accountIds[index - 1] : null,
    }
  }
  return null
}

export interface AccountMove {
  id: string
  folderId: string | null
  afterId: string | null
}

// Collision detection for an account drag. The live preview moves the dragged
// row on every drag-over, which reflows the list, and the droppables are
// re-measured from that new layout. Under closestCenter a folder section (a
// droppable spanning the whole folder, twice: its sortable and its `folder:`
// container) competes with the rows inside it, so a move could make the
// section the closest target, the "end of folder" move then made the row the
// closest again, and the two alternated without the pointer moving until React
// aborted with "Maximum update depth exceeded". Only another account row under
// the pointer or a folder with no other account is a target; anything else
// (a folder header, the dragged row's own slot) is no target, so the preview
// holds still.
export function accountCollisions(buckets: FolderBucket[]): CollisionDetection {
  const accountIds = new Set(buckets.flatMap((b) => b.accountIds))
  return (args) => {
    const activeId = String(args.active.id)
    const isOtherRow = (id: string) => id !== activeId && accountIds.has(id)
    const isEmptyFolder = (id: string) => {
      if (!id.startsWith('folder:')) {
        return false
      }
      const bucket = buckets.find((b) => b.folderId === id.slice('folder:'.length))
      return !bucket || bucket.accountIds.every((a) => a === activeId)
    }
    if (!args.pointerCoordinates) {
      // keyboard drags carry no pointer: nearest valid target by distance
      const nearest = closestCenter(args).find((c) => isOtherRow(String(c.id)) || isEmptyFolder(String(c.id)))
      return nearest ? [nearest] : []
    }
    const hits = pointerWithin(args)
    const target = hits.find((c) => isOtherRow(String(c.id))) ?? hits.find((c) => isEmptyFolder(String(c.id)))
    return target ? [target] : []
  }
}
