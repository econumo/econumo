import { useCallback, useSyncExternalStore } from 'react'
import { useTranslation } from 'react-i18next'
import { onlineManager } from '@tanstack/react-query'

interface ReportQueryState {
  data?: unknown
  isError: boolean
  fetchStatus: string
  dataUpdatedAt: number
}

function useOnline(): boolean {
  return useSyncExternalStore(
    useCallback((onChange: () => void) => onlineManager.subscribe(onChange), []),
    () => onlineManager.isOnline(),
  )
}

// Cached figures shown while they cannot be refreshed (offline, or the last
// fetch failed) say how old they are.
export function ReportUpdatedAt({ query }: { query: ReportQueryState }) {
  const { t, i18n } = useTranslation()
  const online = useOnline()
  const stale = query.isError || query.fetchStatus === 'paused' || !online
  if (query.data === undefined || query.dataUpdatedAt <= 0 || !stale) return null
  const time = new Intl.DateTimeFormat(i18n.language, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(query.dataUpdatedAt))
  return <p className="text-xs text-muted-foreground">{t('reports.updated_at', { time })}</p>
}
