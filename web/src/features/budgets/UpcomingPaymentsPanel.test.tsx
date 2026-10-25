import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { fixtureOwner, fixtureUsd } from '@/test/fixtures'
import { BudgetElementType } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import { UpcomingPaymentsPanel, type UpcomingQuery } from './UpcomingPaymentsPanel'

const upcomingRow = (over: Record<string, unknown>) => ({
  id: 'rt', author: fixtureOwner, currencyId: 'cur-usd', amount: '0', description: '',
  category: null, payee: null, tag: null, spentAt: '2026-07-20 00:00:00',
  templateId: 'rt', schedule: 'monthly', accountId: 'a1', accountName: 'Cash',
  labelNames: [], isNext: true, isOverdue: false, canEdit: true,
  ...over,
})

function serve(upcoming: unknown[]) {
  const seen: URL[] = []
  server.use(
    http.get('*/api/v1/budget/get-transaction-list', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json({ success: true, message: '', data: { items: [], upcoming } })
    }),
  )
  return seen
}

const query = (over: Partial<UpcomingQuery> = {}): UpcomingQuery => ({
  budget: { meta: { id: 'b1' } },
  target: { id: 'cat-food', type: BudgetElementType.CATEGORY, name: 'Food', icon: 'restaurant', currencyId: null },
  periodStart: '2026-07-01',
  currency: fixtureUsd as CurrencyDto,
  exchange: (_from, _to, amount) => amount,
  ...over,
})

function renderPanel(q: UpcomingQuery) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <UpcomingPaymentsPanel query={q} />
    </QueryClientProvider>,
  )
}

it('lists payments with their total', async () => {
  const seen = serve([
    upcomingRow({ id: 'r1', description: 'Rent', amount: '500' }),
    upcomingRow({ id: 'r2', payee: { id: 'p1', name: 'Gym' }, amount: '50', spentAt: '2026-07-25 00:00:00' }),
  ])
  renderPanel(query())
  expect(await screen.findByText('Rent')).toBeInTheDocument()
  expect(screen.getByText('Gym')).toBeInTheDocument()
  expect(screen.getByText('Recurring payments due')).toBeInTheDocument()
  expect(screen.getByText('Total 550.00')).toBeInTheDocument()
  expect(seen[0].searchParams.get('categoryId')).toBe('cat-food')
  expect(seen[0].searchParams.get('periodStart')).toBe('2026-07-01')
})

it('renders nothing when the list is empty', async () => {
  const seen = serve([])
  const { container } = renderPanel(query())
  await vi.waitFor(() => expect(seen).toHaveLength(1))
  await new Promise((r) => setTimeout(r, 20))
  expect(container).toBeEmptyDOMElement()
})

it('marks an overdue payment', async () => {
  serve([upcomingRow({ id: 'r1', description: 'Rent', amount: '500', isOverdue: true, spentAt: '2026-06-28 00:00:00' })])
  renderPanel(query())
  expect(await screen.findByText(/^Overdue · /)).toBeInTheDocument()
})

it('signs a savings row by direction and nets the total', async () => {
  serve([
    upcomingRow({ id: 'r1', description: 'Deposit', amount: '300', direction: 'in' }),
    upcomingRow({ id: 'r2', description: 'Withdrawal', amount: '100', direction: 'out' }),
  ])
  renderPanel(query({ target: { id: 'acc-s', type: BudgetElementType.SAVINGS, name: 'Savings', icon: 'savings', currencyId: 'cur-usd' } }))
  expect(await screen.findByText('Total 200.00')).toBeInTheDocument()
  expect(screen.getByText('-100.00')).toBeInTheDocument()
})

it('converts a row from its account currency into the cell currency', async () => {
  serve([
    upcomingRow({ id: 'r1', description: 'Rent', amount: '500' }),
    upcomingRow({ id: 'r2', description: 'Streaming', amount: '100', currencyId: 'cur-eur' }),
  ])
  const exchange = vi.fn((from: string, to: string, amount: string) => (from === 'cur-eur' && to === 'cur-usd' ? String(Number(amount) * 1.1) : amount))
  renderPanel(query({ exchange }))
  expect(await screen.findByText('110.00')).toBeInTheDocument()
  expect(screen.getByText('Total 610.00')).toBeInTheDocument()
  expect(exchange).toHaveBeenCalledWith('cur-eur', 'cur-usd', '100')
  // a row already in the cell currency is not exchanged
  expect(exchange).not.toHaveBeenCalledWith('cur-usd', 'cur-usd', '500')
})
