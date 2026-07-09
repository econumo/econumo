import { isPaywallEnabled, getVersion } from './config'

export interface EconumoPackage {
  label: string
  includesConnections: boolean
  includesSharedAccess: boolean
  isPaywallEnabled: boolean
  paywallUrl: string
}

// Computed lazily: window.econumoConfig loads before the bundle, but tests
// (and any future config reload) mutate it at runtime.
export function econumoPackage(): EconumoPackage {
  const paywall = isPaywallEnabled()
  return {
    label: getVersion(),
    includesConnections: true,
    includesSharedAccess: true,
    isPaywallEnabled: paywall,
    paywallUrl: paywall ? 'https://pay.econumo.com/cloud/' : '',
  }
}
