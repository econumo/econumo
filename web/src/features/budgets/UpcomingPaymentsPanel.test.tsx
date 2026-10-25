import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { fixtureOwner, fixtureUsd } from '@/test/fixtures'
import { BudgetElementType } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import { UpcomingPaymentsPanel, type UpcomingQuery } from './UpcomingPaymentsPanel'
import { wantsUpcomingPanel } from './planHint'

const upcomingRow = (over: Record<string, unknown>) => ({
  id: 'rt', author: fixtureOwner, currencyId: 'cur-usd', amount: '0', description: '',
  category: null, payee: null, tag: null, spentAt: '2026-07-20 00:00:00',
  templateId: 'rt', schedule: 'monthly', accountId: 'a1', accountName: 'Cash',
  labelNames: [], isNext: true, isOverdue: false, canEdit: true,
  ...over,
})

const itemRow = (over: Record<string, unknown>) => ({
  id: 't', author: fixtureOwner, currencyId: 'cur-usd', amount: '0', description: '',
  category: null, payee: null, tag: null, spentAt: '2026-07-10 00:00:00',
  ...over,
})

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 6, 15, 12))
})
afterEach(() => {
  vi.useRealTimers()
})

function serve(upcoming: unknown[], items: unknown[] = []) {
  const seen: URL[] = []
  server.use(
    http.get('*/api/v1/budget/get-transaction-list', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json({ success: true, message: '', data: { items, upcoming } })
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
  spent: '0',
  ...over,
})

function renderPanel(q: UpcomingQuery, onUse?: (amount: string) => void) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <UpcomingPaymentsPanel query={q} onUse={onUse} />
    </QueryClientProvider>,
  )
}

it('lists payments and offers spent + still due', async () => {
  const seen = serve([
    upcomingRow({ id: 'r1', description: 'Rent', amount: '500' }),
    upcomingRow({ id: 'r2', payee: { id: 'p1', name: 'Gym' }, amount: '50', spentAt: '2026-07-25 00:00:00' }),
  ])
  const onUse = vi.fn()
  renderPanel(query({ spent: '300', upcoming: '550' }), onUse)
  expect(await screen.findByText('Rent')).toBeInTheDocument()
  expect(screen.getByText('Gym')).toBeInTheDocument()
  expect(screen.getByText('Coming up')).toBeInTheDocument()
  expect(screen.getByTestId('upcoming-spent')).toHaveTextContent(/^Spent300\.00$/)
  expect(screen.getByTestId('upcoming-total')).toHaveTextContent(/^Total850\.00$/)
  expect(screen.getByRole('button', { name: 'Total 850.00' })).toBeInTheDocument()
  // a press anywhere on the list fills the editor
  fireEvent.click(screen.getByText('Gym'))
  expect(onUse).toHaveBeenCalledWith('850')
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

it('signs a savings row by direction', async () => {
  serve([
    upcomingRow({ id: 'r1', description: 'Deposit', amount: '300', direction: 'in' }),
    upcomingRow({ id: 'r2', description: 'Withdrawal', amount: '100', direction: 'out' }),
  ])
  renderPanel(query({ target: { id: 'acc-s', type: BudgetElementType.SAVINGS, name: 'Savings', icon: 'savings', currencyId: 'cur-usd' } }))
  expect(await screen.findByText('300.00')).toBeInTheDocument()
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
  expect(exchange).toHaveBeenCalledWith('cur-eur', 'cur-usd', '100')
  // a row already in the cell currency is not exchanged
  expect(exchange).not.toHaveBeenCalledWith('cur-usd', 'cur-usd', '500')
})

it('lists a real transaction dated later this month without counting it twice', async () => {
  serve(
    [upcomingRow({ id: 'r1', description: 'Rent', amount: '250', spentAt: '2026-07-25 00:00:00' })],
    [
      itemRow({ id: 't-past', description: 'Lunch', amount: '200', spentAt: '2026-07-10 13:00:00' }),
      itemRow({ id: 't-future', payee: { id: 'p1', name: 'Dentist' }, amount: '100', spentAt: '2026-07-20 09:00:00' }),
    ],
  )
  const onUse = vi.fn()
  // spent already holds both real rows
  renderPanel(query({ spent: '300', upcoming: '250' }), onUse)
  expect(await screen.findByText('Dentist')).toBeInTheDocument()
  expect(screen.queryByText('Lunch')).not.toBeInTheDocument()
  const names = screen.getAllByRole('listitem').map((li) => li.textContent)
  expect(names[0]).toContain('Dentist')
  expect(names[1]).toContain('Rent')
  fireEvent.click(screen.getByTestId('upcoming-total'))
  expect(onUse).toHaveBeenCalledWith('550')
})

it('lists future real transactions with nothing due', async () => {
  serve([], [itemRow({ id: 't-future', description: 'Concert', amount: '80', spentAt: '2026-07-28 20:00:00' })])
  renderPanel(query({ spent: '80', upcoming: '0' }), vi.fn())
  expect(await screen.findByText('Concert')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Total 80.00' })).toBeInTheDocument()
})

describe('wantsUpcomingPanel', () => {
  it('never for a past month', () => {
    expect(wantsUpcomingPanel('2026-06-01', '300', '250')).toBe(false)
  })
  it('for the current month or later with money spent or due', () => {
    expect(wantsUpcomingPanel('2026-07-01', '300', undefined)).toBe(true)
    expect(wantsUpcomingPanel('2026-08-01', '0', '250')).toBe(true)
    expect(wantsUpcomingPanel('2026-08-01', '0', '0')).toBe(false)
  })
})

it('shows every figure in the cell currency, a future-dated real row included', async () => {
  serve(
    [upcomingRow({ id: 'r1', description: 'Streaming', amount: '100', currencyId: 'cur-eur', spentAt: '2026-07-20 00:00:00' })],
    [itemRow({ id: 't1', description: 'Hotel', amount: '200', currencyId: 'cur-eur', spentAt: '2026-07-25 00:00:00' })],
  )
  const exchange = (from: string, to: string, amount: string) => (from === 'cur-eur' && to === 'cur-usd' ? String(Number(amount) * 1.1) : amount)
  renderPanel(query({ exchange, spent: '220', upcoming: '110' }), vi.fn())
  const [streaming, hotel] = await screen.findAllByRole('listitem')
  expect(streaming).toHaveTextContent('110.00')
  expect(hotel).toHaveTextContent('220.00')
  expect(screen.getByTestId('upcoming-total')).toHaveTextContent('330.00')
  expect(screen.queryByText('100.00')).not.toBeInTheDocument()
  expect(screen.queryByText('200.00')).not.toBeInTheDocument()
})
