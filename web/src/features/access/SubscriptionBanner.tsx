import { useEffect, useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { formatDate } from '@/lib/datetime'
import { pluralPick } from '@/lib/plural'
import { METRICS, trackEvent } from '@/lib/metrics'
import { useAccessState } from '@/features/user/queries'
import { useOpenBillingPortal } from './useOpenBillingPortal'

// Dismissal persists for the local calendar day: the banner stays hidden
// across reloads and returns the next day (the countdown has moved by then).
const DISMISSED_KEY = 'subscriptionBannerDismissedDay'

export function SubscriptionBanner() {
  const { t, i18n } = useTranslation()
  const { state, daysLeft, billingEnabled } = useAccessState()
  const portal = useOpenBillingPortal()
  const [dismissed, setDismissed] = useState(() => localStorage.getItem(DISMISSED_KEY) === formatDate(new Date()))

  const variant =
    state === 'readonly'
      ? 'readonly'
      : state === 'trial' && billingEnabled && daysLeft !== null && daysLeft <= 3 && !dismissed
        ? 'trial'
        : null

  useEffect(() => {
    if (variant) {
      trackEvent(METRICS.SUBSCRIPTION_BANNER_SHOW, { variant })
    }
  }, [variant])

  if (!variant) {
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
        onClick={() => {
          localStorage.setItem(DISMISSED_KEY, formatDate(new Date()))
          setDismissed(true)
        }}
      >
        <X className="size-4" />
      </button>
    </div>
  )
}
