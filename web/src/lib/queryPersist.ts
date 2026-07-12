import axios from 'axios'
import { QueryClient } from '@tanstack/react-query'
import { createSyncStoragePersister } from '@tanstack/query-sync-storage-persister'
import { getVersion } from './config'

// Vue parity: the Quasar app keeps every store in localStorage, so a page
// reload starts from cached data and refreshes in the background instead of
// blocking on the boot loader.
export const QUERY_CACHE_KEY = 'econumo.query-cache'

const CACHE_MAX_AGE_MS = 24 * 60 * 60 * 1000

// An unreachable backend (network error — no HTTP response) retries forever
// with capped backoff, so the boot loader recovers by itself once the backend
// is back. HTTP errors keep the default three attempts.
function retryPolicy(failureCount: number, error: unknown): boolean {
  if (axios.isAxiosError(error) && !error.response) {
    return true
  }
  return failureCount < 3
}

export function createAppQueryClient() {
  return new QueryClient({
    defaultOptions: {
      // gcTime must outlive maxAge, or queries get garbage-collected out of
      // the persisted snapshot before it expires.
      queries: { gcTime: CACHE_MAX_AGE_MS, retry: retryPolicy },
    },
  })
}

export function createPersistOptions() {
  return {
    persister: createSyncStoragePersister({ storage: window.localStorage, key: QUERY_CACHE_KEY }),
    maxAge: CACHE_MAX_AGE_MS,
    // a release may change response shapes; never restore across versions
    buster: getVersion(),
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
