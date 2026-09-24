import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  METRICS,
  analyticsEventName,
  analyticsHost,
  deploymentKind,
  isCloudHost,
  scrubbedPage,
  trackEvent,
  viewMode,
  setAnalyticsAccessState,
} from './metrics'
import { capture, capturePageView } from './analytics'
import * as analyticsModule from './analytics'
import { rememberAnalyticsPreference } from './analyticsPreference'
import { authMethods, forgetAuthMethods, rememberHasPassword, rememberLinkedProviders } from './analyticsAuthMethods'
import { backendHost, selfHosted } from './config'
import { setToken } from './storage'

vi.mock('./analytics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./analytics')>()
  return { ...actual, capture: vi.fn(), capturePageView: vi.fn() }
})

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  // The collector receives authenticated sessions only.
  setToken('eco_ses_test')
  window.econumoConfig = {}
  window.history.replaceState({}, '', '/')
})

it('sends nothing when opted out', () => {
  rememberAnalyticsPreference(false)

  trackEvent(METRICS.ACCOUNT_CREATE)
  trackEvent(METRICS.PAGE_VIEW)

  expect(capture).not.toHaveBeenCalled()
  expect(capturePageView).not.toHaveBeenCalled()
})

it('leaves no dataLayer behind', () => {
  trackEvent(METRICS.TRANSACTION_CREATE)
  expect(window).not.toHaveProperty('dataLayer')
})

describe('collector capture', () => {
  it('sends nothing without a session token', () => {
    localStorage.clear()
    trackEvent(METRICS.PAGE_VIEW)
    trackEvent(METRICS.USER_REGISTRATION)
    expect(capture).not.toHaveBeenCalled()
    expect(capturePageView).not.toHaveBeenCalled()
  })

  // The event's own data and the page it happened on are per-event;
  // everything else describes the session and comes from the context.
  it('captures the event data and the masked path', () => {
    window.history.replaceState({}, '', '/budgets/01980e2c-1111-7000-8000-123456789abc/details')
    trackEvent(METRICS.CLASSIFICATION_MERGE, { type: 'payee' })
    expect(capture).toHaveBeenCalledTimes(1)
    const [event, props] = vi.mocked(capture).mock.calls[0]
    expect(event).toBe('classification_merge')
    expect(props).toEqual({ type: 'payee', $path: '/budgets/:id/details' })
  })

  it('keeps the masked path over event data claiming one', () => {
    window.history.replaceState({}, '', '/account/01980e2c-1111-7000-8000-123456789abc')
    trackEvent(METRICS.TRANSACTION_CREATE, { $path: '/account/01980e2c-1111-7000-8000-123456789abc' })
    const [, props] = vi.mocked(capture).mock.calls[0]
    expect(props?.$path).toBe('/account/:id')
  })

  it('sends the session-wide facts, system keys included, through the context', () => {
    const contextSpy = vi.spyOn(analyticsModule, 'setAnalyticsContext')
    trackEvent(METRICS.TRANSACTION_CREATE)
    expect(contextSpy).toHaveBeenLastCalledWith(
      expect.objectContaining({
        $platform: 'web',
        // jsdom runs on localhost with no INSTANCE_ID configured
        $host: 'selfhosted_unknown',
        $app_locale: 'en',
        $device: 'desktop', // jsdom default viewport is 1024px wide
        deployment: 'self-hosted',
      }),
    )
    const context = contextSpy.mock.calls.at(-1)![0]
    for (const key of ['host', 'locale', 'mode', 'current_url']) {
      expect(context).not.toHaveProperty(key)
    }
    const [, props] = vi.mocked(capture).mock.calls.at(-1)!
    for (const key of ['$host', 'deployment', '$app_locale', '$device']) {
      expect(props).not.toHaveProperty(key)
    }
  })

  it('sends a page view as a masked $page_view, not a product event', () => {
    window.history.replaceState({}, '', '/account/01980e2c-1111-7000-8000-123456789abc')
    trackEvent(METRICS.PAGE_VIEW)
    expect(capture).not.toHaveBeenCalled()
    expect(capturePageView).toHaveBeenCalledWith('/account/01980e2c-1111-7000-8000-123456789abc', {
      $path: '/account/:id',
      // null drops the SDK's document.referrer: a self-hosted instance's own
      // domain would otherwise be stored as a referral source.
      $referrer: null,
    })
  })
})

