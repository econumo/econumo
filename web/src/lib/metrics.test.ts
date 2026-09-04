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
import { capture } from './analytics'
import * as analyticsModule from './analytics'
import { rememberAnalyticsPreference } from './analyticsPreference'
import { backendHost, selfHosted } from './config'

vi.mock('./analytics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./analytics')>()
  return { ...actual, capture: vi.fn() }
})

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  window.econumoConfig = {}
  window.dataLayer = []
  window.history.replaceState({}, '', '/')
})

it('sends nothing to either sink when opted out', () => {
  const captureSpy = vi.spyOn(analyticsModule, 'capture')
  window.dataLayer = []
  rememberAnalyticsPreference(false)

  trackEvent(METRICS.ACCOUNT_CREATE)

  expect(captureSpy).not.toHaveBeenCalled()
  expect(window.dataLayer).toHaveLength(0)
})

it('pushes the event with context to the dataLayer', () => {
  trackEvent(METRICS.TRANSACTION_CREATE, { a: 1 })
  expect(window.dataLayer).toHaveLength(1)
  const entry = window.dataLayer[0] as Record<string, unknown>
  expect(entry.event).toBe('appTransactionCreate')
  expect(entry.eventData).toEqual({ a: 1 })
  expect(entry.eventContext).toMatchObject({ selfHosted: false, locale: 'en' })
})

describe('collector capture', () => {
  it('captures with the whitelisted properties only', () => {
    window.history.replaceState({}, '', '/budgets/01980e2c-1111-7000-8000-123456789abc/details')
    trackEvent(METRICS.TRANSACTION_CREATE, { secret: 'never-sent' })
    expect(capture).toHaveBeenCalledTimes(1)
    const [event, props] = vi.mocked(capture).mock.calls[0]
    expect(event).toBe('transaction_create')
    expect(props).toEqual({
      host: 'selfhosted_unknown', // jsdom runs on localhost with no INSTANCE_ID configured
      deployment: 'self-hosted',
      locale: 'en',
      mode: 'desktop', // jsdom default viewport is 1024px wide
      current_url: 'https://selfhosted_unknown/budgets/:id/details',
    })
    expect(props).not.toHaveProperty('version')
    expect(props).not.toHaveProperty('self_hosted')
  })

  it('keeps ui_modal micro-interactions dataLayer-only', () => {
    trackEvent(METRICS.UI_MODAL_TRANSACTION_OPEN)
    expect(capture).not.toHaveBeenCalled()
    expect(window.dataLayer).toHaveLength(1)
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

  it('is attached to captures once set', () => {
    setAnalyticsAccessState('trial')
    trackEvent(METRICS.USER_LOGIN)
    const [, props] = vi.mocked(capture).mock.calls.at(-1)!
    expect(props).toMatchObject({ access_state: 'trial' })
  })

  it('is absent before any state is known', () => {
    trackEvent(METRICS.USER_LOGIN)
    const [, props] = vi.mocked(capture).mock.calls.at(-1)!
    expect(props).not.toHaveProperty('access_state')
  })
})
