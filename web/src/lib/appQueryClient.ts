import axios from 'axios'
import { MutationCache, QueryClient, onlineManager } from '@tanstack/react-query'
import { toast } from 'sonner'
import { queryKeys } from '@/app/queryKeys'
import { apiErrorMessage } from './apiError'

declare module '@tanstack/react-query' {
  interface Register {
    mutationMeta: { handlesOwnErrors?: boolean }
  }
}

/** For a mutation hook whose callers differ: the ones that show the failure themselves pass this. */
export type OwnErrorHandling = { handlesOwnErrors?: boolean }

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
  void client.invalidateQueries({ queryKey: queryKeys.reportNetWorth })
}

export function createAppQueryClient() {
  // The manager assumes online until the first browser event, so an app opened
  // with no network would retry every read forever.
  onlineManager.setOnline(navigator.onLine !== false)
  const client: QueryClient = new QueryClient({
    mutationCache: new MutationCache({
      onSuccess: () => invalidateReports(client),
      // With networkMode 'always' an offline write fails at once, and a site
      // with no error handling of its own would otherwise fail silently. A
      // `mutate(v, { onError })` or a try around `mutateAsync` is invisible here,
      // so such a hook says so through its meta.
      onError: (error, _variables, _result, mutation) => {
        if (axios.isAxiosError(error) && !error.response && !mutation.options.onError && !mutation.meta?.handlesOwnErrors) {
          toast.error(apiErrorMessage(error), { id: 'no-response' })
        }
      },
    }),
    defaultOptions: {
      // gcTime must outlive the persister's maxAge, or queries get
      // garbage-collected out of the persisted snapshot before it expires.
      queries: { gcTime: CACHE_MAX_AGE_MS, retry: retryPolicy },
      // A write outside the queue fails at once offline rather than pausing with its form open.
      mutations: { networkMode: 'always', retry: false },
    },
  })
  return client
}
