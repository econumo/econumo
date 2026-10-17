import axios from 'axios'
import { QueryClient } from '@tanstack/react-query'

// A leaf module on purpose: queryPersist imports the transaction sync, which
// reaches the api client and through it the app's queryClient singleton, so
// the singleton's factory must not live behind that cycle.
export const CACHE_MAX_AGE_MS = 24 * 60 * 60 * 1000

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
      // gcTime must outlive the persister's maxAge, or queries get
      // garbage-collected out of the persisted snapshot before it expires.
      queries: { gcTime: CACHE_MAX_AGE_MS, retry: retryPolicy },
    },
  })
}
