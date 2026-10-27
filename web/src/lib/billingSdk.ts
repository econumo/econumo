import { getBillingUrl } from './config'

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
    script.src = `${billingUrl.replace(/\/+$/, '')}/sdk.js`
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
  if (billingUrl === '') return null
  try {
    if (!(await loadSdk(billingUrl)) || !window.EconumoBilling) return null
    const banner = await window.EconumoBilling.getBanner(input)
    return isBanner(banner) ? banner : null
  } catch {
    return null
  }
}
