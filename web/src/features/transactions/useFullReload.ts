import { useCallback, useSyncExternalStore } from 'react'
import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { queryKeys } from '@/app/queryKeys'
import { markForReload } from '@/lib/transactionStore'
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
    // nothing could be downloaded; say so instead of a reload that only waits
    if (navigator.onLine === false) {
      toast(t('settings.sync.full_reload_offline'), { id: 'full-reload' })
      return
    }
    setPending(queryClient, true)
    try {
      trackEvent(METRICS.DATA_FULL_RELOAD, { source })
      toast(t('settings.sync.full_reload'), { id: 'full-reload' })
      // The stored rows stay until their replacement arrives: the server can be
      // unreachable while the browser reports online, and a failed reload must
      // leave them readable offline. The in-memory state goes first, so no
      // write-through of the rows being replaced lands after the mark.
      requestFullReload()
      await markForReload()
      void queryClient.invalidateQueries()
      await settled(queryClient)
    } finally {
      setPending(queryClient, false)
    }
  }, [queryClient, t])
}
