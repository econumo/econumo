import { getItem, removeItem, setItem } from './storage'

export interface LocaleOption {
  value: string
  label: string
  short: string
}

export interface EconumoConfig {
  ALLOW_REGISTRATION?: boolean | string
  ALLOW_CUSTOM_API?: boolean | string
  VERSION?: string
  ANALYTICS?: boolean | string
  BILLING_URL?: string
  LILTAG_CONFIG_URL?: string
  LILTAG_CACHE_TTL?: string
}

declare global {
  interface Window {
    econumoConfig: EconumoConfig
  }
}

export function selfHosted(value?: boolean): boolean {
  if (!isCustomApiAllowed()) {
    return false
  }
  if (value === undefined) {
    return !!getItem('selfHosted')
  }
  setItem('selfHosted', value)
  return value
}

export function backendHost(value?: string): string {
  if (!isCustomApiAllowed()) {
    const url = new URL(window.location.href)
    return `${url.protocol}//${url.host}`
  }
  if (value === undefined) {
    const defaultHost = window.location.origin
    if (!selfHosted()) {
      return defaultHost
    }
    return (getItem('backendHost') as string | null) ?? defaultHost
  }
  setItem('backendHost', value)
  return value
}

export function clearBackendHost(): void {
  removeItem('backendHost')
}

export function rememberedEmail(value?: string): string {
  if (value === undefined) {
    return (getItem('rememberedEmail') as string | null) ?? ''
  }
  setItem('rememberedEmail', value)
  return value
}

export function clearRememberedEmail(): void {
  removeItem('rememberedEmail')
}

export function isHttps(): boolean {
  return window.location.protocol === 'https:'
}

export function locale(value?: string): string {
  const supported = new Set(getLocaleOptions().map((o) => o.value))
  if (value === undefined) {
    const stored = getItem('locale')
    if (typeof stored === 'string' && supported.has(stored)) {
      return stored
    }
    const candidates = navigator.languages?.length ? navigator.languages : [navigator.language]
    for (const tag of candidates) {
      const primary = (tag || '').toLowerCase().split('-')[0]
      if (supported.has(primary)) {
        return primary
      }
    }
    return 'en'
  }
  setItem('locale', value)
  return value
}

export function getLocaleOptions(): LocaleOption[] {
  return [
    { value: 'en', label: 'English', short: 'EN' },
    { value: 'ru', label: 'Русский', short: 'РУ' },
  ]
}

export function getWebsiteUrl(): string {
  return import.meta.env.WEBSITE_URL ?? 'https://econumo.com'
}

export function getVersion(): string {
  return window.econumoConfig?.VERSION || String(import.meta.env.ECONUMO_VERSION ?? 'dev')
}

// The server merges BILLING_URL unconditionally into the served
// econumo-config.js (server truth); '' means billing UI is disabled.
export function getBillingUrl(): string {
  return window.econumoConfig?.BILLING_URL || ''
}

export function isCustomApiAllowed(): boolean {
  const allowCustomApi = window.econumoConfig?.ALLOW_CUSTOM_API
  if (typeof allowCustomApi === 'boolean') {
    return allowCustomApi
  }
  return allowCustomApi === 'true'
}

export function isRegistrationAllowed(): boolean {
  const allowRegistration = window.econumoConfig?.ALLOW_REGISTRATION
  if (allowRegistration === undefined) {
    return true
  }
  if (typeof allowRegistration === 'boolean') {
    return allowRegistration
  }
  return allowRegistration === 'true'
}

export function analyticsEnabled(): boolean {
  const analytics = window.econumoConfig?.ANALYTICS
  if (typeof analytics === 'boolean') {
    return analytics
  }
  // Absent or unrecognized fails OPEN (enabled): a stale hand-hosted config
  // file keeps the enabled-by-default contract.
  return analytics !== 'false'
}
