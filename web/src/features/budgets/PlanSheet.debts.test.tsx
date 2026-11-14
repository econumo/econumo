import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureElementDebt, fixtureEur, fixtureUsd, fixtureUser, fixtureWireBudget, fixtureWirePlan, planHandler } from '@/test/fixtures'
import type { BudgetPlanDto } from '@/api/dto/budget'
import { wireDebt } from '@/features/debts/testDebt'
import { BudgetPage } from './BudgetPage'
import { useBudgetPeriodStore } from './budgetStore'
import { useUiStore } from '@/app/uiStore'
import { makePlanExchange, planTotals } from './planMath'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})
vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

const userWithBudget = {
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b1' } : o)),
}

function mockViewport(tablet = false) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: tablet && q.includes('1023'), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/plan', element: <BudgetPage key="plan" mode="plan" /> }], { initialEntries: ['/plan'] })
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

// plan months May..Aug; the window shows Jun/Jul/Aug and today is in August.
// Food carries the active debt d1 from June on; Living paid off an older debt in May.
const foodDebt = (over: object) => fixtureElementDebt({ id: 'd1', payoffDate: '2028-03-05', basePayoffDate: '2028-10-05', ...over })
const debtPlan = (() => {
  const plan = JSON.parse(JSON.stringify(fixtureWirePlan)) as BudgetPlanDto
  const food = plan.structure.elements.find((el) => el.id === 'cat-food')!
  food.debt = foodDebt({ paymentsLeft: 21 })
  food.cells[1].debt = foodDebt({ paymentsLeft: 23 })
  food.cells[2].debt = foodDebt({ paymentsLeft: 22 })
  food.cells[3] = { actual: '0', planned: '500', debt: foodDebt({ paymentsLeft: 21, scheduledTotal: '1000' }) }
  const living = plan.structure.elements.find((el) => el.id === 'pe1')!
  living.cells[0].debt = fixtureElementDebt({ id: 'd0', isPaidOff: 1, paymentsLeft: 0, closingBalance: '0' })
  return plan
})()

function useHandlers(extra: Parameters<typeof server.use> = [], debts: unknown[] = [wireDebt]) {
  server.use(
    ...coreHandlers({ user: userWithBudget, debts }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(debtPlan),
    http.get('*/api/v1/budget/get-comment-list', () => HttpResponse.json({ success: true, message: '', data: { items: [], truncated: false } })),
    ...extra,
  )
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 7, 15, 12, 0, 0))
  vi.clearAllMocks()
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
  useUiStore.setState({ debtDetailsId: null, debtDetailsTab: 'overview' })
  useBudgetPeriodStore.setState({
    selectedDate: '2026-07-01',
    unfoldedElements: {},
    planUnfoldedElements: {},
    foldBudgetId: null,
    planFolds: {},
    planSumsShown: {},
  })
})

afterEach(() => {
  vi.useRealTimers()
})

it('has no Debts section: the debt shows on its element row among the expenses', async () => {
  useHandlers()
  renderPage()
  const expense = await screen.findByTestId('plan-section-expense')
  expect(screen.queryByTestId('plan-section-debts')).toBeNull()
  const food = expense.querySelector('[data-row-id="cat-food:1"]') as HTMLElement
  // the row's header carries the active debt's payoff
  expect(within(food).getByTestId('debt-payoff')).toHaveTextContent(/^Paid off by Mar 2028$/)
  // its cells keep the element's own figures, with the debt's payments left under them
  const jul = screen.getByTestId('plan-cell-cat-food:1')
  expect(within(jul).getByTestId('cell-actual')).toHaveTextContent('125.00')
  expect(within(jul).getByTestId('cell-actual')).not.toHaveAttribute('data-figure')
  expect(within(jul).getByTestId('cell-payments-left')).toHaveTextContent('22 payments left')
  // a row with no active debt has no payoff line; a paid-off debt's month outside the window shows nowhere
  const living = expense.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  expect(within(living).queryByTestId('debt-payoff')).toBeNull()
})

it('a cell shows the debt hint of its own month', async () => {
  useHandlers()
  renderPage()
  await screen.findByTestId('plan-section-expense')
  // August plans 500 below the 1,000 scheduled
  expect(within(screen.getByTestId('plan-cell-cat-food:2')).getByTestId('cell-planned').className).toContain('text-expense')
  // a past month is never red
  expect(within(screen.getByTestId('plan-cell-cat-food:1')).getByTestId('cell-planned').className).not.toContain('text-expense')
})

it('the expense totals are the elements\' own', async () => {
  useHandlers()
  renderPage()
  await screen.findByTestId('plan-section-expense')
  const plan = debtPlan
  const ex = makePlanExchange(plan, [fixtureUsd, fixtureEur])
  // the debt objects change nothing in the sums
  const withoutDebts = planTotals(fixtureWirePlan as unknown as BudgetPlanDto, ex)
  const withDebts = planTotals({ ...plan, structure: { ...plan.structure, elements: (fixtureWirePlan as unknown as BudgetPlanDto).structure.elements.map((el) => (el.id === 'cat-food' ? plan.structure.elements.find((e) => e.id === 'cat-food')! : el)) } }, ex)
  expect(withDebts[2].expenseActual).toBe(withoutDebts[2].expenseActual)
})

it('the row\'s menu offers the active debt\'s actions', async () => {
  useHandlers()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage()
  await screen.findByTestId('plan-section-expense')
  await user.click(await screen.findByRole('button', { name: 'menu Food' }))
  const items = (await screen.findAllByRole('menuitem')).map((i) => i.textContent)
  expect(items).toEqual(expect.arrayContaining(['Debt details', 'Mark as paid off', 'Delete debt']))
  expect(items).not.toContain('Track as debt')
})

it('on a tablet the cell\'s sheet has a Debt section whose Details opens the review', async () => {
  mockViewport(true)
  useHandlers()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage()
  await screen.findByTestId('plan-section-expense')
  await user.click(screen.getByTestId('plan-cell-cat-food:1'))
  const sheet = await screen.findByTestId('element-sheet')
  expect(within(sheet).getByTestId('sheet-figure-budget')).toBeInTheDocument()
  const debt = within(sheet).getByTestId('sheet-debt')
  expect(debt).toHaveTextContent('22 payments left')
  await user.click(within(debt).getByRole('button', { name: 'Details' }))
  expect(useUiStore.getState().debtDetailsId).toBe('d1')
  expect(useUiStore.getState().debtDetailsTab).toBe('overview')
})
