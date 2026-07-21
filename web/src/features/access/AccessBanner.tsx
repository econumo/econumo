import { useEffect, useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { pluralPick } from '@/lib/plural'
import { METRICS, trackEvent } from '@/lib/metrics'
import { useAccessState } from '@/features/user/queries'
import { useOpenBillingPortal } from './useOpenBillingPortal'

export function AccessBanner() {
  const { t, i18n } = useTranslation()
  const { state, daysLeft, billingEnabled } = useAccessState()
  const portal = useOpenBillingPortal()
  // Per-session dismissal: component state in the persistent layout — the
  // banner returns on the next page load or login.
  const [dismissed, setDismissed] = useState(false)

  const variant =
    state === 'readonly'
      ? 'readonly'
      : state === 'trial' && billingEnabled && daysLeft !== null && daysLeft <= 3
        ? 'trial'
        : null

  useEffect(() => {
    if (variant) {
      trackEvent(METRICS.ACCESS_BANNER_SHOW, { variant })
    }
  }, [variant])

  if (!variant || (variant === 'trial' && dismissed)) {
    return null
  }

  const cta = billingEnabled ? (
    <Button type="button" size="sm" variant="outline" disabled={portal.pending} onClick={() => portal.open()}>
      {t('access.banner.cta')}
    </Button>
  ) : null

  if (variant === 'readonly') {
    return (
      <div className="flex items-center gap-3 bg-destructive/10 px-4 py-2 text-sm text-destructive">
        <span className="min-w-0 flex-1">{t('access.banner.readonly')}</span>
        {cta}
      </div>
    )
  }

  return (
    <div className="flex items-center gap-3 bg-primary/10 px-4 py-2 text-sm text-primary">
      <span className="min-w-0 flex-1">
        {pluralPick(t('access.banner.trial'), Math.max(daysLeft ?? 0, 0), i18n.language)}
      </span>
      {cta}
      <button
        type="button"
        aria-label={t('access.banner.dismiss')}
        className="shrink-0 hover:opacity-70"
        onClick={() => setDismissed(true)}
      >
        <X className="size-4" />
      </button>
    </div>
  )
}
