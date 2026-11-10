import { CloudOff } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useOnline } from '@/hooks/useOnline'
import { cn } from '@/lib/utils'

// The one explanation, on a page or dialog whose writes all need the server,
// for why its controls are disabled while offline. Pages where something can
// still be changed offline (transactions, limits, the Inbox) do not render it.
export function OfflineNotice({ className }: { className?: string }) {
  const { t } = useTranslation()
  const online = useOnline()
  if (online) {
    return null
  }
  return (
    <p
      role="status"
      data-testid="offline-notice"
      className={cn('flex items-start gap-2 rounded-lg border px-3 py-2 text-sm text-muted-foreground', className)}
    >
      <CloudOff className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <span>{t('common.offline_notice')}</span>
    </p>
  )
}
