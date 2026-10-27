import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getSaleBanner } from '@/lib/billingSdk'
import type { SaleBanner, SaleBannerInput } from '@/lib/billingSdk'
import { getBillingUrl } from '@/lib/config'
import { useUserData } from '@/features/user/queries'
import { useConnections } from '@/features/connections/queries'

// setTimeout overflows past ~24.8 days; long waits re-arm daily instead.
const MAX_TIMER_MS = 86_400_000
// Billing picks the sale by its own clock; a client running ahead can get the
// ended sale back at endsAt, so it asks again a few times before giving up.
const STALE_RETRY_MS = 30_000
const MAX_STALE_RETRIES = 10

// The billing-provided sale banner, only while startsAt <= now < endsAt.
export function useSaleBanner(): SaleBanner | null {
  const { i18n } = useTranslation()
  const billingEnabled = getBillingUrl() !== ''
  const { data: user } = useUserData()
  const { data: connections } = useConnections({ enabled: billingEnabled })
  const [banner, setBanner] = useState<SaleBanner | null>(null)
  // Ticks re-render at boundaries and on wake; visibility itself reads the clock.
  const [now, setNow] = useState(() => Date.now())
  const [refetches, setRefetches] = useState(0)
  // Consecutive fetches that returned an already-ended sale.
  const [staleFetches, setStaleFetches] = useState(0)

  // Serialized so refetched query data with unchanged content does not
  // re-ask billing; only access fields and locale matter.
  const input: SaleBannerInput | null =
    billingEnabled && user && connections
      ? {
          locale: i18n.language,
          user: { accessLevel: user.accessLevel, accessUntil: user.accessUntil, registeredAt: user.createdAt },
          connections: connections.map((c) => ({
            accessLevel: c.accessLevel,
            accessUntil: c.accessUntil,
            registeredAt: c.createdAt,
          })),
        }
      : null
  const inputKey = input ? JSON.stringify(input) : null

  useEffect(() => {
    if (inputKey === null) {
      setBanner(null)
      return
    }
    let cancelled = false
    void getSaleBanner(JSON.parse(inputKey) as SaleBannerInput).then((next) => {
      if (cancelled) return
      setBanner(next)
      setStaleFetches((n) => (next && Date.parse(next.endsAt) <= Date.now() ? n + 1 : 0))
      setNow(Date.now())
    })
    return () => {
      cancelled = true
    }
  }, [inputKey, refetches])

  // setTimeout pauses while the machine sleeps; waking re-checks the clock.
  useEffect(() => {
    const wake = () => setNow(Date.now())
    document.addEventListener('visibilitychange', wake)
    return () => document.removeEventListener('visibilitychange', wake)
  }, [])

  // Wake at the next boundary. Once endsAt has passed the banner is dropped
  // before the refetch, so a refetch that fails or hangs leaves it hidden; the
  // refetch is what hands one sale over to the next (Black Friday -> Cyber
  // Monday), retried while billing still returns the ended sale.
  useEffect(() => {
    if (!banner) return
    const startsAt = Date.parse(banner.startsAt)
    const endsAt = Date.parse(banner.endsAt)
    if (Number.isNaN(startsAt) || Number.isNaN(endsAt)) return
    const current = Date.now()
    let delay: number
    if (current < startsAt) delay = startsAt - current
    else if (current < endsAt) delay = endsAt - current
    else if (staleFetches === 0) delay = 0
    else if (staleFetches <= MAX_STALE_RETRIES) delay = STALE_RETRY_MS
    else return
    const timer = setTimeout(
      () => {
        if (Date.now() >= endsAt) {
          setBanner(null)
          setRefetches((n) => n + 1)
        }
        setNow(Date.now())
      },
      Math.min(delay, MAX_TIMER_MS),
    )
    return () => clearTimeout(timer)
  }, [banner, now, staleFetches])

  if (!banner || inputKey === null) return null
  const current = Date.now()
  const active = Date.parse(banner.startsAt) <= current && current < Date.parse(banner.endsAt)
  return active ? banner : null
}
