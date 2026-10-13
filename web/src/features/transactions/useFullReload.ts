import { useCallback } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { clearTransactions } from '@/lib/transactionStore'
import { METRICS, trackEvent } from '@/lib/metrics'
import { requestFullReload } from './revisionTracking'

export type FullReloadSource = 'sync_button' | 'settings'

export function useFullReload(): (source: FullReloadSource) => Promise<void> {
  const queryClient = useQueryClient()
  const { t } = useTranslation()
  return useCallback(async (source: FullReloadSource) => {
    trackEvent(METRICS.DATA_FULL_RELOAD, { source })
    toast(t('settings.sync.full_reload'), { id: 'full-reload' })
    // the flag goes up after the clear so the sync that starts below finds it set
    await clearTransactions()
    requestFullReload()
    void queryClient.invalidateQueries()
  }, [queryClient, t])
}
