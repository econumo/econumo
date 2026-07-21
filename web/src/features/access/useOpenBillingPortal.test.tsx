import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { useOpenBillingPortal } from './useOpenBillingPortal'

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
  window.dataLayer = []
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
  expect(window.dataLayer).toContainEqual(expect.objectContaining({ event: 'appSubscriptionCtaClick' }))
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
  expect(window.dataLayer).toContainEqual(expect.objectContaining({ event: 'appSubscriptionPartnerCtaClick' }))
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
  expect(window.dataLayer).toEqual([])
})
