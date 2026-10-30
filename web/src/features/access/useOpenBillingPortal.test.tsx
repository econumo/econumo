import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { useOpenBillingPortal } from './useOpenBillingPortal'
import { METRICS, trackEvent } from '@/lib/metrics'
import { rememberAnalyticsPreference } from '@/lib/analyticsPreference'
import { fixtureUser } from '@/test/fixtures'

vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return { wrapper, queryClient }
}

function mockTab() {
  const tab = { location: { href: '' }, close: vi.fn() }
  window.open = vi.fn().mockReturnValue(tab)
  return tab
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  vi.mocked(trackEvent).mockClear()
})

const RAW_ID = '018f1e5c-0000-7000-8000-000000000001'

function serveUser(options: { name: string; value: string }[] = []) {
  server.use(
    http.get('*/api/v1/user/get-user-data', () =>
      HttpResponse.json({ success: true, message: '', data: { user: { ...fixtureUser, id: RAW_ID, options } } }),
    ),
  )
}

// The fragment is built from the user data the hook reads (on a warm reload,
// the persisted cache), so these tests click once that data is in.
async function renderWithUser() {
  const { wrapper, queryClient } = makeWrapper()
  const { result } = renderHook(() => useOpenBillingPortal(), { wrapper })
  await waitFor(() => expect(queryClient.getQueryData(['user'])).toBeDefined())
  return result
}

function decodeFragment(href: string): unknown {
  const value = new URL(href).hash.replace(/^#a=/, '')
  return JSON.parse(atob(value.replace(/-/g, '+').replace(/_/g, '/')))
}

// The portal reports into the same analytics project as this same user and
// group, handed over in a fragment so no server ever receives it.
it('hands the analytics user and group to the portal in the link fragment', async () => {
  server.use(
    http.post('*/api/v1/user/create-billing-link', () =>
      HttpResponse.json({ success: true, message: '', data: { url: 'https://pay.example.test/?t=abc' } }),
    ),
  )
  window.econumoConfig = { INSTANCE_ID: 'a3f19c02b7d4' }
  serveUser()
  const tab = mockTab()
  const result = await renderWithUser()
  result.current.open()
  await waitFor(() => expect(tab.location.href).toMatch(/^https:\/\/pay\.example\.test\/\?t=abc#a=[A-Za-z0-9_-]+$/))
  expect(decodeFragment(tab.location.href)).toEqual({
    u: '928709326e2884148b1dacddcfc5ea4c',
    g: 'a3f19c02b7d4',
    // jsdom serves localhost, which is not a cloud host.
    n: 'selfhosted_a3f19c02b7d4',
  })
})

it('hands nothing over for a user who opted out of analytics', async () => {
  server.use(
    http.post('*/api/v1/user/create-billing-link', () =>
      HttpResponse.json({ success: true, message: '', data: { url: 'https://pay.example.test/?t=abc' } }),
    ),
  )
  serveUser([{ name: 'analytics', value: '0' }])
  const tab = mockTab()
  const result = await renderWithUser()
  result.current.open()
  await waitFor(() => expect(tab.location.href).toBe('https://pay.example.test/?t=abc'))
})

// The device mirror alone also keeps it off (set on an earlier opt-out).
it('hands nothing over when this device remembers an opt-out', async () => {
  server.use(
    http.post('*/api/v1/user/create-billing-link', () =>
      HttpResponse.json({ success: true, message: '', data: { url: 'https://pay.example.test/?t=abc' } }),
    ),
  )
  serveUser()
  const tab = mockTab()
  const result = await renderWithUser()
  // After the user data, which rewrites the mirror from the server's option.
  rememberAnalyticsPreference(false)
  result.current.open()
  await waitFor(() => expect(tab.location.href).toBe('https://pay.example.test/?t=abc'))
})

it('mints a self link per click and points the pre-opened tab at it', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/create-billing-link', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { url: 'https://pay.example.test/?t=abc' } })
    }),
  )
  const tab = mockTab()
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useOpenBillingPortal(), { wrapper })
  result.current.open()
  await waitFor(() => expect(tab.location.href).toBe('https://pay.example.test/?t=abc'))
  expect(body).toEqual({})
  expect(trackEvent).toHaveBeenCalledWith(METRICS.SUBSCRIPTION_CTA_CLICK, expect.objectContaining({ target: 'self' }))
})

it('sends the partner id as the for hint and fires the partner metric', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/create-billing-link', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { url: 'https://pay.example.test/?t=abc&for=u2' } })
    }),
  )
  const tab = mockTab()
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useOpenBillingPortal(), { wrapper })
  result.current.open('u2')
  await waitFor(() => expect(tab.location.href).toBe('https://pay.example.test/?t=abc&for=u2'))
  expect(body).toEqual({ for: 'u2' })
  expect(trackEvent).toHaveBeenCalledWith(METRICS.SUBSCRIPTION_CTA_CLICK, expect.objectContaining({ target: 'partner' }))
})

it('closes the pre-opened tab and fires no metric when minting fails', async () => {
  server.use(
    http.post('*/api/v1/user/create-billing-link', () =>
      HttpResponse.json({ success: false, message: 'Billing is not configured', code: 400, errors: {} }, { status: 400 }),
    ),
  )
  const tab = mockTab()
  const { wrapper } = makeWrapper()
  const { result } = renderHook(() => useOpenBillingPortal(), { wrapper })
  result.current.open()
  await waitFor(() => expect(tab.close).toHaveBeenCalled())
  expect(trackEvent).not.toHaveBeenCalled()
})
