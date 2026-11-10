import type { FetchStatus } from '@tanstack/react-query'
import { useOnline } from '@/hooks/useOnline'

interface QueryState {
  data?: unknown
  isPlaceholderData?: boolean
  fetchStatus: FetchStatus
  isError: boolean
}

// Offline, a query with nothing cached never settles: networkMode 'online'
// leaves it pending and paused until the connection returns, so its loader
// would spin forever. An error caught before the browser noticed the drop
// (a request that failed on a dead network) is the same situation, not a
// server fault. Placeholder data belongs to another key (the previous month,
// scope or period), so it does not count as this query's own.
export function isUnavailableOffline(query: QueryState, online: boolean): boolean {
  if (query.data !== undefined && !query.isPlaceholderData) {
    return false
  }
  return query.fetchStatus === 'paused' || (!online && query.isError)
}

export function useUnavailableOffline(...queries: QueryState[]): boolean {
  const online = useOnline()
  return queries.some((q) => isUnavailableOffline(q, online))
}
