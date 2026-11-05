import { useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useUpdateLastSeenVersion } from '@/features/user/queries'
import { METRICS, trackEvent } from '@/lib/metrics'
import type { Metric } from '@/lib/metrics'
import { useWhatsNew } from './useWhatsNew'

export function WhatsNewNotice() {
  const { t } = useTranslation()
  const whatsNew = useWhatsNew()
  const markSeen = useUpdateLastSeenVersion()
  // Hidden right away, not once the write lands: a failed write (offline)
  // only brings the card back on the next load.
  const [hidden, setHidden] = useState<string | null>(null)
  if (!whatsNew || hidden === whatsNew.version) {
    return null
  }
  const close = (metric: Metric) => {
    setHidden(whatsNew.version)
    trackEvent(metric, { version: whatsNew.version, notesVersion: whatsNew.notesVersion })
    markSeen.mutate(whatsNew.version)
  }
  return (
    <div className="mx-3 mb-2 flex items-center gap-2 rounded-md bg-accent px-3 py-2 text-xs text-muted-foreground">
      <a
        href={whatsNew.url}
        target="_blank"
        rel="noreferrer"
        className="min-w-0 flex-1 truncate font-medium hover:text-foreground"
        onClick={() => close(METRICS.WHATS_NEW_OPEN)}
      >
        {t('whatsNew.notice', { version: whatsNew.notesVersion })}
      </a>
      <button
        type="button"
        aria-label={t('whatsNew.dismiss')}
        className="shrink-0 hover:text-foreground"
        onClick={() => close(METRICS.WHATS_NEW_DISMISS)}
      >
        <X className="size-3.5" />
      </button>
    </div>
  )
}
