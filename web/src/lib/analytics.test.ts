import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

type AnalyticsModule = typeof import('./analytics')

let analytics: AnalyticsModule
let fetchMock: ReturnType<typeof vi.fn>

beforeEach(async () => {
  vi.useFakeTimers()
  fetchMock = vi.fn(() => Promise.resolve(new Response()))
  vi.stubGlobal('fetch', fetchMock)
  vi.resetModules()
  analytics = await import('./analytics')
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

function sentPayload(call = 0): { api_key: string; batch: Array<Record<string, unknown>> } {
  const init = fetchMock.mock.calls[call][1] as RequestInit
  return JSON.parse(init.body as string)
}

describe('analyticsDomain', () => {
  it.each([
    ['econumo.com', 'econumo.com'],
    ['app.econumo.com', 'app.econumo.com'],
    ['myeconumo.com', 'self-hosted'],
    ['budget.example.org', 'self-hosted'],
    ['localhost', 'self-hosted'],
  ])('%s -> %s', (hostname, expected) => {
    expect(analytics.analyticsDomain(hostname)).toBe(expected)
  })
})

describe('capture', () => {
  it('batches and flushes on the timer with the PostHog payload shape', () => {
    analytics.capture('appTransactionCreate', { locale: 'en' })
    expect(fetchMock).not.toHaveBeenCalled()
    vi.advanceTimersByTime(10_000)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('https://us.i.posthog.com/batch/')
    expect(init.method).toBe('POST')
    expect(init.keepalive).toBe(true)
    const payload = sentPayload()
    expect(payload.api_key).toMatch(/^phc_/)
    expect(payload.batch).toHaveLength(1)
    expect(payload.batch[0].event).toBe('appTransactionCreate')
    expect(payload.batch[0].timestamp).toBeTruthy()
    expect(payload.batch[0].properties).toMatchObject({
      locale: 'en',
      $process_person_profile: false,
    })
  })

  it('uses one in-memory distinct_id per page load', () => {
    analytics.capture('a')
    analytics.capture('b')
    vi.advanceTimersByTime(10_000)
    const { batch } = sentPayload()
    expect(batch[0].distinct_id).toBe(batch[1].distinct_id)
    expect(batch[0].distinct_id).toMatch(/^[0-9a-f-]{36}$/)
    expect(document.cookie).toBe('')
    expect(localStorage.length).toBe(0)
  })

  it('flushes immediately at 10 queued events', () => {
    for (let i = 0; i < 10; i++) {
      analytics.capture(`event-${i}`)
    }
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(sentPayload().batch).toHaveLength(10)
    // The timer was cleared by the size-triggered flush: nothing further goes out.
    vi.advanceTimersByTime(10_000)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('drops the batch silently when fetch rejects', async () => {
    fetchMock.mockImplementation(() => Promise.reject(new TypeError('blocked')))
    analytics.capture('a')
    vi.advanceTimersByTime(10_000)
    await vi.runAllTimersAsync()
    // No unhandled rejection, and the queue restarts empty.
    analytics.capture('b')
    vi.advanceTimersByTime(10_000)
    expect(sentPayload(1).batch).toHaveLength(1)
  })

  it('sends the tail via sendBeacon when the tab hides', () => {
    const beacon = vi.fn(() => true)
    vi.stubGlobal('navigator', { ...window.navigator, sendBeacon: beacon })
    analytics.capture('a')
    Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true })
    document.dispatchEvent(new Event('visibilitychange'))
    expect(beacon).toHaveBeenCalledTimes(1)
    const [url, body] = beacon.mock.calls[0] as unknown as [string, string]
    expect(url).toBe('https://us.i.posthog.com/batch/')
    expect(JSON.parse(body).batch).toHaveLength(1)
    expect(fetchMock).not.toHaveBeenCalled()
    Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true })
  })
})
