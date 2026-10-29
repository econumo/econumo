import axios from 'axios'
import { MutationCache, QueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/app/queryKeys'

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

// Reports aggregate every write the app can make (transactions, imports,
// classification merges, recurring posts, plans, account deletes), and those
// writes invalidate their own queries from a dozen places. Every one of them
// is a mutation, so a successful mutation of any kind marks the reports stale;
// an inactive report only refetches when it is next shown.
export function invalidateReports(client: QueryClient): void {
  void client.invalidateQueries({ queryKey: queryKeys.reportFlow })
  void client.invalidateQueries({ queryKey: queryKeys.reportNetWorth })
}

export function createAppQueryClient() {
  const client: QueryClient = new QueryClient({
    mutationCache: new MutationCache({ onSuccess: () => invalidateReports(client) }),
    defaultOptions: {
      // gcTime must outlive the persister's maxAge, or queries get
      // garbage-collected out of the persisted snapshot before it expires.
      queries: { gcTime: CACHE_MAX_AGE_MS, retry: retryPolicy },
    },
  })
  return client
}
