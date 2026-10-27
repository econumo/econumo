import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getSaleBanner } from '@/lib/billingSdk'
import type { SaleBanner, SaleBannerInput } from '@/lib/billingSdk'
import { getBillingUrl } from '@/lib/config'
import { useUserData } from '@/features/user/queries'
import { useConnections } from '@/features/connections/queries'

// setTimeout overflows past ~24.8 days; long waits re-arm daily instead.
const MAX_TIMER_MS = 86_400_000

// The billing-provided sale banner, only while startsAt <= now < endsAt.
export function useSaleBanner(): SaleBanner | null {
  const { i18n } = useTranslation()
  const billingEnabled = getBillingUrl() !== ''
  const { data: user } = useUserData()
  const { data: connections } = useConnections({ enabled: billingEnabled })
  const [banner, setBanner] = useState<SaleBanner | null>(null)
  const [now, setNow] = useState(() => Date.now())
  const [refetches, setRefetches] = useState(0)

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
    if (inputKey === null) return
    let cancelled = false
    void getSaleBanner(JSON.parse(inputKey) as SaleBannerInput).then((next) => {
      if (cancelled) return
      setBanner(next)
      setNow(Date.now())
    })
    return () => {
      cancelled = true
    }
  }, [inputKey, refetches])

  // Wake at the next boundary. At endsAt the banner is dropped before the
  // refetch, so a refetch that fails or hangs leaves it hidden; the refetch
  // is what hands one sale over to the next (Black Friday -> Cyber Monday).
  useEffect(() => {
    if (!banner) return
    const startsAt = Date.parse(banner.startsAt)
    const endsAt = Date.parse(banner.endsAt)
    const current = Date.now()
    const boundary = current < startsAt ? startsAt : endsAt
    if (!(boundary > current)) return
    const timer = setTimeout(
      () => {
        if (Date.now() >= endsAt) {
          setBanner(null)
          setRefetches((n) => n + 1)
        }
        setNow(Date.now())
      },
      Math.min(boundary - current, MAX_TIMER_MS),
    )
    return () => clearTimeout(timer)
  }, [banner, now])

  if (!banner || !billingEnabled) return null
  const active = Date.parse(banner.startsAt) <= now && now < Date.parse(banner.endsAt)
  return active ? banner : null
}
