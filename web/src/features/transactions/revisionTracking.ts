import type { QueryCacheNotifyEvent, QueryClient } from '@tanstack/react-query'
import * as accountApi from '@/api/account'
import * as transactionApi from '@/api/transaction'
import { getUserData } from '@/api/user'
import type { AccountDto } from '@/api/dto/account'
import type { TransactionDto } from '@/api/dto/transaction'
import type { CurrentUserDto, UserDto } from '@/api/dto/user'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import { isPendingForMe } from '@/features/connections/shared'
import { deleteBuckets, putBuckets, storeEpoch, tryLoadBuckets, type Bucket } from '@/lib/transactionStore'
import { assemble, hasRevisions, syncBuckets } from './sync'

const WRITE_THROUGH_DELAY_MS = 500

interface Landing {
  /** the raw accounts list the sync compared against */
  accounts: AccountDto[]
  revisions: Map<string, number>
}

// Revision of every stored bucket as this page knows it.
let revisions = new Map<string, number>()
// Accounts whose bucket revision moved through noteOwnWrite and must be written.
const marked = new Set<string>()
// The rows each bucket held when it was last synced or written; a row object
// that is not in here was changed by a local patch.
let baseline = new Map<string, TransactionDto[]>()
let dirty = false
let writeTimer: ReturnType<typeof setTimeout> | null = null
// A sync's outcome takes effect only when its result actually lands in the
// cache: a result that is cancelled or overwritten must not move the revisions.
let landing: Landing | null = null
let fullReload = false
// Sequence number of the newest run, and of the newest run that was a full reload.
let runSeq = 0
let fullSeq = 0

function isKey(key: readonly unknown[], wanted: readonly unknown[]): boolean {
  return key.length === wanted.length && key[0] === wanted[0]
}

function visibleAccounts(queryClient: QueryClient, raw: AccountDto[]): AccountDto[] {
  const userId = queryClient.getQueryData<CurrentUserDto>(queryKeys.user)?.id
  return raw.filter((a) => !isPendingForMe(a, userId))
}

function revisionsDiffer(accounts: AccountDto[]): boolean {
  return accounts.length !== revisions.size || accounts.some((a) => revisions.get(a.id) !== a.transactionsRevision)
}

function followAccounts(queryClient: QueryClient, raw: AccountDto[] | undefined): void {
  if (!raw) {
    return
  }
  const accounts = visibleAccounts(queryClient, raw)
  if (hasRevisions(accounts) && revisionsDiffer(accounts)) {
    void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
  }
}

function groupByAccount(rows: TransactionDto[]): Map<string, TransactionDto[]> {
  const out = new Map<string, TransactionDto[]>()
  const add = (id: string, row: TransactionDto) => {
    const list = out.get(id)
    if (list) {
      list.push(row)
    } else {
      out.set(id, [row])
    }
  }
  for (const row of rows) {
    add(row.accountId, row)
    if (row.accountRecipientId && row.accountRecipientId !== row.accountId) {
      add(row.accountRecipientId, row)
    }
  }
  return out
}

function sameRows(a: TransactionDto[], b: TransactionDto[]): boolean {
  return a.length === b.length && a.every((row, i) => row === b[i])
}

function toBucket(accountId: string, revision: number, rows: TransactionDto[]): Bucket {
  const authors = new Map<string, UserDto>()
  const items = rows.map(({ author, ...rest }) => {
    authors.set(author.id, author)
    return { ...rest, authorId: author.id }
  })
  return { accountId, revision, items, authors: [...authors.values()] }
}

function flushWriteThrough(queryClient: QueryClient): Promise<void> {
  if (writeTimer !== null) {
    clearTimeout(writeTimer)
    writeTimer = null
  }
  if (!dirty) {
    return Promise.resolve()
  }
  dirty = false
  const data = queryClient.getQueryData<TransactionDto[]>(queryKeys.transactions)
  const userId = queryClient.getQueryData<CurrentUserDto>(queryKeys.user)?.id
  if (!data || !userId) {
    marked.clear()
    return Promise.resolve()
  }
  const grouped = groupByAccount(data)
  const changed: Bucket[] = []
  // only accounts with a stored bucket: anything else is downloaded by the next sync
  for (const [accountId, revision] of revisions) {
    const rows = grouped.get(accountId) ?? []
    // an account with no rows has no baseline entry either
    if (!marked.has(accountId) && sameRows(baseline.get(accountId) ?? [], rows)) {
      continue
    }
    changed.push(toBucket(accountId, revision, rows))
    baseline.set(accountId, rows)
  }
  marked.clear()
  return putBuckets(userId, changed)
}

function scheduleWriteThrough(queryClient: QueryClient): void {
  dirty = true
  if (writeTimer !== null) {
    clearTimeout(writeTimer)
  }
  writeTimer = setTimeout(() => void flushWriteThrough(queryClient), WRITE_THROUGH_DELAY_MS)
}

function onTransactionsData(queryClient: QueryClient, data: TransactionDto[] | undefined, manual: boolean): void {
  if (!data) {
    return
  }
  if (manual) {
    scheduleWriteThrough(queryClient)
    return
  }
  baseline = groupByAccount(data)
  const outcome = landing
  landing = null
  if (!outcome) {
    return
  }
  revisions = outcome.revisions
  marked.clear()
  // the accounts moved while the sync ran (a local write, a refetch): compare again
  const current = queryClient.getQueryData<AccountDto[]>(queryKeys.accounts)
  if (current !== outcome.accounts) {
    followAccounts(queryClient, current)
  }
}

