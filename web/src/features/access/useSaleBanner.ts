import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getSaleBanner } from '@/lib/billingBanner'
import type { SaleBanner, SaleBannerInput } from '@/lib/billingBanner'
import { getBillingUrl } from '@/lib/config'
import { useUserData } from '@/features/user/queries'
import { useConnections } from '@/features/connections/queries'

// setTimeout overflows past ~24.8 days; long waits re-arm daily instead.
const MAX_TIMER_MS = 86_400_000
// Billing picks the sale by its own clock; a client running ahead can get the
// ended sale back at endsAt, so it asks again a few times before giving up.
const STALE_RETRY_MS = 30_000
const MAX_STALE_RETRIES = 10
// Open tabs re-ask billing this often (and on becoming visible), so a sale
// announced later, or switched off early, reaches them.
const SALE_POLL_MS = 30 * 60_000

// The billing-provided sale banner, only while startsAt <= now < endsAt.
export function useSaleBanner(): SaleBanner | null {
  const { i18n } = useTranslation()
  const billingEnabled = getBillingUrl() !== ''
  const { data: user } = useUserData()
  const { data: connections } = useConnections({ enabled: billingEnabled })
  const [banner, setBanner] = useState<SaleBanner | null>(null)
  // Ticks re-render at boundaries and on wake; visibility itself reads the clock.
  const [now, setNow] = useState(() => Date.now())
  // Asks billing again unless an answer is already pending; null without input.
  const askRef = useRef<(() => void) | null>(null)
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
    // A banner picked for the previous access data or locale no longer
    // applies; drop it rather than show it while the new answer is pending.
    setBanner(null)
    const input = JSON.parse(inputKey) as SaleBannerInput
    let cancelled = false
    let pending = false
    const ask = () => {
      if (pending) return
      pending = true
      void getSaleBanner(input).then((next) => {
        pending = false
        if (cancelled) return
        setBanner(next)
        setStaleFetches((n) => (next && Date.parse(next.endsAt) <= Date.now() ? n + 1 : 0))
        setNow(Date.now())
      })
    }
    askRef.current = ask
    ask()
    const poll = setInterval(ask, SALE_POLL_MS)
    const onVisible = () => {
      if (document.visibilityState === 'visible') ask()
    }
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      cancelled = true
      askRef.current = null
      clearInterval(poll)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [inputKey])

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
          askRef.current?.()
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
