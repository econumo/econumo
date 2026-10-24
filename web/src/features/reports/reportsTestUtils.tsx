import { render } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createAppQueryClient } from '@/lib/appQueryClient'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { ReportsPage } from './ReportsPage'

const envelope = (item: unknown) => HttpResponse.json({ success: true, message: '', data: { item } })

const thisYear = new Date().getFullYear()
export const months = [`${thisYear}-01-01`, `${thisYear}-02-01`, `${thisYear}-03-01`]

const row = (id: string, name: string, kind: string, amounts: string[]) => ({
  id, kind, name, icon: '', isArchived: 0, ownerUserId: null, amounts, planned: [], children: [],
})

export const flowReport = {
  currencyId: 'cur-usd',
  months,
  expense: [row('cat-food', 'Groceries', 'category', ['100', '150', '250']), row('cat-fun', 'Fun', 'category', ['50', '0', '0'])],
  income: [row('cat-salary', 'Salary', 'income_category', ['1000', '1000', '1000'])],
  transfersIn: ['0', '0', '0'],
  transfersOut: ['0', '0', '0'],
  savings: [],
}

export const netWorthReport = {
  currencyId: 'cur-usd',
  months,
  accounts: [{ id: 'a1', name: 'Cash', icon: 'wallet', currencyId: 'cur-usd', ownerUserId: 'u1', isDeleted: 0, balances: ['1000', '1800', '2500'] }],
}

export function reportHandlers(flow: unknown = flowReport, netWorth: unknown = netWorthReport) {
  return [
    http.get('*/api/v1/report/get-flow-report', () => envelope(flow)),
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
