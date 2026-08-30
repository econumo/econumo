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

it('strip offers months before the start (read-only history), marks active; click sets the period', async () => {
  const user = userEvent.setup()
  render(<PeriodStrip startedAt="2026-01-01 00:00:00" />)
  // pre-start months stay browsable so past spending can be reviewed
  expect(screen.getByRole('tab', { name: 'Dec 2025' })).toBeInTheDocument()
  expect(screen.getByRole('tab', { selected: true })).toHaveTextContent('July')
  await user.click(screen.getByRole('tab', { name: 'March' }))
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-03-01')
})

it('navigating to a pre-start month works and keeps it selectable', async () => {
  const user = userEvent.setup()
  render(<PeriodStrip startedAt="2026-06-01 00:00:00" />)
  const past = screen.getByRole('tab', { name: 'March' })
  expect(past).toBeInTheDocument()
  await user.click(past)
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-03-01')
})

it('desktop arrows step one month back and forward', async () => {
  const user = userEvent.setup()
  render(<PeriodStrip startedAt="2026-06-01 00:00:00" />)
  await user.click(screen.getByRole('button', { name: 'Previous month' }))
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-06-01')
  await user.click(screen.getByRole('button', { name: 'Next month' }))
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-07-01')
})

it('the back arrow steps into months before the budget start', async () => {
  const user = userEvent.setup()
  useBudgetPeriodStore.setState({ selectedDate: '2026-06-01' })
  render(<PeriodStrip startedAt="2026-06-01 00:00:00" />)
  await user.click(screen.getByRole('button', { name: 'Previous month' }))
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-05-01')
})

it('the forward arrow is disabled at an ended budget\'s end month', () => {
  useBudgetPeriodStore.setState({ selectedDate: '2026-08-01' })
  render(<PeriodStrip startedAt="2026-01-01 00:00:00" endedAt="2026-08-01 00:00:00" />)
  expect(screen.getByRole('button', { name: 'Next month' })).toBeDisabled()
  expect(screen.getByRole('button', { name: 'Previous month' })).toBeEnabled()
})

it('strip clamps at the end month for an ended budget but keeps the active tab', () => {
  useBudgetPeriodStore.setState({ selectedDate: '2026-09-01' })
  render(<PeriodStrip startedAt="2026-01-01 00:00:00" endedAt="2026-08-01 00:00:00" />)
  // Sep is only shown because it is the (stale) selection; Oct+ are gone
  expect(screen.getByRole('tab', { selected: true })).toHaveTextContent('September')
  expect(screen.queryByRole('tab', { name: 'October' })).not.toBeInTheDocument()
  // pre-start months remain browsable; only the end boundary truncates
  expect(screen.getByRole('tab', { name: 'Dec 2025' })).toBeInTheDocument()
  const tabs = screen.getAllByRole('tab')
  expect(tabs[tabs.length - 1]).toHaveTextContent('September')
})

it('strip extends both directions but never past the end month', () => {
  render(<PeriodStrip startedAt="2026-01-01 00:00:00" />)
  const strip = screen.getByRole('tablist')
  Object.defineProperty(strip, 'clientWidth', { value: 800, configurable: true })
  Object.defineProperty(strip, 'scrollWidth', { value: 4000, configurable: true })
  const initial = screen.getAllByRole('tab').length

  // the left edge keeps extending into the past (read-only history)
  strip.scrollLeft = 100
  fireEvent.scroll(strip)
  expect(screen.getAllByRole('tab')).toHaveLength(initial + 12)

  // open-ended budgets keep extending forward
  strip.scrollLeft = 3500
  fireEvent.scroll(strip)
  expect(screen.getAllByRole('tab')).toHaveLength(initial + 24)
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
