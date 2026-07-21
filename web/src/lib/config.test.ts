import { analyticsEnabled, backendHost, selfHosted, locale, isCustomApiAllowed, isRegistrationAllowed, isPaywallEnabled, getVersion, getBillingUrl } from './config'

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

describe('backendHost', () => {
  it('prefers econumoConfig.API_URL over everything', () => {
    window.econumoConfig = { API_URL: 'https://api.example.test', ALLOW_CUSTOM_API: 'true' }
    expect(backendHost()).toBe('https://api.example.test')
  })

  it('falls back to the page origin when custom API is not allowed', () => {
    window.econumoConfig = { ALLOW_CUSTOM_API: 'false' }
    expect(backendHost()).toBe(window.location.origin)
  })

  it('uses the stored custom host when self-hosted mode is on and custom API allowed', () => {
    window.econumoConfig = { ALLOW_CUSTOM_API: 'true' }
    selfHosted(true)
    backendHost('https://my.box.test')
    expect(backendHost()).toBe('https://my.box.test')
  })
})

describe('flags', () => {
  it('parses booleans and strings for ALLOW_CUSTOM_API', () => {
    window.econumoConfig = { ALLOW_CUSTOM_API: true }
    expect(isCustomApiAllowed()).toBe(true)
    window.econumoConfig = { ALLOW_CUSTOM_API: 'false' }
    expect(isCustomApiAllowed()).toBe(false)
  })

  it('defaults registration=true, paywall=false when unset', () => {
    expect(isRegistrationAllowed()).toBe(true)
    expect(isPaywallEnabled()).toBe(false)
  })

  it('returns the billing URL, empty when unset', () => {
    expect(getBillingUrl()).toBe('')
    window.econumoConfig = { BILLING_URL: 'https://pay.example.test/cloud/' }
    expect(getBillingUrl()).toBe('https://pay.example.test/cloud/')
  })
})

describe('locale and version', () => {
  it('persists a chosen locale and defaults to en', () => {
    expect(locale()).toBe('en')
    locale('en')
    expect(locale()).toBe('en')
  })

  it('prefers econumoConfig.VERSION for the version label', () => {
    window.econumoConfig = { VERSION: 'v9.9.9' }
    expect(getVersion()).toBe('v9.9.9')
  })

  it('ignores an unsupported stored locale', () => {
    // getItem/setItem JSON-encode under the raw 'locale' key (no prefix, see lib/storage.ts)
    localStorage.setItem('locale', JSON.stringify('de'))
    expect(locale()).toBe('en')
  })

  it('detects the first supported language from navigator.languages', () => {
    vi.stubGlobal('navigator', { ...navigator, languages: ['de-DE', 'ru-RU'], language: 'de-DE' })
    expect(locale()).toBe('ru')
  })

  it('falls back to english when nothing is supported', () => {
    vi.stubGlobal('navigator', { ...navigator, languages: ['de-DE', 'fr-FR'], language: 'de-DE' })
    expect(locale()).toBe('en')
  })
})

describe('analyticsEnabled', () => {
  it.each([
    [undefined, true],
    [true, true],
    ['true', true],
    [false, false],
    ['false', false],
    ['garbage', true], // unknown fails OPEN: enabled-by-default contract
  ])('ANALYTICS=%s -> %s', (value, expected) => {
    window.econumoConfig = { ANALYTICS: value as boolean | string | undefined }
    expect(analyticsEnabled()).toBe(expected)
  })
})
