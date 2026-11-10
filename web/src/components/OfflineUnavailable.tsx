import { CloudOff } from 'lucide-react'
import { useTranslation } from 'react-i18next'

const TEXT = {
  page: 'common.offline_unavailable.page',
  month: 'common.offline_unavailable.month',
  months: 'common.offline_unavailable.months',
  report: 'common.offline_unavailable.report',
  imports: 'common.offline_unavailable.imports',
} as const

export type OfflineSubject = keyof typeof TEXT

// What a page shows offline in place of a loader that would never finish or a
// generic error: the data was never loaded on this device. It names no retry,
// because the query resumes on its own once the connection returns.
export function OfflineUnavailable({ subject = 'page', compact = false }: { subject?: OfflineSubject; compact?: boolean }) {
  const { t } = useTranslation()
  const text = t(TEXT[subject])
  if (compact) {
    return (
      <p role="status" data-testid="offline-unavailable" className="flex items-start gap-2 text-sm text-muted-foreground">
        <CloudOff className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
        <span>{text}</span>
      </p>
    )
  }
  return (
    <div role="status" data-testid="offline-unavailable" className="flex h-full flex-1 flex-col items-center justify-center gap-2 p-6 text-center">
      <CloudOff className="size-6 text-muted-foreground" aria-hidden="true" />
      <p className="text-base">{t('common.offline_unavailable.title')}</p>
      <p className="max-w-sm text-sm text-muted-foreground">{text}</p>
    </div>
  )
}
