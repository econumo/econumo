import { hashKey, type QueryClient, type QueryState } from '@tanstack/react-query'
import type { PersistedClient, Persister } from '@tanstack/react-query-persist-client'
import { createSyncStoragePersister } from '@tanstack/query-sync-storage-persister'
import type { AccountDto } from '@/api/dto/account'
import type { TransactionDto } from '@/api/dto/transaction'
import type { CurrentUserDto } from '@/api/dto/user'
import { queryKeys } from '@/app/queryKeys'
import { isPendingForMe } from '@/features/connections/shared'
import { resetRevisions, seedRevisions } from '@/features/transactions/revisionTracking'
import { assemble, hasRevisions } from '@/features/transactions/sync'
import { CACHE_MAX_AGE_MS } from './appQueryClient'
import { getVersion } from './config'
import { getSessionUserId, hasToken } from './storage'
import { clearTransactions, deleteBuckets, tryLoadStored } from './transactionStore'

// Vue parity: the Quasar app keeps every store in localStorage, so a page
// reload starts from cached data and refreshes in the background instead of
// blocking on the boot loader.
export const QUERY_CACHE_KEY = 'econumo.query-cache'

function snapshotData<T>(snapshot: PersistedClient, key: readonly unknown[]): T | undefined {
  const hash = hashKey(key)
  return snapshot.clientState.queries.find((q) => q.queryHash === hash)?.state.data as T | undefined
}

interface Seed {
  rows: TransactionDto[]
  savedAt: number | undefined
}

// `snapshot`: the restored cache of this release, fresh or expired, if any.
// `owner`: the user the session token was issued to; the store's meta must
// name the same one, so it seeds without the snapshot too.
async function loadSeed(snapshot: PersistedClient | undefined, owner: string): Promise<Seed | null> {
  const user = snapshot && snapshotData<CurrentUserDto>(snapshot, queryKeys.user)
  const raw = snapshot && snapshotData<AccountDto[]>(snapshot, queryKeys.accounts)
  if (user && user.id !== owner) {
    return null
  }
  const stored = await tryLoadStored()
  if (!stored || stored.userId !== owner) {
    return null
  }
  let buckets = stored.buckets
  if (raw) {
    const accounts = raw.filter((a) => !isPendingForMe(a, stored.userId))
    // an older server: nothing is synced by revision
    if (!hasRevisions(accounts)) {
      return null
    }
    buckets = new Map()
    for (const a of accounts) {
      const bucket = stored.buckets.get(a.id)
      if (bucket) {
        buckets.set(a.id, bucket)
      }
    }
    // The sync works from what is seeded here and never sees the rest of the
    // store, so a bucket of an account that is gone (a share revoked, an account
    // deleted) would stay on disk.
    void deleteBuckets([...stored.buckets.keys()].filter((id) => !buckets.has(id)))
  }
  // without the snapshot's accounts every stored bucket is shown; the sync
  // that follows drops the accounts that are gone
  if (buckets.size === 0) {
    return null
  }
  const rows = assemble(buckets)
  seedRevisions(buckets, rows)
  return { rows, savedAt: stored.savedAt }
}

// Putting the transactions back as a restored query (rather than setQueryData
// after the restore) keeps the boot loader suppressed, and lands as a restore,
// not as a local edit to write back.
function withTransactions(client: PersistedClient, rows: TransactionDto[], updatedAt: number): PersistedClient {
  const state: QueryState<TransactionDto[], Error> = {
    data: rows,
    dataUpdateCount: 1,
    dataUpdatedAt: updatedAt,
    error: null,
    errorUpdateCount: 0,
    errorUpdatedAt: 0,
    fetchFailureCount: 0,
    fetchFailureReason: null,
    fetchMeta: null,
    isInvalidated: false,
    status: 'success',
    fetchStatus: 'idle',
  }
  const hash = hashKey(queryKeys.transactions)
  return {
    ...client,
    clientState: {
      ...client.clientState,
      queries: [
        ...client.clientState.queries.filter((q) => q.queryHash !== hash),
        { queryKey: queryKeys.transactions, queryHash: hash, state },
      ],
    },
  }
}

function withTransactionRestore(inner: Persister, maxAge: number, buster: string): Persister {
  return {
    persistClient: (client) => inner.persistClient(client),
    removeClient: () => inner.removeClient(),
    restoreClient: async () => {
      const snapshot = await inner.restoreClient()
      // signed out: the next sign-in clears the store anyway
      const owner = hasToken() ? getSessionUserId() : null
      if (!owner) {
        return snapshot
      }
      // a snapshot of another release must contribute nothing: response shapes may differ
      const usable = snapshot?.timestamp && snapshot.buster === buster ? snapshot : undefined
      const seed = await loadSeed(usable, owner)
      if (!seed) {
        return snapshot
      }
      if (usable && Date.now() - usable.timestamp <= maxAge) {
        return withTransactions(usable, seed.rows, usable.timestamp)
      }
      if (usable) {
        // Past maxAge the provider would discard the snapshot, and without the
        // accounts and the rest of it the boot loader would cover the seeded
        // list (an offline start after a day away). Re-stamped, it is restored
        // and then refreshed like any other.
        return withTransactions({ ...usable, timestamp: Date.now() }, seed.rows, seed.savedAt ?? 0)
      }
      if (snapshot) {
        await inner.removeClient()
      }
      return withTransactions({ buster, timestamp: Date.now(), clientState: { queries: [], mutations: [] } }, seed.rows, seed.savedAt ?? 0)
    },
  }
}

// throttleTime: tests pass 0 — the persister's 1 s default equals waitFor's
// default timeout, so asserting on the written snapshot would race it.
export function createPersistOptions({ throttleTime }: { throttleTime?: number } = {}) {
  // a release may change response shapes; never restore across versions
  const buster = getVersion()
  return {
    persister: withTransactionRestore(
      createSyncStoragePersister({ storage: window.localStorage, key: QUERY_CACHE_KEY, throttleTime }),
      CACHE_MAX_AGE_MS,
      buster,
    ),
    maxAge: CACHE_MAX_AGE_MS,
    buster,
    dehydrateOptions: {
      // an authorization request carries single-use OAuth state and is only
      // meaningful for the page that asked; transactions are kept in IndexedDB
      shouldDehydrateQuery: (query: { queryKey: readonly unknown[]; state: { status: string } }) =>
        query.queryKey[0] !== 'authserver' && query.queryKey[0] !== queryKeys.transactions[0] && query.state.status === 'success',
    },
  }
}

// The cache holds one user's finances — purge it whenever the session owner
// can change (logout, and login as a possibly different user).
export async function clearPersistedQueryCache(): Promise<void> {
  localStorage.removeItem(QUERY_CACHE_KEY)
  resetRevisions()
  await clearTransactions()
}

// After a restore the data may be minutes old (or edited from another
// device), yet staleTime keeps it "fresh" — force a background refresh so
// every boot shows cached data instantly and then catches up, Vue-style.
export function refreshRestoredQueries(queryClient: QueryClient) {
  if (queryClient.getQueryCache().getAll().length > 0) {
    void queryClient.invalidateQueries()
  }
}
