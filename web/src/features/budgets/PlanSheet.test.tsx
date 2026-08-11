import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { delay, http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureEur, fixtureOwner, fixtureUsd, fixtureUser, fixtureWireBudget, fixtureWirePlan, planHandler } from '@/test/fixtures'
import type { BudgetPlanDto } from '@/api/dto/budget'
import { BudgetPage } from './BudgetPage'
import { useBudgetPeriodStore } from './budgetStore'
import { METRICS, trackEvent } from '@/lib/metrics'
import { balanceRow, makePlanExchange, planTotals } from './planMath'
import { moneyFormat } from '@/lib/money'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

const userWithBudget = {
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b1' } : o)),
}

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/budget', element: <BudgetPage /> }], { initialEntries: ['/budget'] })
  const result = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return { queryClient, router, ...result }
}

function usePlanHandlers() {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
  useBudgetPeriodStore.setState({
    selectedDate: '2026-07-01',
    unfoldedElements: {},
    foldBudgetId: null,
    budgetMode: 'budget',
    planFirstMonth: null,
    planFolds: {},
    planHideEmpty: false,
  })
})

it('toggle switches to plan mode and renders the sheet: months, income on top, cells', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByRole('tablist', { name: 'period' })
  await user.click(screen.getByRole('tab', { name: /plan/i }))
  await screen.findByText(/jul/i)
  const income = screen.getByTestId('plan-section-income')
  const firstExpense = screen.getByTestId('plan-section-expense')
  expect(income.compareDocumentPosition(firstExpense) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  const cell = screen.getAllByTestId('plan-cell-pe1:0')[0]
  expect(within(cell).getByTestId('cell-actual')).toBeInTheDocument()
  expect(within(cell).getByTestId('cell-planned')).toBeInTheDocument()
})

it('arrows shift the window by one month, clamped at the budget start', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-01-01' })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByText(/jan/i)
  const prev = screen.getByRole('button', { name: 'Earlier months' })
  const next = screen.getByRole('button', { name: 'Later months' })
  expect(prev).toBeDisabled()

  await user.click(next)
  await waitFor(() => expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-02-01'))
  expect(screen.getByRole('button', { name: 'Earlier months' })).not.toBeDisabled()

  await user.click(screen.getByRole('button', { name: 'Earlier months' }))
  await waitFor(() => expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-01-01'))
  expect(screen.getByRole('button', { name: 'Earlier months' })).toBeDisabled()
})

it('mode and first month persist in the store', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  const { unmount } = renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Later months' }))
  const monthAfterNav = useBudgetPeriodStore.getState().planFirstMonth
  expect(monthAfterNav).not.toBeNull()
  unmount()

  renderPage()
  expect(useBudgetPeriodStore.getState().budgetMode).toBe('plan')
  expect(useBudgetPeriodStore.getState().planFirstMonth).toBe(monthAfterNav)
  await screen.findByTestId('plan-sheet')
})

it('setBudgetMode fires BUDGET_PLAN_OPEN once switching to plan, not on a no-op switch', () => {
  useBudgetPeriodStore.getState().setBudgetMode('plan')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_OPEN)
  expect(trackEvent).toHaveBeenCalledTimes(1)
  useBudgetPeriodStore.getState().setBudgetMode('plan')
  expect(trackEvent).toHaveBeenCalledTimes(1)
})

it('setPlanFirstMonth fires BUDGET_PLAN_CHANGE_WINDOW', () => {
  useBudgetPeriodStore.getState().setPlanFirstMonth('2026-05-01')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_CHANGE_WINDOW)
})

it('editing a planned cell sends set-limit with the cell month and patches optimistically', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    // the response never resolves within the test, so the invalidated refetch (which
    // would otherwise revert to the unchanged mock data) can't race the optimistic patch
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      body = await request.json()
      await delay('infinite')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  // pe1's second visible column (Jul/Aug/Sep window -> Aug)
  const cell = screen.getByTestId('plan-cell-pe1:1')
  await user.click(within(cell).getByRole('button', { name: 'limit Living' }))
  const input = await screen.findByLabelText('Budget')
  await user.clear(input)
  await user.type(input, '350')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(body).toEqual({ budgetId: 'b1', elementId: 'pe1', period: '2026-08-01', amount: '350' }))
  expect(within(cell).getByTestId('cell-planned')).toHaveTextContent('350')
})

it('totals block renders income/expenses/net pairs and the running balance', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  const totalsBlock = screen.getByTestId('plan-totals')
  expect(within(totalsBlock).getByText('Income')).toBeInTheDocument()
  expect(within(totalsBlock).getByText('Expenses')).toBeInTheDocument()
  expect(within(totalsBlock).getByText('Net')).toBeInTheDocument()
  expect(within(totalsBlock).getByText('Balance')).toBeInTheDocument()

  // window is Jun/Jul/Aug (visible=3 in jsdom, firstMonth pinned above); the last
  // visible column (index 2) is Aug, the fixture's last fetched month (index 3),
  // so its running balance is the seed plus the FULL sum of effectiveNet across
  // all 4 fetched months — computed here via the math core, not hand-derived.
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const ex = makePlanExchange(plan, [fixtureUsd, fixtureEur])
  const totals = planTotals(plan, ex)
  const balance = balanceRow(plan, totals, ex)
  const expected = moneyFormat(balance[3], fixtureUsd, { showCurrency: false, useNativePrecision: false })

  expect(screen.getByTestId('plan-balance-2')).toHaveTextContent(expected)
})

it('uncategorized and child cells are not editable; guest role sees no editors', async () => {
  const guestBudget = {
    ...fixtureWireBudget,
    meta: { ...fixtureWireBudget.meta, access: [{ user: fixtureOwner, role: 'guest' as const, isAccepted: 1 as const }] },
  }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: guestBudget } })),
    planHandler(),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  // guest role: pe1 would normally be editable for the owner, but not here
  const pe1Cell = screen.getByTestId('plan-cell-pe1:1')
  expect(within(pe1Cell).queryByRole('button', { name: /limit/i })).not.toBeInTheDocument()

  // children never carry their own limit, regardless of role
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  await user.click(within(pe1Row).getByText('Living'))
  const childCell = await screen.findByTestId('plan-cell-cat-rent:1')
  expect(within(childCell).queryByRole('button')).not.toBeInTheDocument()

  // uncategorized rows are never editable, regardless of role
  for (const cell of screen.getAllByTestId('plan-cell-uncategorized:1')) {
    expect(within(cell).queryByRole('button', { name: /limit/i })).not.toBeInTheDocument()
  }
})
