import type { AccountDto } from '@/api/dto/account'
import type { AccountTransactionListDto, StoredTransaction, TransactionDto } from '@/api/dto/transaction'
import type { Bucket } from '@/lib/transactionStore'

// The server rejects more ids than this in one request.
export const MAX_IDS = 200

export interface SyncInput {
  accounts: AccountDto[]
  stored: Map<string, Bucket>
  full: boolean
  fetch: (accountIds?: string[]) => Promise<AccountTransactionListDto>
}

export interface SyncOutput {
  buckets: Map<string, Bucket>
  changed: Bucket[]
  removed: string[]
}

export function hasRevisions(accounts: AccountDto[]): boolean {
  return accounts.every((a) => typeof a.transactionsRevision === 'number')
}

export async function syncBuckets(input: SyncInput): Promise<SyncOutput> {
  const revisions = new Map(input.accounts.map((a) => [a.id, a.transactionsRevision]))
  const ids = new Set(revisions.keys())
  const stale = input.full ? [...ids] : [...ids].filter((id) => input.stored.get(id)?.revision !== revisions.get(id))

  let requested: string[] = []
  let res: AccountTransactionListDto | null = null
  if (stale.length > 0) {
    const all = input.full || input.stored.size === 0 || stale.length > MAX_IDS
    requested = all ? [...ids] : stale
    res = await input.fetch(all ? undefined : stale)
  }

  const removed = new Set([...input.stored.keys()].filter((id) => !ids.has(id)))
  const changed: Bucket[] = []
  if (res) {
    const rows = new Map<string, StoredTransaction[]>(requested.map((id) => [id, []]))
    for (const t of res.items) {
      rows.get(t.accountId)?.push(t)
      // A transfer belongs to both its accounts' buckets.
      if (t.accountRecipientId && t.accountRecipientId !== t.accountId) {
        rows.get(t.accountRecipientId)?.push(t)
      }
    }
    for (const id of requested) {
      const revision = res.revisions[id]
      if (revision === undefined) {
        removed.add(id)
        continue
      }
      const items = rows.get(id)!
      const authorIds = new Set(items.map((t) => t.authorId))
      changed.push({
        accountId: id,
        revision,
        items,
        authors: res.authors.filter((u) => authorIds.has(u.id)),
      })
    }
  }

  const buckets = new Map(input.stored)
  for (const id of removed) {
    buckets.delete(id)
  }
  for (const bucket of changed) {
    buckets.set(bucket.accountId, bucket)
  }
  return { buckets, changed, removed: [...removed] }
}

export function assemble(buckets: Map<string, Bucket>): TransactionDto[] {
  const seen = new Set<string>()
  const out: TransactionDto[] = []
  for (const bucket of buckets.values()) {
    const authors = new Map(bucket.authors.map((u) => [u.id, u]))
    for (const { authorId, ...rest } of bucket.items) {
      if (seen.has(rest.id)) {
        continue
      }
      seen.add(rest.id)
      out.push({ ...rest, author: authors.get(authorId) ?? { id: authorId, name: '', avatar: '' } })
    }
  }
  return out
}
