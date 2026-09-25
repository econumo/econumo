import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { useOpenBillingPortal } from './useOpenBillingPortal'
import { METRICS, trackEvent } from '@/lib/metrics'

vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

function makeWrapper() {
  const queryClient = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return { wrapper }
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
