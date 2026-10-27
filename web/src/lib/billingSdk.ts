import { backendHost, getBillingUrl } from './config'
import { isNativeApp } from './platform'

export interface SaleBanner {
  id: string
  text: string
  cta: string
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

declare global {
  interface Window {
    EconumoBilling?: { getBanner(input: SaleBannerInput): Promise<SaleBanner | null> }
  }
}

const BANNER_FIELDS = ['id', 'text', 'cta', 'startsAt', 'endsAt'] as const

let sdkLoad: Promise<boolean> | null = null

// The script is injected once per session; a failed load stays memoized as
// false so a broken billing portal is not hit again on every call.
function loadSdk(billingUrl: string): Promise<boolean> {
  sdkLoad ??= new Promise<boolean>((resolve) => {
    const script = document.createElement('script')
    try {
      // Through the URL API so a query on BILLING_URL stays a query rather
      // than swallowing the path.
      const src = new URL(billingUrl)
      src.pathname = `${src.pathname.replace(/\/+$/, '')}/sdk.js`
      script.src = src.toString()
    } catch {
      resolve(false)
      return
    }
    script.async = true
    script.addEventListener('load', () => resolve(true))
    script.addEventListener('error', () => resolve(false))
    document.head.appendChild(script)
  })
  return sdkLoad
}

function isBanner(value: unknown): value is SaleBanner {
  if (typeof value !== 'object' || value === null) return false
  const record = value as Record<string, unknown>
  return BANNER_FIELDS.every((field) => typeof record[field] === 'string' && record[field] !== '')
}

export async function getSaleBanner(input: SaleBannerInput): Promise<SaleBanner | null> {
  const billingUrl = getBillingUrl()
  if (billingUrl === '' || !servesSelectedBackend()) return null
  try {
    if (!(await loadSdk(billingUrl)) || !window.EconumoBilling) return null
    const banner = await window.EconumoBilling.getBanner(input)
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
