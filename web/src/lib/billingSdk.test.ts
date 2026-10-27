import { beforeEach, describe, expect, it, vi } from 'vitest'

const getBillingUrl = vi.fn<() => string>()
vi.mock('./config', () => ({ getBillingUrl: () => getBillingUrl() }))

const entry = { accessLevel: 'full' as const, accessUntil: '2027-01-01T00:00:00Z', registeredAt: '2025-01-01T00:00:00Z' }
const input = { locale: 'en', user: entry, connections: [] }
const banner = { id: 's1', text: 'Sale', cta: 'Buy', startsAt: '2026-11-01T00:00:00Z', endsAt: '2026-11-30T00:00:00Z' }

const scripts = () => Array.from(document.querySelectorAll<HTMLScriptElement>('script[src$="/sdk.js"]'))

async function load() {
  return (await import('./billingSdk')).getSaleBanner
}

describe('getSaleBanner', () => {
  beforeEach(() => {
    vi.resetModules()
    document.head.innerHTML = ''
    delete window.EconumoBilling
    getBillingUrl.mockReturnValue('https://billing.example')
  })

  it('returns null without touching the DOM when billing is disabled', async () => {
    getBillingUrl.mockReturnValue('')
    const getSaleBanner = await load()
    expect(await getSaleBanner(input)).toBeNull()
    expect(scripts()).toHaveLength(0)
  })

  it('injects the script once and resolves the banner after load', async () => {
    const getBanner = vi.fn().mockResolvedValue(banner)
    const getSaleBanner = await load()
    const first = getSaleBanner(input)
    const second = getSaleBanner(input)
    expect(scripts()).toHaveLength(1)
    expect(scripts()[0].src).toBe('https://billing.example/sdk.js')
    window.EconumoBilling = { getBanner }
    scripts()[0].dispatchEvent(new Event('load'))
    expect(await first).toEqual(banner)
    expect(await second).toEqual(banner)
    expect(getBanner).toHaveBeenCalledWith(input)
  })

  it('normalizes a trailing slash on the billing URL', async () => {
    getBillingUrl.mockReturnValue('https://billing.example/')
    const getSaleBanner = await load()
    void getSaleBanner(input)
    expect(scripts()[0].src).toBe('https://billing.example/sdk.js')
  })

  it('returns null when the script fails to load, without retrying', async () => {
    const getSaleBanner = await load()
    const pending = getSaleBanner(input)
    scripts()[0].dispatchEvent(new Event('error'))
    expect(await pending).toBeNull()
    expect(await getSaleBanner(input)).toBeNull()
    expect(scripts()).toHaveLength(1)
  })

  it('returns null when the global is missing', async () => {
    const getSaleBanner = await load()
    const pending = getSaleBanner(input)
    scripts()[0].dispatchEvent(new Event('load'))
    expect(await pending).toBeNull()
  })

  it('returns null when the global throws or rejects', async () => {
    const getSaleBanner = await load()
    const pending = getSaleBanner(input)
    window.EconumoBilling = { getBanner: () => { throw new Error('boom') } }
    scripts()[0].dispatchEvent(new Event('load'))
    expect(await pending).toBeNull()
    window.EconumoBilling = { getBanner: () => Promise.reject(new Error('boom')) }
    expect(await getSaleBanner(input)).toBeNull()
  })

  it('returns null when the result is missing a field', async () => {
    const getSaleBanner = await load()
    const pending = getSaleBanner(input)
    window.EconumoBilling = { getBanner: () => Promise.resolve({ ...banner, cta: undefined } as never) }
    scripts()[0].dispatchEvent(new Event('load'))
    expect(await pending).toBeNull()
  })
})
