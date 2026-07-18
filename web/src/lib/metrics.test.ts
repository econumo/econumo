import { beforeEach, describe, expect, it, vi } from 'vitest'
import { METRICS, posthogEventName, scrubbedPage, trackEvent } from './metrics'
import { capture } from './analytics'

vi.mock('./analytics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./analytics')>()
  return { ...actual, capture: vi.fn() }
})

beforeEach(() => {
  vi.clearAllMocks()
  window.econumoConfig = {}
  window.dataLayer = []
  window.history.replaceState({}, '', '/')
})

it('pushes the event with context to the dataLayer', () => {
  trackEvent(METRICS.TRANSACTION_CREATE, { a: 1 })
  expect(window.dataLayer).toHaveLength(1)
  const entry = window.dataLayer[0] as Record<string, unknown>
  expect(entry.event).toBe('appTransactionCreate')
  expect(entry.eventData).toEqual({ a: 1 })
  expect(entry.eventContext).toMatchObject({ selfHosted: false, locale: 'en' })
})

describe('PostHog capture', () => {
  it('captures with the whitelisted properties only', () => {
    window.history.replaceState({}, '', '/budgets/01980e2c-1111-7000-8000-123456789abc/details')
    trackEvent(METRICS.TRANSACTION_CREATE, { secret: 'never-sent' })
    expect(capture).toHaveBeenCalledTimes(1)
    const [event, props] = vi.mocked(capture).mock.calls[0]
    expect(event).toBe('transaction_create')
    expect(props).toEqual({
      host: 'self-hosted', // jsdom runs on localhost
      self_hosted: true,
      locale: 'en',
      version: 'dev',
      current_url: 'https://self-hosted/budgets/:id/details',
    })
  })

  it('keeps ui_modal micro-interactions dataLayer-only', () => {
    trackEvent(METRICS.UI_MODAL_TRANSACTION_OPEN)
    expect(capture).not.toHaveBeenCalled()
    expect(window.dataLayer).toHaveLength(1)
  })

  it('is gated by ANALYTICS=false but the dataLayer push survives', () => {
    window.econumoConfig = { ANALYTICS: false }
    trackEvent(METRICS.TRANSACTION_CREATE)
    expect(capture).not.toHaveBeenCalled()
    expect(window.dataLayer).toHaveLength(1)
  })
})

describe('posthogEventName', () => {
  it.each([
    ['appPageView', 'page_view'],
    ['appTransactionCreate', 'transaction_create'],
    ['appUIModalTransactionOpen', 'ui_modal_transaction_open'],
    ['appApiAccountOrderList', 'api_account_order_list'],
    ['appBudgetTransferEnvelopeBudget', 'budget_transfer_envelope_budget'],
  ])('%s -> %s', (metric, expected) => {
    expect(posthogEventName(metric)).toBe(expected)
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
