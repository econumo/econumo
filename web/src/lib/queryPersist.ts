import type { Query } from '@tanstack/react-query'
import { QueryClient } from '@tanstack/react-query'
import { createSyncStoragePersister } from '@tanstack/query-sync-storage-persister'
import { getVersion } from './config'

// Vue parity: the Quasar app keeps every store in localStorage, so a page
// reload starts from cached data and refreshes in the background instead of
// blocking on the boot loader.
export const QUERY_CACHE_KEY = 'econumo.query-cache'

const CACHE_MAX_AGE_MS = 24 * 60 * 60 * 1000

export function createAppQueryClient() {
  return new QueryClient({
    defaultOptions: {
      // gcTime must outlive maxAge, or queries get garbage-collected out of
      // the persisted snapshot before it expires.
      queries: { gcTime: CACHE_MAX_AGE_MS },
    },
  })
}

// query families that must NOT hit localStorage: on-demand full-list fetches
// (search) and month windows (budget dialog) would re-inflate storage
const EPHEMERAL_QUERIES = new Set(['transactionSearch', 'transactionPeriod'])

export function createPersistOptions() {
  return {
    persister: createSyncStoragePersister({ storage: window.localStorage, key: QUERY_CACHE_KEY }),
    maxAge: CACHE_MAX_AGE_MS,
    // a release may change response shapes; never restore across versions
    buster: getVersion(),
    dehydrateOptions: {
      shouldDehydrateQuery: (query: Query) =>
        query.state.status === 'success' && !EPHEMERAL_QUERIES.has(query.queryKey[0] as string),
    },
  }
}

// The cache holds one user's finances — purge it whenever the session owner
// can change (logout, and login as a possibly different user).
export function clearPersistedQueryCache() {
  localStorage.removeItem(QUERY_CACHE_KEY)
}

// After a restore the data may be minutes old (or edited from another
// device), yet staleTime keeps it "fresh" — force a background refresh so
// every boot shows cached data instantly and then catches up, Vue-style.
export function refreshRestoredQueries(queryClient: QueryClient) {
  if (queryClient.getQueryCache().getAll().length > 0) {
    void queryClient.invalidateQueries()
  }
}
