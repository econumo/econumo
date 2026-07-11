import { getItem, removeItem, setItem } from './storage'

export interface LocaleOption {
  value: string
  label: string
  short: string
}

export interface EconumoConfig {
  API_URL?: string
  ALLOW_REGISTRATION?: boolean | string
  PAYWALL_ENABLED?: boolean | string
  ALLOW_CUSTOM_API?: boolean | string
  VERSION?: string
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
  if (window.econumoConfig.API_URL) {
    return window.econumoConfig.API_URL
  }
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

export function isHttps(): boolean {
  return window.location.protocol === 'https:'
}

export function locale(value?: string): string {
  if (value === undefined) {
    const stored = getItem('locale')
    if (stored) {
      return stored as string
    }
    return (navigator.language || 'en').split('-')[0] || 'en'
  }
  setItem('locale', value)
  return value
}

export function getLocaleOptions(): LocaleOption[] {
  return [{ value: 'en', label: 'English', short: 'Eng' }]
}

export function getWebsiteUrl(): string {
  return import.meta.env.WEBSITE_URL ?? 'https://econumo.com'
}

export function getVersion(): string {
  return window.econumoConfig?.VERSION || String(import.meta.env.ECONUMO_VERSION ?? 'dev')
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

export function isPaywallEnabled(): boolean {
  const paywall = window.econumoConfig?.PAYWALL_ENABLED
  if (paywall === undefined) {
    return false
  }
  if (typeof paywall === 'boolean') {
    return paywall
  }
  return paywall === 'true'
}