describe('host attributes', () => {
  it('keeps cloud hostnames verbatim', () => {
    expect(analyticsHost('econumo.com')).toBe('econumo.com')
    expect(analyticsHost('app.econumo.com')).toBe('app.econumo.com')
    expect(deploymentKind('app.econumo.com')).toBe('cloud')
  })

  it('replaces every other hostname with the instance id', () => {
    window.econumoConfig = { ...window.econumoConfig, INSTANCE_ID: 'a3f19c02b7d4' }
    expect(analyticsHost('money.example.com')).toBe('selfhosted_a3f19c02b7d4')
    expect(analyticsHost('192.168.1.20')).toBe('selfhosted_a3f19c02b7d4')
    expect(deploymentKind('money.example.com')).toBe('self-hosted')
  })

  it('falls back to a bare marker when the server sent no instance id', () => {
    window.econumoConfig = { ...window.econumoConfig, INSTANCE_ID: '' }
    expect(analyticsHost('money.example.com')).toBe('selfhosted_unknown')
  })

  // A lookalike domain must not be treated as cloud.
  it('does not match a suffix impostor', () => {
    expect(isCloudHost('notdeconumo.com')).toBe(false)
    expect(isCloudHost('econumo.com.evil.test')).toBe(false)
  })
})

describe('analyticsEventName', () => {
  it.each([
    ['appPageView', 'page_view'],
    ['appTransactionCreate', 'transaction_create'],
    ['appUIModalTransactionOpen', 'ui_modal_transaction_open'],
    ['appApiAccountOrderList', 'api_account_order_list'],
    ['appBudgetTransferEnvelopeBudget', 'budget_transfer_envelope_budget'],
  ])('%s -> %s', (metric, expected) => {
    expect(analyticsEventName(metric)).toBe(expected)
  })
})

describe('viewMode', () => {
  it.each([
    [320, 'mobile'],
    [767, 'mobile'],
    [768, 'tablet'],
    [1023, 'tablet'],
    [1024, 'desktop'],
    [1920, 'desktop'],
  ])('%dpx -> %s', (width, expected) => {
    expect(viewMode(width)).toBe(expected)
  })
})

describe('scrubbedPage', () => {
  it.each([
    ['/accounts', 'accounts'],
    ['/budgets/01980e2c-1111-7000-8000-123456789abc', 'budgets/:id'],
    [
      '/budgets/01980E2C-1111-7000-8000-123456789ABC/tags/01980e2c-2222-7000-8000-123456789abc',
      'budgets/:id/tags/:id',
    ],
    ['/', ''],
  ])('%s -> %s', (path, expected) => {
    expect(scrubbedPage(path)).toBe(expected)
  })
})

describe('analytics group', () => {
  // The mobile app's main.tsx statically imports the module that used to set
  // the group at module scope, before the async fetchServerConfig() merges
  // the real INSTANCE_ID — so a fixed-at-import read would leave the group
  // unset for the whole session. The group must therefore be resolved fresh
  // on every trackEvent call, so a late-arriving INSTANCE_ID still takes
  // effect on the very next event.
  it('picks up an INSTANCE_ID that arrives after module evaluation', () => {
    const groupSpy = vi.spyOn(analyticsModule, 'setAnalyticsGroup')

    trackEvent(METRICS.USER_LOGIN)
    expect(groupSpy).not.toHaveBeenCalled()

    window.econumoConfig = { ...window.econumoConfig, INSTANCE_ID: 'a3f19c02b7d4' }
    trackEvent(METRICS.USER_LOGIN)
    expect(groupSpy).toHaveBeenCalledWith('a3f19c02b7d4', 'selfhosted_a3f19c02b7d4')
  })
})

describe('native app host resolution', () => {
  afterEach(() => {
    delete (window as { Capacitor?: unknown }).Capacitor
  })

  // window.location.hostname is always 'localhost' inside a Capacitor
  // WebView, so the effective host must come from the configured backend.
  it('treats the Econumo Cloud backend as cloud in the native app', () => {
    window.Capacitor = { isNativePlatform: () => true }
    selfHosted(true)
    backendHost('https://app.econumo.com')
    expect(isCloudHost()).toBe(true)
    expect(deploymentKind()).toBe('cloud')
  })

  it('treats a configured self-hosted backend as self-hosted in the native app', () => {
    window.Capacitor = { isNativePlatform: () => true }
    window.econumoConfig = { ...window.econumoConfig, INSTANCE_ID: 'a3f19c02b7d4' }
    selfHosted(true)
    backendHost('https://my.server.example')
    expect(isCloudHost()).toBe(false)
    expect(deploymentKind()).toBe('self-hosted')
    expect(analyticsHost()).toBe('selfhosted_a3f19c02b7d4')
  })

  it('falls back to window.location.hostname on the web (unchanged behavior)', () => {
    expect(isCloudHost()).toBe(false)
  })
})

