import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { server } from '@/test/msw'
import { coreHandlers, fixtureWireBudget } from '@/test/fixtures'
import { PeriodStrip } from './PeriodStrip'
import { ExpenseWidget } from './ExpenseWidget'
import { useBudgetPeriodStore } from './budgetStore'
import type { BudgetDto } from '@/api/dto/budget'

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null })
})

it('strip clamps at the start month, marks active; click sets the period', async () => {
  const user = userEvent.setup()
  render(<PeriodStrip startedAt="2026-01-01 00:00:00" />)
  // months before the budget start are not offered (Jan..Jul + 23 ahead)
  const tabs = screen.getAllByRole('tab')
  expect(tabs[0]).toHaveTextContent('January')
  expect(screen.queryByRole('tab', { name: 'Dec 2025' })).not.toBeInTheDocument()
  expect(screen.getByRole('tab', { selected: true })).toHaveTextContent('July')
  await user.click(screen.getByRole('tab', { name: 'March' }))
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-03-01')
})

it('strip clamps at the end month for an ended budget but keeps the active tab', () => {
  useBudgetPeriodStore.setState({ selectedDate: '2026-09-01' })
  render(<PeriodStrip startedAt="2026-01-01 00:00:00" endedAt="2026-08-01 00:00:00" />)
  // Sep is only shown because it is the (stale) selection; Oct+ are gone
  expect(screen.getByRole('tab', { selected: true })).toHaveTextContent('September')
  expect(screen.queryByRole('tab', { name: 'October' })).not.toBeInTheDocument()
  const tabs = screen.getAllByRole('tab')
  expect(tabs[0]).toHaveTextContent('January')
})

it('strip extends the window forward but never past the budget bounds', () => {
  render(<PeriodStrip startedAt="2026-01-01 00:00:00" />)
  const strip = screen.getByRole('tablist')
  Object.defineProperty(strip, 'clientWidth', { value: 800, configurable: true })
  Object.defineProperty(strip, 'scrollWidth', { value: 4000, configurable: true })
  const initial = screen.getAllByRole('tab').length

  // the left edge is already clamped at the start month — no extension
  strip.scrollLeft = 100
  fireEvent.scroll(strip)
  expect(screen.getAllByRole('tab')).toHaveLength(initial)

  // open-ended budgets keep extending forward
  strip.scrollLeft = 3500
  fireEvent.scroll(strip)
  expect(screen.getAllByRole('tab')).toHaveLength(initial + 12)
  // the selected month is unchanged — only the rendered window grew
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-07-01')
})

it('mouse wheel scrolls the strip horizontally (the scrollbar is hidden)', () => {
  render(<PeriodStrip startedAt="2026-01-01 00:00:00" />)
  const strip = screen.getByRole('tablist')
  Object.defineProperty(strip, 'clientWidth', { value: 800, configurable: true })
  Object.defineProperty(strip, 'scrollWidth', { value: 4000, configurable: true })
  strip.scrollLeft = 1000
  fireEvent.wheel(strip, { deltaY: 240 })
  expect(strip.scrollLeft).toBe(1240)
  fireEvent.wheel(strip, { deltaX: -100, deltaY: 0 })
  expect(strip.scrollLeft).toBe(1140)
})

it('widget renders spent/total, progress and the conversion hint', async () => {
  server.use(...coreHandlers())
  const budget = JSON.parse(JSON.stringify(fixtureWireBudget)) as BudgetDto
  budget.balances[0] = { currencyId: 'cur-usd', startBalance: '100', endBalance: null, income: '400', expenses: '-450', exchanges: '-25', holdings: '30' }
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ExpenseWidget budget={budget} currencyId="cur-usd" />
    </QueryClientProvider>,
  )
  expect(screen.getByText('Spending progress')).toBeInTheDocument()
  expect(await screen.findByText('475.00 $')).toBeInTheDocument()
  expect(screen.getByText('530.00 $')).toBeInTheDocument()
  // budget currency = usd, selected = usd -> no conversion hint
  expect(screen.queryByText(/average rate/)).not.toBeInTheDocument()
})

it('widget shows the conversion hint for a non-base currency', async () => {
  server.use(...coreHandlers())
  const budget = JSON.parse(JSON.stringify(fixtureWireBudget)) as BudgetDto
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ExpenseWidget budget={budget} currencyId="cur-eur" />
    </QueryClientProvider>,
  )
  expect(await screen.findByText(/Average rate for Jul 2026: 1 USD = 0.9/)).toBeInTheDocument()
})
