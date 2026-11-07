import { render } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createAppQueryClient } from '@/lib/appQueryClient'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { ReportsPage } from './ReportsPage'

const envelope = (item: unknown) => HttpResponse.json({ success: true, message: '', data: { item } })

const thisYear = new Date().getFullYear()
export const months = [`${thisYear}-01-01`, `${thisYear}-02-01`, `${thisYear}-03-01`]

export const netWorthReport = {
  currencyId: 'cur-usd',
  months,
  accounts: [{ id: 'a1', name: 'Cash', icon: 'wallet', currencyId: 'cur-usd', ownerUserId: 'u1', balances: ['1000', '1800', '2500'] }],
}

export function reportHandlers(netWorth: unknown = netWorthReport) {
  return [
    http.get('*/api/v1/report/get-net-worth-report', () => envelope(netWorth)),
    // get-user-data's best-effort analytics probe
    http.get('*/api/v1/oauth/get-identity-list', () => HttpResponse.json({ success: true, message: '', data: [] })),
  ]
}

export function setViewport(phone: boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: phone && q.includes('max-width'), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

export function renderReports() {
  window.econumoConfig = { FEEDBACK: true, ...window.econumoConfig }
  // the app's client, so the report invalidation on writes is exercised
  const queryClient = createAppQueryClient()
  queryClient.setDefaultOptions({ queries: { ...queryClient.getDefaultOptions().queries, retry: false }, mutations: { retry: false } })
  const router = createMemoryRouter([{ path: '/reports', element: <ReportsPage /> }], { initialEntries: ['/reports'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return { queryClient, router }
}
