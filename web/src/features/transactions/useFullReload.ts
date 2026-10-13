import { useCallback, useSyncExternalStore } from 'react'
import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { queryKeys } from '@/app/queryKeys'
import { clearTransactions } from '@/lib/transactionStore'
import { METRICS, trackEvent } from '@/lib/metrics'
import { requestFullReload } from './revisionTracking'

export type FullReloadSource = 'sync_button' | 'settings'

// Per client, so the sync button and the Settings row share one reload.
const pending = new WeakSet<QueryClient>()
const listeners = new Set<() => void>()

function setPending(queryClient: QueryClient, on: boolean): void {
  if (on) {
    pending.add(queryClient)
  } else {
    pending.delete(queryClient)
  }
  listeners.forEach((l) => l())
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

// The reload counts as running until the transactions download lands, fails
// once or pauses for a lost connection: an unreachable server retries
// forever, and that must not keep the action locked.
function settled(queryClient: QueryClient): Promise<void> {
  const done = () => {
    const state = queryClient.getQueryState(queryKeys.transactions)
    return !state || state.fetchStatus !== 'fetching' || state.fetchFailureCount > 0
  }
  return new Promise((resolve) => {
    if (done()) {
      resolve()
      return
    }
    const unsubscribe = queryClient.getQueryCache().subscribe(() => {
      if (done()) {
        unsubscribe()
        resolve()
      }
    })
  })
}

export function useFullReloadPending(): boolean {
  const queryClient = useQueryClient()
  return useSyncExternalStore(subscribe, () => pending.has(queryClient))
}

export function useFullReload(): (source: FullReloadSource) => Promise<void> {
  const queryClient = useQueryClient()
  const { t } = useTranslation()
  return useCallback(async (source: FullReloadSource) => {
    if (pending.has(queryClient)) {
      return
    }
    // The clear comes first and the download could not follow: an offline
    // reload would leave nothing to start from.
    if (navigator.onLine === false) {
      toast(t('settings.sync.full_reload_offline'), { id: 'full-reload' })
      return
    }
    setPending(queryClient, true)
    try {
      trackEvent(METRICS.DATA_FULL_RELOAD, { source })
      toast(t('settings.sync.full_reload'), { id: 'full-reload' })
      // the flag goes up after the clear so the sync that starts below finds it set
      await clearTransactions()
      requestFullReload()
      void queryClient.invalidateQueries()
      await settled(queryClient)
    } finally {
      setPending(queryClient, false)
    }
  }, [queryClient, t])
}
