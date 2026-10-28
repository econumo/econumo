import { backendHost, getBillingUrl } from './config'
import { isNativeApp } from './platform'

// The running (or upcoming) sale, as data: billing owns the promo name and
// the percentage, Econumo writes every word around them.
export interface SaleBanner {
  id: string
  name: string
  percent: number
  startsAt: string
  endsAt: string
}

interface SaleBannerEntry {
  accessLevel: 'full' | 'readonly'
  accessUntil: string
  registeredAt: string
}

export interface SaleBannerInput {
  locale: string
  user: SaleBannerEntry
  connections: SaleBannerEntry[]
}

const TEXT_FIELDS = ['id', 'name', 'startsAt', 'endsAt'] as const

// A hung request must not hold the banner hook's next ask hostage.
const TIMEOUT_MS = 10_000

function isBanner(value: unknown): value is SaleBanner {
  if (typeof value !== 'object' || value === null) return false
  const record = value as Record<string, unknown>
  const percent = record.percent
  return (
    TEXT_FIELDS.every((field) => typeof record[field] === 'string' && record[field] !== '') &&
    typeof percent === 'number' &&
    percent > 0 &&
    percent <= 100
  )
}

// Built through the URL API so a query on BILLING_URL stays a query rather
// than swallowing the path.
function bannerEndpoint(billingUrl: string): string {
  const url = new URL(billingUrl)
  url.pathname = `${url.pathname.replace(/\/+$/, '')}/api/v1/public/get-offer`
  return url.toString()
}

// Asks the billing portal which sale banner, if any, this user should see.
// Billing owns the rules and the copy; any failure is simply "no banner".
export async function getSaleBanner(input: SaleBannerInput): Promise<SaleBanner | null> {
  const billingUrl = getBillingUrl()
  if (billingUrl === '' || !servesSelectedBackend()) return null
  try {
    const res = await fetch(bannerEndpoint(billingUrl), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
      credentials: 'omit',
      signal: AbortSignal.timeout(TIMEOUT_MS),
    })
    if (!res.ok) return null
    const body = (await res.json()) as { data?: { banner?: unknown } }
    const banner = body.data?.banner
    return isBanner(banner) ? banner : null
  } catch {
    return null
  }
}

// On the web BILLING_URL describes the instance serving the page. When the SPA
// is pointed at a custom backend on another origin, that instance's sales have
// nothing to do with the selected backend's users (same rule as mcpUrl()).
function servesSelectedBackend(): boolean {
  if (isNativeApp()) return true
  try {
    return new URL(backendHost()).origin === window.location.origin
  } catch {
    return false
  }
}
