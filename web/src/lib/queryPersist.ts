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
import { clearTransactions, deleteBuckets, loadBuckets, type Bucket } from './transactionStore'

// Vue parity: the Quasar app keeps every store in localStorage, so a page
// reload starts from cached data and refreshes in the background instead of
// blocking on the boot loader.
export const QUERY_CACHE_KEY = 'econumo.query-cache'

function snapshotData<T>(snapshot: PersistedClient, key: readonly unknown[]): T | undefined {
  const hash = hashKey(key)
  return snapshot.clientState.queries.find((q) => q.queryHash === hash)?.state.data as T | undefined
}

// Transactions live in IndexedDB, not in the snapshot. Putting them back as a
// restored query (rather than setQueryData after the restore) keeps the boot
// loader suppressed, and lands as a restore, not as a local edit to write back.
async function withStoredTransactions(snapshot: PersistedClient): Promise<PersistedClient> {
  const user = snapshotData<CurrentUserDto>(snapshot, queryKeys.user)
  const raw = snapshotData<AccountDto[]>(snapshot, queryKeys.accounts)
  if (!user || !raw) {
    return snapshot
  }
  const accounts = raw.filter((a) => !isPendingForMe(a, user.id))
  if (!hasRevisions(accounts)) {
    return snapshot
  }
  // the store wipes itself when it belongs to someone else
  const stored = await loadBuckets(user.id)
  const buckets = new Map<string, Bucket>()
  for (const a of accounts) {
    const bucket = stored.get(a.id)
    if (bucket) {
      buckets.set(a.id, bucket)
    }
  }
  if (buckets.size === 0) {
    return snapshot
  }
  // The sync works from what is seeded here and never sees the rest of the
  // store, so a bucket of an account that is gone (a share revoked, an account
  // deleted) would stay on disk.
  void deleteBuckets([...stored.keys()].filter((id) => !buckets.has(id)))
  const rows = assemble(buckets)
  seedRevisions(buckets, rows)
  const state: QueryState<TransactionDto[], Error> = {
    data: rows,
    dataUpdateCount: 1,
    dataUpdatedAt: snapshot.timestamp,
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
    ...snapshot,
    clientState: {
      ...snapshot.clientState,
      queries: [
        ...snapshot.clientState.queries.filter((q) => q.queryHash !== hash),
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
      // a snapshot about to be discarded must not seed anything
      if (!snapshot?.timestamp || Date.now() - snapshot.timestamp > maxAge || snapshot.buster !== buster) {
        return snapshot
      }
      return withStoredTransactions(snapshot)
    },
  }
}

export function createPersistOptions() {
  // a release may change response shapes; never restore across versions
  const buster = getVersion()
  return {
    persister: withTransactionRestore(
      createSyncStoragePersister({ storage: window.localStorage, key: QUERY_CACHE_KEY }),
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
