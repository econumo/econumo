import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

type AnalyticsModule = typeof import('./analytics')

const SDK_URL = 'https://t.econumo.com/js/twillingate.js'
const USER = 'u'.repeat(32)

// Records every SDK call in order, so tests can assert both what was sent and
// that a flush came before an identity change.
function fakeInstance() {
  const log: Array<[string, ...unknown[]]> = []
  const instance = {
    log,
    init: vi.fn((opts: unknown) => log.push(['init', opts])),
    identify: vi.fn((user: string) => log.push(['identify', user])),
    group: vi.fn((id: string, name?: string) => log.push(['group', id, name])),
    track: vi.fn((name: string, attrs?: unknown) => log.push(['track', name, attrs])),
    page: vi.fn((path: string, attrs?: unknown) => log.push(['page', path, attrs])),
    flush: vi.fn(() => log.push(['flush'])),
    reset: vi.fn(() => log.push(['reset'])),
  }
  return instance
}

let analytics: AnalyticsModule
let instance: ReturnType<typeof fakeInstance>

function sdkScripts(): HTMLScriptElement[] {
  return Array.from(document.head.querySelectorAll<HTMLScriptElement>(`script[src="${SDK_URL}"]`))
}

// What the browser does once the injected tag has run: the SDK global exists
// and the script fires load.
function loadSdk(): void {
  window.twillingate = {
    get: vi.fn(() => undefined),
    create: vi.fn(() => instance),
  }
  sdkScripts()[0].dispatchEvent(new Event('load'))
}

function tracked(): string[] {
  return instance.log.filter(([call]) => call === 'track').map(([, name]) => name as string)
}

beforeEach(async () => {
  document.head.innerHTML = ''
  delete window.twillingate
  instance = fakeInstance()
  vi.resetModules()
  analytics = await import('./analytics')
})

afterEach(() => {
  delete window.twillingate
})

describe('loading the SDK', () => {
  it('injects nothing until the first capture', () => {
    analytics.setAnalyticsUser(USER)
    expect(sdkScripts()).toHaveLength(0)
  })

  it('injects the collector-served SDK once, dormant, as a named instance', () => {
    analytics.capture('a')
    analytics.capture('b')
    const scripts = sdkScripts()
    expect(scripts).toHaveLength(1)
    expect(scripts[0].dataset.instance).toBe('econumo')
    // Without data-key the tag does not auto-init; start() does.
    expect(scripts[0].dataset.key).toBeUndefined()
    expect(scripts[0].referrerPolicy).toBe('no-referrer')
  })

  it('initialises the instance identified, without consent and without automatic tracking', () => {
    analytics.setAnalyticsUser(USER)
    analytics.capture('a')
    loadSdk()
    expect(window.twillingate!.create).toHaveBeenCalledWith('econumo')
    expect(instance.init).toHaveBeenCalledTimes(1)
    const opts = instance.init.mock.calls[0][0] as Record<string, unknown>
    expect(opts.key).toMatch(/^ak_/)
    expect(opts.identity).toBe('identified')
    expect(opts.autoPageviews).toBe(false)
    expect(opts.taggedEvents).toBe(false)
    // Consent unlocks device storage; the SDK default (none) keeps nothing there.
    expect(opts).not.toHaveProperty('consent')
    expect(opts).not.toHaveProperty('storage')
  })

  it('reuses an instance another SDK copy already registered', () => {
    analytics.setAnalyticsUser(USER)
    analytics.capture('a')
    window.twillingate = { get: vi.fn(() => instance), create: vi.fn() }
    sdkScripts()[0].dispatchEvent(new Event('load'))
    expect(window.twillingate.create).not.toHaveBeenCalled()
    expect(tracked()).toEqual(['a'])
  })

  it('goes quiet when the script is blocked', () => {
    analytics.setAnalyticsUser(USER)
    analytics.capture('a')
    sdkScripts()[0].dispatchEvent(new Event('error'))
    expect(() => analytics.capture('b')).not.toThrow()
    expect(sdkScripts()).toHaveLength(1)
  })
})