function onCacheEvent(queryClient: QueryClient, event: QueryCacheNotifyEvent): void {
  const key = event.query.queryKey
  if (event.type === 'updated' && event.action.type === 'success') {
    if (isKey(key, queryKeys.accounts)) {
      followAccounts(queryClient, event.action.data as AccountDto[] | undefined)
    } else if (isKey(key, queryKeys.transactions)) {
      onTransactionsData(queryClient, event.action.data as TransactionDto[] | undefined, event.action.manual === true)
    }
  } else if (event.type === 'added' && isKey(key, queryKeys.transactions)) {
    // a query restored with data starts out as what is stored
    const data = event.query.state.data as TransactionDto[] | undefined
    if (data) {
      baseline = groupByAccount(data)
    }
  }
}

// Touches no module state when called: queryClient.ts installs this while the
// api -> queryClient import cycle may still be evaluating this module.
export function installTransactionTracking(queryClient: QueryClient): () => void {
  return queryClient.getQueryCache().subscribe((event) => onCacheEvent(queryClient, event))
}

/**
 * Records a write this client made. The server moves a touched account's
 * revision by exactly one per write, so `stored + 1` means the local patch is
 * the whole change and the account need not be downloaded; anything else
 * leaves it stale for the next sync. Call before the response's accounts reach
 * the cache, or the accounts subscription sees the move first.
 */
export function noteOwnWrite(accounts: AccountDto[], touchedAccountIds: (string | null | undefined)[]): void {
  for (const id of new Set(touchedAccountIds)) {
    if (!id) {
      continue
    }
    const known = revisions.get(id)
    const next = accounts.find((a) => a.id === id)?.transactionsRevision
    if (known !== undefined && next === known + 1) {
      revisions.set(id, next)
      marked.add(id)
      dirty = true
    }
  }
}

/**
 * Call before patching the transactions list for a write this client made.
 * Cancels a sync that read the rows before the write. Returns false when there
 * is no list yet (the first load was in flight): patching would turn the one
 * row into the whole list, so a fresh load is started instead.
 */
export function beginLocalWrite(queryClient: QueryClient): boolean {
  void queryClient.cancelQueries({ queryKey: queryKeys.transactions })
  if (queryClient.getQueryData(queryKeys.transactions) !== undefined) {
    return true
  }
  void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
  return false
}

export function requestFullReload(): void {
  fullReload = true
}

export function consumeFullReload(): boolean {
  const full = fullReload
  fullReload = false
  return full
}

/**
 * Adopts restored buckets. `rows` must be the very list the restored query
 * holds: the write-through tells changed rows by identity, and a restore into
 * a query a component already created raises no event this module sees.
 */
export function seedRevisions(buckets: Map<string, Bucket>, rows: TransactionDto[]): void {
  revisions = new Map([...buckets].map(([id, b]) => [id, b.revision]))
  baseline = groupByAccount(rows)
}

export function resetRevisions(): void {
  if (writeTimer !== null) {
    clearTimeout(writeTimer)
    writeTimer = null
  }
  revisions = new Map()
  marked.clear()
  baseline = new Map()
  dirty = false
  landing = null
  fullReload = false
}

function inBackground(write: Promise<void>): void {
  write.catch((e: unknown) => console.warn('Could not store transactions', e))
}

export async function transactionsQueryFn(
  queryClient: QueryClient,
  opts: { full: boolean; signal?: AbortSignal },
): Promise<TransactionDto[]> {
  const seq = ++runSeq
  if (opts.full) {
    fullSeq = seq
  }
  // A cancelled run (a local write, or a newer sync) must neither download nor
  // land. A cancelled full reload is re-run: a newer sync started before this
  // one noticed, so it did not see the request. A newer full run already is
  // that re-run; queueing another would make two full runs cancel each other
  // for good.
  const superseded = () => {
    if (!opts.signal?.aborted) {
      return false
    }
    if (opts.full && fullSeq === seq) {
      fullReload = true
      void queryClient.invalidateQueries({ queryKey: queryKeys.transactions })
    }
    return true
  }
  // a clear (logout, login) while this runs makes every store call below a no-op
  const at = storeEpoch()
  const user = await queryClient.fetchQuery({ queryKey: queryKeys.user, queryFn: getUserData, staleTime: TEN_MINUTES })
  const raw = await queryClient.fetchQuery({ queryKey: queryKeys.accounts, queryFn: accountApi.getAccountList, staleTime: TEN_MINUTES })
  if (superseded()) {
    return []
  }
  const accounts = raw.filter((a) => !isPendingForMe(a, user.id))
  if (!hasRevisions(accounts)) {
    const list = await transactionApi.getTransactionList()
    if (!opts.signal?.aborted) {
      landing = { accounts: raw, revisions: new Map() }
    }
    return list
  }
  // Writes are never awaited: a write that does not complete on some device
  // must not hold the list. IndexedDB runs readwrite transactions on the same
  // stores in the order they were created, so a load started after a write
  // still sees it. Patches not yet written would otherwise be replaced by the
  // older stored rows.
  inBackground(flushWriteThrough(queryClient))
  // null: the store did not answer in time — download everything, persist nothing
  const stored = await tryLoadBuckets(user.id, at)
  if (superseded()) {
    return []
  }
  const out = await syncBuckets({
    accounts,
    stored: stored ?? new Map(),
    full: opts.full,
    fetch: (ids) => transactionApi.getAccountTransactionList(ids),
  })
  if (superseded()) {
    return []
  }
  if (stored) {
    inBackground(putBuckets(user.id, out.changed, at))
    inBackground(deleteBuckets(out.removed, at))
  }
  if (opts.signal?.aborted) {
    return []
  }
  landing = { accounts: raw, revisions: new Map([...out.buckets].map(([id, b]) => [id, b.revision])) }
  return assemble(out.buckets)
}
