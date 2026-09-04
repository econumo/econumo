import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

type AnalyticsModule = typeof import('./analytics')

let analytics: AnalyticsModule
let fetchMock: ReturnType<typeof vi.fn>

const COLLECTOR = 'https://t.kuznetsov.dev/api/events'
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/

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

interface Envelope {
  key: string
  attributes: Record<string, unknown>
  events: Array<Record<string, unknown>>
}

function sentPayload(call = 0): Envelope {
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
  it('batches and flushes on the timer with the collector envelope shape', () => {
    analytics.capture('transaction_create', { locale: 'en' })
    expect(fetchMock).not.toHaveBeenCalled()
    vi.advanceTimersByTime(10_000)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe(COLLECTOR)
    expect(init.method).toBe('POST')
    expect(init.keepalive).toBe(true)
    // text/plain keeps the POST CORS-simple (no preflight) and matches what
    // sendBeacon sends for a string body, so both transports look identical.
    expect(init.headers).toEqual({ 'Content-Type': 'text/plain' })
    const payload = sentPayload()
    expect(payload.key).toMatch(/^ak_/)
    expect(payload.events).toHaveLength(1)
    expect(payload.events[0].name).toBe('transaction_create')
    expect(payload.events[0].ts).toBeTruthy()
    expect(payload.events[0].attributes).toEqual({ locale: 'en' })
  })

  it('gives every event its own id, so a replayed batch dedupes server-side', () => {
    analytics.capture('a')
    analytics.capture('b')
    vi.advanceTimersByTime(10_000)
    const { events } = sentPayload()
    expect(events[0].id).toMatch(UUID_RE)
    expect(events[0].id).not.toBe(events[1].id)
  })

  it('uses one in-memory $install_id per page load', () => {
    analytics.capture('a')
    vi.advanceTimersByTime(10_000)
    const first = sentPayload().attributes.$install_id
    analytics.capture('b')
    vi.advanceTimersByTime(10_000)
    expect(sentPayload(1).attributes.$install_id).toBe(first)
    expect(first).toMatch(UUID_RE)
    expect(document.cookie).toBe('')
    expect(localStorage.length).toBe(0)
  })

  it('flushes immediately at 10 queued events', () => {
    for (let i = 0; i < 10; i++) {
      analytics.capture(`event-${i}`)
    }
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(sentPayload().events).toHaveLength(10)
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
    expect(sentPayload(1).events).toHaveLength(1)
  })

  // crypto.randomUUID exists only in secure contexts, so a self-hosted instance
  // reached over http://<lan-ip> does not have it (issue #197).
  it('assigns an $install_id in an insecure context, where crypto.randomUUID is absent', async () => {
    const getRandomValues = globalThis.crypto.getRandomValues.bind(globalThis.crypto)
    vi.stubGlobal('crypto', { getRandomValues })
    vi.resetModules()
    const insecure = await import('./analytics')
    insecure.capture('a')
    vi.advanceTimersByTime(10_000)
    const payload = sentPayload()
    expect(payload.attributes.$install_id).toMatch(UUID_RE)
    expect(payload.events[0].id).toMatch(UUID_RE)
  })

  it('sends the tail via sendBeacon when the tab hides', () => {
    const beacon = vi.fn(() => true)
    vi.stubGlobal('navigator', { ...window.navigator, sendBeacon: beacon })
    analytics.capture('a')
    Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true })
    document.dispatchEvent(new Event('visibilitychange'))
    expect(beacon).toHaveBeenCalledTimes(1)
    const [url, body] = beacon.mock.calls[0] as unknown as [string, string]
    expect(url).toBe(COLLECTOR)
    expect(JSON.parse(body).events).toHaveLength(1)
    expect(fetchMock).not.toHaveBeenCalled()
    Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true })
  })

  it('carries identity on the batch and drops it on reset', () => {
    analytics.setAnalyticsUser('a'.repeat(32))
    analytics.setAnalyticsGroup('a3f19c02b7d4', 'selfhosted_a3f19c02b7d4')

    analytics.capture('test_event')
    vi.advanceTimersByTime(10_000)

    const first = sentPayload()
    expect(first.attributes.$user_id).toBe('a'.repeat(32))
    expect(first.attributes.$group_id).toBe('a3f19c02b7d4')
    expect(first.attributes.$group_name).toBe('selfhosted_a3f19c02b7d4')
    expect(first.attributes.$user_name).toBeUndefined()
    const installId = first.attributes.$install_id

    analytics.resetAnalyticsIdentity()
    analytics.capture('after_logout')
    vi.advanceTimersByTime(10_000)

    const second = sentPayload(1)
    expect(second.attributes.$user_id).toBeUndefined()
    // A fresh install id so the next person on a shared browser is not linked.
    expect(second.attributes.$install_id).not.toBe(installId)
    // The group is the deployment, not the person, so it survives logout.
    expect(second.attributes.$group_id).toBe('a3f19c02b7d4')
  })
})
