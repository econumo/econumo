import { useEffect, useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { formatDate } from '@/lib/datetime'
import { pluralPick } from '@/lib/plural'
import { METRICS, trackEvent } from '@/lib/metrics'
import { worstConnectionAttention } from '@/lib/access'
import { useAccessState } from '@/features/user/queries'
import { useConnections } from '@/features/connections/queries'
import { useOpenBillingPortal } from './useOpenBillingPortal'

// Dismissal persists for the local calendar day: the banner stays hidden
// across reloads and returns the next day (the countdown has moved by then).
const DISMISSED_KEY = 'subscriptionBannerDismissedDay'

export function SubscriptionBanner() {
  const { t, i18n } = useTranslation()
  const { state, daysLeft, billingEnabled } = useAccessState()
  // Partner warnings only exist where the billing portal can act on them,
  // so self-hosted instances (no BILLING_URL) never fetch connections here.
  const { data: connections = [] } = useConnections({ enabled: billingEnabled })
  const portal = useOpenBillingPortal()
  const [dismissed, setDismissed] = useState(() => localStorage.getItem(DISMISSED_KEY) === formatDate(new Date()))

  const partner = billingEnabled && !dismissed ? worstConnectionAttention(connections) : null

  // One banner; own state outranks any partner's.
  const variant =
    state === 'readonly'
      ? ('readonly' as const)
      : state === 'trial' && billingEnabled && daysLeft !== null && daysLeft <= 3 && !dismissed
        ? ('trial' as const)
        : partner
          ? partner.state === 'readonly'
            ? ('partner_readonly' as const)
            : ('partner_trial' as const)
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
      {t('subscription.banner.cta')}
    </Button>
  ) : null

  if (variant === 'readonly') {
    return (
      <div className="flex items-center gap-3 bg-destructive/10 px-4 py-2 text-sm text-destructive">
        <span className="min-w-0 flex-1">{t('subscription.banner.readonly')}</span>
        {cta}
      </div>
    )
  }

  const message =
    variant === 'trial'
      ? pluralPick(t('subscription.banner.trial'), Math.max(daysLeft ?? 0, 0), i18n.language)
      : variant === 'partner_trial'
        ? pluralPick(t('subscription.banner.connection_trial', { name: partner!.name }), partner!.daysLeft ?? 0, i18n.language)
        : t('subscription.banner.connection_readonly', { name: partner!.name })

  return (
    <div className="flex items-center gap-3 bg-primary/10 px-4 py-2 text-sm text-primary">
      <span className="min-w-0 flex-1">{message}</span>
      {cta}
      <button
        type="button"
        aria-label={t('subscription.banner.dismiss')}
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
