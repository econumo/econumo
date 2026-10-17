import type { AccountDto } from '@/api/dto/account'
import type { AccountTransactionListDto, StoredTransaction, TransactionDto } from '@/api/dto/transaction'
import { RELOAD_REVISION, type Bucket } from '@/lib/transactionStore'

// The server rejects more ids than this in one request.
export const MAX_IDS = 200

// Whole-account downloads in flight at once. More would only queue: a SQLite
// server answers them one at a time.
const PARALLEL = 2

// Rows per request when small accounts share one; an account this big or
// bigger goes alone.
export const BATCH_ROWS = 2000

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

/**
 * Groups whole-account downloads into requests by size: largest first, each
 * into the first request it fits (up to BATCH_ROWS rows and MAX_IDS ids), so
 * small accounts share a request and a large one goes alone. An account of
 * unknown size (an older server) goes alone, as before.
 */
export function packDownloads(ids: string[], sizeOf: (id: string) => number | undefined): string[][] {
  const order = new Map(ids.map((id, i) => [id, i]))
  const byOrder = (a: string, b: string) => order.get(a)! - order.get(b)!
  const unknown = ids.filter((id) => sizeOf(id) === undefined)
  const known = ids.filter((id) => sizeOf(id) !== undefined).sort((a, b) => sizeOf(b)! - sizeOf(a)! || byOrder(a, b))
  const batches: { ids: string[]; rows: number }[] = []
  for (const id of known) {
    const rows = sizeOf(id)!
    const fit = rows < BATCH_ROWS ? batches.find((b) => b.rows + rows <= BATCH_ROWS && b.ids.length < MAX_IDS) : undefined
    if (fit) {
      fit.ids.push(id)
      fit.rows += rows
    } else {
      // a large account's request takes no other account, not even an empty one
      batches.push({ ids: [id], rows: rows < BATCH_ROWS ? rows : Infinity })
    }
  }
  return [...unknown.map((id) => [id]), ...batches.map((b) => b.ids.sort(byOrder))]
}

// A failure stops new requests but lets the one in flight land and persist,
// so the next sync has less left to download.
async function downloadEach(batches: string[][], input: SyncInput): Promise<Download> {
  const out: Download = { changed: [], gone: [] }
  const failures: unknown[] = []
  let next = 0
  const worker = async () => {
    while (failures.length === 0 && !input.signal?.aborted && next < batches.length) {
      const ids = batches[next++]
      try {
        const part = toBuckets(await input.fetch(ids), ids)
        out.changed.push(...part.changed)
        out.gone.push(...part.gone)
        input.persist?.(part.changed)
      } catch (error) {
        failures.push(error)
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(PARALLEL, batches.length) }, worker))
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

  // A whole account history (no bucket yet, one a full reload marked, or a
  // full reload) goes in requests packed by size, each stored on arrival, so
  // an interrupted load resumes; changed buckets go in one request unless more
  // than MAX_IDS changed.
  const counts = new Map(input.accounts.map((a) => [a.id, a.transactionsCount]))
  const whole = (id: string) => (input.stored.get(id)?.revision ?? RELOAD_REVISION) === RELOAD_REVISION
  const changed = input.full ? [] : stale.filter((id) => !whole(id))
  const batched = changed.length > MAX_IDS ? [] : changed
  const each = batched.length === 0 ? stale : stale.filter(whole)
  const download: Download = { changed: [], gone: [] }
  if (batched.length > 0) {
    const part = toBuckets(await input.fetch(batched), batched)
    input.persist?.(part.changed)
    download.changed.push(...part.changed)
    download.gone.push(...part.gone)
  }
  if (each.length > 0) {
    const part = await downloadEach(packDownloads(each, (id) => counts.get(id)), input)
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
