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
