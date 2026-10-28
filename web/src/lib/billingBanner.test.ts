import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { getSaleBanner } from './billingBanner'

const backendHost = vi.fn<() => string>()
vi.mock('./config', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./config')>()),
  backendHost: () => backendHost(),
}))

const entry = { accessLevel: 'full' as const, accessUntil: '2026-12-10 00:00:00', registeredAt: '2025-01-01 00:00:00' }
const input = { locale: 'en', user: entry, connections: [entry] }
const banner = { id: 'bf', name: 'Black Friday', percent: 30, startsAt: '2026-11-27T00:00:00Z', endsAt: '2026-11-30T00:00:00Z' }
const ENDPOINT = 'https://billing.example/api/v1/public/get-offer'

function answer(data: unknown, status = 200) {
  return HttpResponse.json({ success: status < 400, message: '', data }, { status })
}

beforeEach(() => {
  window.econumoConfig = { BILLING_URL: 'https://billing.example' }
  backendHost.mockReturnValue(window.location.origin)
})

describe('getSaleBanner', () => {
  it('returns null without a request when billing is disabled', async () => {
    window.econumoConfig = {}
    let called = false
    server.use(http.post(ENDPOINT, () => ((called = true), answer({ banner }))))
    expect(await getSaleBanner(input)).toBeNull()
    expect(called).toBe(false)
  })

  it('posts the input to the public get-offer and returns the banner', async () => {
    let body: unknown
    server.use(
      http.post(ENDPOINT, async ({ request }) => {
        body = await request.json()
        return answer({ banner })
      }),
    )
    expect(await getSaleBanner(input)).toEqual(banner)
    expect(body).toEqual(input)
  })

  it('keeps a trailing slash and a query on BILLING_URL out of the path', async () => {
    window.econumoConfig = { BILLING_URL: 'https://billing.example/portal/?tenant=x' }
    let url = ''
    server.use(
      http.post('https://billing.example/portal/api/v1/public/get-offer', ({ request }) => {
        url = request.url
        return answer({ banner })
      }),
    )
    expect(await getSaleBanner(input)).toEqual(banner)
    expect(url).toBe('https://billing.example/portal/api/v1/public/get-offer?tenant=x')
  })

  it('returns null when billing has no banner', async () => {
    server.use(http.post(ENDPOINT, () => answer({ banner: null })))
    expect(await getSaleBanner(input)).toBeNull()
  })

  it('returns null on an error status, a network error, or a malformed banner', async () => {
    server.use(http.post(ENDPOINT, () => answer(null, 500)))
    expect(await getSaleBanner(input)).toBeNull()
    server.use(http.post(ENDPOINT, () => HttpResponse.error()))
    expect(await getSaleBanner(input)).toBeNull()
    server.use(http.post(ENDPOINT, () => answer({ banner: { ...banner, name: '' } })))
    expect(await getSaleBanner(input)).toBeNull()
    server.use(http.post(ENDPOINT, () => answer({ banner: { ...banner, percent: '30' } })))
    expect(await getSaleBanner(input)).toBeNull()
    server.use(http.post(ENDPOINT, () => answer({ banner: { ...banner, percent: 0 } })))
    expect(await getSaleBanner(input)).toBeNull()
  })

  it('returns null for an older billing without the endpoint, or a non-JSON answer', async () => {
    server.use(http.post(ENDPOINT, () => new HttpResponse('404 page not found', { status: 404 })))
    expect(await getSaleBanner(input)).toBeNull()
    server.use(http.post(ENDPOINT, () => new HttpResponse('<html>oops', { headers: { 'Content-Type': 'application/json' } })))
    expect(await getSaleBanner(input)).toBeNull()
  })

  it('gives up on a billing that never answers', async () => {
    // Stands in for the 10s timeout firing.
    vi.spyOn(AbortSignal, 'timeout').mockReturnValue(AbortSignal.abort(new DOMException('timed out', 'TimeoutError')))
    server.use(http.post(ENDPOINT, () => new Promise<never>(() => {})))
    expect(await getSaleBanner(input)).toBeNull()
    expect(AbortSignal.timeout).toHaveBeenCalledWith(10_000)
    vi.restoreAllMocks()
  })

  it('shows no sale when the SPA is pointed at a custom backend on another origin', async () => {
    backendHost.mockReturnValue('https://other-instance.example')
    let called = false
    server.use(http.post(ENDPOINT, () => ((called = true), answer({ banner }))))
    expect(await getSaleBanner(input)).toBeNull()
    expect(called).toBe(false)
  })
})