describe('access_state property', () => {
  afterEach(() => setAnalyticsAccessState(null))

  it('is attached to the batch once set', () => {
    const contextSpy = vi.spyOn(analyticsModule, 'setAnalyticsContext')
    setAnalyticsAccessState('trial')
    trackEvent(METRICS.USER_LOGIN)
    expect(contextSpy).toHaveBeenLastCalledWith(expect.objectContaining({ access_state: 'trial' }))
    const [, props] = vi.mocked(capture).mock.calls.at(-1)!
    expect(props).not.toHaveProperty('access_state')
  })

  it('is absent before any state is known', () => {
    const contextSpy = vi.spyOn(analyticsModule, 'setAnalyticsContext')
    trackEvent(METRICS.USER_LOGIN)
    expect(contextSpy).toHaveBeenLastCalledWith(expect.not.objectContaining({ access_state: expect.anything() }))
  })
})

describe('auth method flags', () => {
  afterEach(() => forgetAuthMethods())

  it('rides the batch context, not the per-event properties', () => {
    const contextSpy = vi.spyOn(analyticsModule, 'setAnalyticsContext')
    rememberHasPassword(true)
    rememberLinkedProviders(['google'])

    trackEvent(METRICS.TRANSACTION_CREATE)

    expect(contextSpy).toHaveBeenLastCalledWith(
      expect.objectContaining({ auth_password: 'on', auth_google: 'on', auth_apple: 'off', auth_sso: 'off' }),
    )
    const [, props] = vi.mocked(capture).mock.calls.at(-1)!
    expect(props).not.toHaveProperty('auth_password')
  })

  it('maps the custom OIDC slot to auth_sso', () => {
    rememberLinkedProviders(['oidc'])
    expect(authMethods()).toMatchObject({ auth_sso: 'on', auth_google: 'off', auth_apple: 'off' })
  })

  it("reports an OAuth-only account as auth_password 'off'", () => {
    rememberHasPassword(false)
    rememberLinkedProviders(['apple'])
    expect(authMethods()).toEqual({ auth_password: 'off', auth_google: 'off', auth_apple: 'on', auth_sso: 'off' })
  })

  // The two halves arrive from different endpoints; whichever lands second
  // must not erase the other's answer.
  it('merges the two writers rather than overwriting', () => {
    rememberHasPassword(true)
    rememberLinkedProviders(['google', 'apple'])
    expect(authMethods()).toEqual({ auth_password: 'on', auth_google: 'on', auth_apple: 'on', auth_sso: 'off' })

    // the identity list refetches after an unlink; the password flag survives
    rememberLinkedProviders(['google'])
    expect(authMethods()).toEqual({ auth_password: 'on', auth_google: 'on', auth_apple: 'off', auth_sso: 'off' })
  })

  it('omits a flag whose source has not answered yet', () => {
    rememberHasPassword(true)
    // no identity list yet — the OAuth flags are unknown, not "off"
    expect(authMethods()).toEqual({ auth_password: 'on' })
  })

  it('is absent entirely before anything is known', () => {
    const contextSpy = vi.spyOn(analyticsModule, 'setAnalyticsContext')
    trackEvent(METRICS.USER_LOGIN)
    expect(contextSpy).toHaveBeenLastCalledWith(expect.not.objectContaining({ auth_password: expect.anything() }))
  })

  it('survives a reload, so the boot page view still carries it', () => {
    rememberHasPassword(true)
    rememberLinkedProviders(['google'])
    expect(authMethods()).toMatchObject({ auth_password: 'on', auth_google: 'on' })
  })

  // A build shipped 0/1 before these became words; a value left in storage by
  // it must be ignored rather than sent on as a stray numeric label.
  it('drops a stale numeric value from an earlier build', () => {
    localStorage.setItem('authMethods', JSON.stringify({ auth_password: 1, auth_google: 0 }))
    expect(authMethods()).toBeNull()
  })

  it('does not outlive the session it describes', () => {
    rememberHasPassword(true)
    rememberLinkedProviders(['google'])
    forgetAuthMethods()
    expect(authMethods()).toBeNull()
  })
})
