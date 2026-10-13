import type { AccountDto } from '@/api/dto/account'
import type { AccountTransactionListDto, StoredTransaction, TransactionDto } from '@/api/dto/transaction'
import type { Bucket } from '@/lib/transactionStore'

// The server rejects more ids than this in one request.
export const MAX_IDS = 200

// Account downloads in flight at once when they go one by one.
const PARALLEL = 2

export interface SyncInput {
  accounts: AccountDto[]
  stored: Map<string, Bucket>
  full: boolean
  fetch: (accountIds: string[]) => Promise<AccountTransactionListDto>
  /** Every downloaded bucket, once, as soon as it arrives. */
  persist?: (buckets: Bucket[]) => void
  signal?: AbortSignal
}

export interface SyncOutput {
  buckets: Map<string, Bucket>
  changed: Bucket[]
  removed: string[]
}

interface Download {
  changed: Bucket[]
  gone: string[]
}

export function hasRevisions(accounts: AccountDto[]): boolean {
  return accounts.every((a) => typeof a.transactionsRevision === 'number')
}

function toBuckets(res: AccountTransactionListDto, requested: string[]): Download {
  const rows = new Map<string, StoredTransaction[]>(requested.map((id) => [id, []]))
  for (const t of res.items) {
    rows.get(t.accountId)?.push(t)
    // A transfer belongs to both its accounts' buckets.
    if (t.accountRecipientId && t.accountRecipientId !== t.accountId) {
      rows.get(t.accountRecipientId)?.push(t)
    }
  }
  const out: Download = { changed: [], gone: [] }
  for (const id of requested) {
    const revision = res.revisions[id]
    if (revision === undefined) {
      out.gone.push(id)
      continue
    }
    const items = rows.get(id)!
    const authorIds = new Set(items.map((t) => t.authorId))
    out.changed.push({ accountId: id, revision, items, authors: res.authors.filter((u) => authorIds.has(u.id)) })
  }
  return out
}

// A failure stops new requests but lets the one in flight land and persist,
// so the next sync has less left to download.
async function downloadEach(ids: string[], input: SyncInput): Promise<Download> {
  const out: Download = { changed: [], gone: [] }
  const failures: unknown[] = []
  let next = 0
  const worker = async () => {
    while (failures.length === 0 && !input.signal?.aborted && next < ids.length) {
      const id = ids[next++]
      try {
        const part = toBuckets(await input.fetch([id]), [id])
        out.changed.push(...part.changed)
        out.gone.push(...part.gone)
        input.persist?.(part.changed)
      } catch (error) {
        failures.push(error)
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(PARALLEL, ids.length) }, worker))
  if (failures.length > 0) {
    throw failures[0]
  }
  if (input.signal?.aborted) {
    throw new DOMException('The sync was cancelled', 'AbortError')
  }
  return out
}

export async function syncBuckets(input: SyncInput): Promise<SyncOutput> {
  const revisions = new Map(input.accounts.map((a) => [a.id, a.transactionsRevision]))
  const ids = [...revisions.keys()]
  const stale = input.full ? ids : ids.filter((id) => input.stored.get(id)?.revision !== revisions.get(id))

  // A whole account history (no bucket yet, or a full reload) goes one per
  // request, stored on arrival, so an interrupted load resumes; changed
  // buckets go in one request unless more than MAX_IDS changed.
  const changed = input.full ? [] : stale.filter((id) => input.stored.has(id))
  const batched = changed.length > MAX_IDS ? [] : changed
  const each = batched.length === 0 ? stale : stale.filter((id) => !input.stored.has(id))
  const download: Download = { changed: [], gone: [] }
  if (batched.length > 0) {
    const part = toBuckets(await input.fetch(batched), batched)
    input.persist?.(part.changed)
    download.changed.push(...part.changed)
    download.gone.push(...part.gone)
  }
  if (each.length > 0) {
    const part = await downloadEach(each, input)
    download.changed.push(...part.changed)
    download.gone.push(...part.gone)
  }

  const removed = new Set([...input.stored.keys()].filter((id) => !revisions.has(id)))
  for (const id of download.gone) {
    removed.add(id)
  }
  const buckets = new Map(input.stored)
  for (const id of removed) {
    buckets.delete(id)
  }
  for (const bucket of download.changed) {
    buckets.set(bucket.accountId, bucket)
  }
  return { buckets, changed: download.changed, removed: [...removed] }
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
