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
import { useSaleBanner } from './useSaleBanner'

// Dismissal persists for the local calendar day: the banner stays hidden
// across reloads and returns the next day (the countdown has moved by then).
const DISMISSED_KEY = 'subscriptionBannerDismissedDay'
// Per sale, so a dismissed Black Friday does not hide Cyber Monday.
const saleDismissedKey = (id: string) => `saleBannerDismissedDay:${id}`

export function SubscriptionBanner() {
  const { t, i18n } = useTranslation()
  const { state, daysLeft, billingEnabled } = useAccessState()
  // Partner warnings only exist where the billing portal can act on them,
  // so self-hosted instances (no BILLING_URL) never fetch connections here.
  const { data: connections = [] } = useConnections({ enabled: billingEnabled })
  const portal = useOpenBillingPortal()
  const sale = useSaleBanner()
  // Only forces the re-render after a close; whether a banner is dismissed is
  // always read from storage against today, so a tab left open overnight
  // shows it again the next day.
  const [, setClosedAt] = useState(0)
  const today = formatDate(new Date())
  const dismissed = localStorage.getItem(DISMISSED_KEY) === today
  const saleDismissed = sale !== null && localStorage.getItem(saleDismissedKey(sale.id)) === today

  const partner = billingEnabled && !dismissed ? worstConnectionAttention(connections) : null

  // One banner; a running sale outranks everything, then own state outranks
  // any partner's.
  const variant =
    sale && !saleDismissed
      ? ('sale' as const)
      : state === 'readonly'
        ? ('readonly' as const)
        : state === 'trial' && billingEnabled && daysLeft !== null && daysLeft <= 3 && !dismissed
          ? ('trial' as const)
          : partner
            ? partner.state === 'readonly'
              ? ('partner_readonly' as const)
              : ('partner_trial' as const)
            : null

  const saleId = variant === 'sale' ? sale!.id : null
  useEffect(() => {
    if (saleId) {
      trackEvent(METRICS.SUBSCRIPTION_BANNER_SHOW, { variant: 'sale', sale: saleId })
    } else if (variant) {
      trackEvent(METRICS.SUBSCRIPTION_BANNER_SHOW, { variant })
    }
  }, [variant, saleId])

  if (!variant) {
    return null
  }

  // The sale only exists with billing enabled, so it always gets a CTA.
  const cta = billingEnabled ? (
    <Button type="button" size="sm" variant="outline" disabled={portal.pending} onClick={() => portal.open()}>
      {variant === 'sale' ? t('subscription.banner.sale_cta') : t('subscription.banner.cta')}
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

  const dismiss = (
    <button
      type="button"
      aria-label={t('subscription.banner.dismiss')}
      className="shrink-0 p-1 hover:opacity-70"
      onClick={() => {
        if (variant === 'sale') {
          localStorage.setItem(saleDismissedKey(sale!.id), today)
        }
        localStorage.setItem(DISMISSED_KEY, today)
        setClosedAt(Date.now())
      }}
    >
      <X className="size-4" />
    </button>
  )

  if (variant === 'sale') {
    // One line on wide screens: "Black Friday: 30% off · ends Nov 29, 7:00 PM
    // [Get the deal] ✕". On phones a two-row grid, so neither the sale name
    // nor the button gets squeezed: the name beside ✕, then the end time and
    // the button across the full width. The end time never breaks inside
    // itself. (sm:contents dissolves the second row into the one-line flex.)
    return (
      <div className="grid grid-cols-[1fr_auto] items-center gap-x-2 gap-y-1.5 bg-primary/10 py-2 pr-2 pl-4 text-sm text-econumo-link sm:flex sm:gap-3 sm:pr-4">
        <span className="font-medium">
          {t('subscription.banner.sale', {
            name: sale!.name,
            percent: new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 2 }).format(sale!.percent),
          })}
        </span>
        <span className="sm:order-last">{dismiss}</span>
        <div className="col-span-2 flex items-center justify-between gap-3 pr-2 sm:contents">
          <span className="whitespace-nowrap opacity-80 sm:-ml-1.5">
            <span aria-hidden className="mr-1.5 hidden sm:inline">
              ·
            </span>
            {t('subscription.banner.sale_ends', { date: formatSaleEnd(sale!.endsAt, i18n.language) })}
          </span>
          <span className="shrink-0 sm:ml-auto">{cta}</span>
        </div>
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
    <div className="flex items-center gap-3 bg-primary/10 px-4 py-2 text-sm text-econumo-link">
      <span className="min-w-0 flex-1">{message}</span>
      {cta}
      {dismiss}
    </div>
  )
}

// When the sale ends, in the user's own time zone and language ("Nov 30,
// 3:00 AM"): the end is an instant, and its calendar day differs by zone.
function formatSaleEnd(endsAt: string, lang: string): string {
  return new Intl.DateTimeFormat(lang, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' }).format(
    new Date(endsAt),
  )
}