describe('capture', () => {
  it('sends events captured before the script loaded, in order, once it has', () => {
    analytics.setAnalyticsUser(USER)
    analytics.capture('a')
    analytics.capture('b')
    expect(instance.track).not.toHaveBeenCalled()
    loadSdk()
    expect(tracked()).toEqual(['a', 'b'])
  })

  it('stamps the session context under the event properties at capture time', () => {
    analytics.setAnalyticsUser(USER)
    loadSdkAfterFirstCapture()
    analytics.setAnalyticsContext({ $app_version: 'v1.2.3', locale: 'en', mode: 'desktop' })
    analytics.capture('transaction_create', { current_url: 'https://h/x', mode: 'mobile' })
    analytics.setAnalyticsContext({ locale: 'de' })
    expect(instance.track).toHaveBeenLastCalledWith('transaction_create', {
      $app_version: 'v1.2.3',
      locale: 'en',
      mode: 'mobile',
      current_url: 'https://h/x',
    })
  })

  it('sends a page view as a view with the given raw path and masked attributes', () => {
    analytics.setAnalyticsUser(USER)
    loadSdkAfterFirstCapture()
    analytics.capturePageView('/account/1234', { $host: 'h', $path: '/account/:id', $referrer: null })
    expect(instance.page).toHaveBeenCalledWith('/account/1234', { $host: 'h', $path: '/account/:id', $referrer: null })
  })
})

describe('identity', () => {
  it('holds events captured before the user is known and sends them once the user resolves', () => {
    analytics.capture('boot_page_view')
    loadSdk()
    expect(instance.track).not.toHaveBeenCalled()

    analytics.setAnalyticsUser(USER)
    const calls = instance.log.map(([call]) => call)
    expect(calls.indexOf('identify')).toBeLessThan(calls.indexOf('track'))
    expect(instance.identify).toHaveBeenCalledWith(USER)
    expect(tracked()).toEqual(['boot_page_view'])
  })

  it('carries the group to the instance whenever it is set', () => {
    analytics.setAnalyticsGroup('a3f19c02b7d4', 'selfhosted_a3f19c02b7d4')
    analytics.setAnalyticsUser(USER)
    analytics.capture('a')
    loadSdk()
    expect(instance.group).toHaveBeenCalledWith('a3f19c02b7d4', 'selfhosted_a3f19c02b7d4')
  })

  // The SDK reads identity at flush time, so a batch still queued at an
  // identity change must go out before the change.
  it('flushes under the outgoing user before switching to a new one', () => {
    analytics.setAnalyticsUser('a'.repeat(32))
    loadSdkAfterFirstCapture()
    analytics.setAnalyticsUser('b'.repeat(32))
    const calls = instance.log.map(([call]) => call)
    expect(calls.lastIndexOf('flush')).toBeLessThan(calls.lastIndexOf('identify'))
    expect(instance.identify).toHaveBeenLastCalledWith('b'.repeat(32))
  })

  it('flushes, resets and keeps the group on logout', () => {
    analytics.setAnalyticsGroup('a3f19c02b7d4', 'selfhosted_a3f19c02b7d4')
    analytics.setAnalyticsUser(USER)
    loadSdkAfterFirstCapture()
    instance.log.length = 0

    analytics.resetAnalyticsIdentity()

    expect(instance.log).toEqual([
      ['flush'],
      ['reset'],
      // The group is the deployment, not the person, so it survives logout.
      ['group', 'a3f19c02b7d4', 'selfhosted_a3f19c02b7d4'],
    ])
  })

  it('sends nothing after logout until the next user resolves', () => {
    analytics.setAnalyticsUser('a'.repeat(32))
    loadSdkAfterFirstCapture()
    analytics.resetAnalyticsIdentity()
    analytics.capture('after_logout')
    expect(tracked()).not.toContain('after_logout')

    analytics.setAnalyticsUser('b'.repeat(32))
    expect(tracked()).toContain('after_logout')
    expect(instance.identify).toHaveBeenLastCalledWith('b'.repeat(32))
  })

  it('discards held events when identity is reset before a user resolved', () => {
    analytics.capture('held_then_expired')
    loadSdk()
    // A 401 on get-user-data: the token was dead, nobody ever resolved.
    analytics.resetAnalyticsIdentity()
    analytics.setAnalyticsUser(USER)
    analytics.capture('by_user')
    expect(tracked()).toEqual(['by_user'])
  })
})

// Loads the SDK through the only path that injects it: a first capture.
function loadSdkAfterFirstCapture(): void {
  analytics.capture('first')
  loadSdk()
}
