import { getVersionLabel } from './config'

export interface EconumoPackage {
  label: string
  includesConnections: boolean
  includesSharedAccess: boolean
}

// Computed lazily: window.econumoConfig loads before the bundle, but tests
// (and any future config reload) mutate it at runtime.
export function econumoPackage(): EconumoPackage {
  return {
    label: getVersionLabel(),
    includesConnections: true,
    includesSharedAccess: true,
  }
}
