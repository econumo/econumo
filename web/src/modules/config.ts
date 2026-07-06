import { getItem, setItem } from './storage'
import { Quasar } from 'quasar'

interface LocaleOption {
  value: string
  label: string
  short: string
}

interface EconumoConfig {
  API_URL?: string
  ALLOW_REGISTRATION?: boolean | string
  PAYWALL_ENABLED?: boolean | string
  ALLOW_CUSTOM_API?: boolean | string
  VERSION?: string
}

declare global {
  interface Window {
    econumoConfig: EconumoConfig
  }
}

function selfHosted(value?: boolean): boolean | string {
  if (!isCustomApiAllowed()) {
    return '0'
  }

  if (value === undefined) {
    return !!getItem('selfHosted')
  }
  setItem('selfHosted', value ? '1' : '0')
  return value ? '1' : '0'
}

function backendHost(value?: string): string {
  if (window.econumoConfig.API_URL) {
    return window.econumoConfig.API_URL
  }

  if (!isCustomApiAllowed()) {
    const url = new URL(window.location.href)
    return `${url.protocol}//${url.host}`
  }

  if (value === undefined) {
    const defaultHost = window.econumoConfig.API_URL ? window.econumoConfig.API_URL : window.location.origin
    if (!selfHosted()) {
      return defaultHost
    } else {
      return getItem('backendHost') ?? defaultHost
    }
  }

  setItem('backendHost', value)
  return value
}

function isHttps(): boolean {
  return window.location.protocol === 'https:'
}

function locale(value?: string): string {
  if (value === undefined) {
    if (getItem('locale')) {
      return getItem('locale') as string
    } else if (Quasar.lang.isoName) {
      return Quasar.lang.isoName?.split('-')[0]
    } else {
      return 'en'
    }
  }

  setItem('locale', value)
  return value
}

function getLocaleOptions(): LocaleOption[] {
  return [
    { value: 'en', label: 'English', short: 'Eng' }
  ]
}

export function getWebsiteUrl(): string {
  return process.env.WEBSITE_URL ?? 'https://econumo.com'
}

export function getVersion(): string {
  return window.econumoConfig?.VERSION || String(process.env.ECONUMO_VERSION)
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
  const isPaywallEnabled = window.econumoConfig?.PAYWALL_ENABLED
  if (isPaywallEnabled === undefined) {
    return false
  }
  if (typeof isPaywallEnabled === 'boolean') {
    return isPaywallEnabled
  }
  return isPaywallEnabled === 'true'
}

export default {
  selfHosted,
  backendHost,
  isHttps,
  locale,
  getLocaleOptions,
  getWebsiteUrl,
  getVersion,
  isCustomApiAllowed,
  isRegistrationAllowed,
  isPaywallEnabled
} 