import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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
import { balanceRow, formatPlanMonth, makePlanExchange, planTotals } from './planMath'
import { moneyFormat } from '@/lib/money'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

// jsdom cannot drive real dnd-kit pointer drags (no layout), so onDragEnd is
// captured here and fired directly with a synthetic {active, over} pair — the
// same shape dnd-kit itself would report. PlanSheet mounts one DndContext per
// band, income before expense, on every render — so this array only grows
// (never resets), but its LAST entry is always the current expense band's
// handler and the one before it the current income band's, regardless of how
// many renders happened first.
let capturedDragEnds: ((event: { active: { id: string }; over: { id: string } | null }) => void)[] = []
vi.mock('@dnd-kit/core', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@dnd-kit/core')>()
  return {
    ...actual,
    DndContext: ({ onDragEnd, children }: { onDragEnd: (event: never) => void; children: ReactNode }) => {
      capturedDragEnds.push(onDragEnd as never)
      return children
    },
  }
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

function mockCompactViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: true, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
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
  capturedDragEnds = []
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

it('totals block renders one effective value per cell for income/expenses/net, and the running balance', async () => {
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
  expect(within(screen.getByTestId('plan-balance-row')).getByText('Balance')).toBeInTheDocument()

  // window is Jun/Jul/Aug (visible=3 in jsdom, firstMonth pinned above); the last
  // visible column (index 2) is Aug, the fixture's last fetched month (index 3).
  // Both the totals rows and the running balance are computed here via the math
  // core, not hand-derived.
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const ex = makePlanExchange(plan, [fixtureUsd, fixtureEur])
  const totals = planTotals(plan, ex)
  const balance = balanceRow(plan, totals, ex)

  // index 2 is the uncategorized expense row slotted between Expenses and Net
  const rows = within(totalsBlock).getAllByRole('row')
  const [incomeRow, expensesRow, , netRow] = rows
  const augTotals = totals[2]

  expect(within(incomeRow).getByText(moneyFormat(augTotals.effectiveIncome, fixtureUsd, { showCurrency: false, useNativePrecision: false }))).toBeInTheDocument()
  expect(within(incomeRow).queryByTestId('cell-actual')).not.toBeInTheDocument()
  expect(within(incomeRow).queryByTestId('cell-planned')).not.toBeInTheDocument()

  expect(within(expensesRow).getByText(moneyFormat(augTotals.effectiveExpense, fixtureUsd, { showCurrency: false, useNativePrecision: false }))).toBeInTheDocument()
  expect(within(expensesRow).queryByTestId('cell-actual')).not.toBeInTheDocument()
  expect(within(expensesRow).queryByTestId('cell-planned')).not.toBeInTheDocument()

  expect(within(netRow).getByText(moneyFormat(augTotals.effectiveNet, fixtureUsd, { showCurrency: false, useNativePrecision: false }))).toBeInTheDocument()
  expect(within(netRow).queryByTestId('cell-actual')).not.toBeInTheDocument()
  expect(within(netRow).queryByTestId('cell-planned')).not.toBeInTheDocument()

  const expected = moneyFormat(balance[3], fixtureUsd, { showCurrency: false, useNativePrecision: false })
  expect(screen.getByTestId('plan-balance-2')).toHaveTextContent(expected)
})

it('folding a section header collapses its rows and persists', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  const { unmount } = renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Essentials' }))
  expect(document.querySelector('[data-row-id="pe1:0"]')).not.toBeInTheDocument()
  expect(useBudgetPeriodStore.getState().planFolds.bf1).toBe(true)

  unmount()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  expect(document.querySelector('[data-row-id="pe1:0"]')).not.toBeInTheDocument()
})

it('hide-empty removes dormant rows, shows the per-section count, Show reveals them', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  expect(document.querySelector('[data-row-id="cat-dormant:1"]')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitemcheckbox', { name: 'Hide empty rows' }))
  expect(document.querySelector('[data-row-id="cat-dormant:1"]')).not.toBeInTheDocument()
  expect(screen.getByText('1 hidden')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Show' }))
  expect(document.querySelector('[data-row-id="cat-dormant:1"]')).toBeInTheDocument()
  expect(screen.queryByText('1 hidden')).not.toBeInTheDocument()
})

it('the hide-empty toggle fires its metric once, not double-fired by the sheet', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  vi.mocked(trackEvent).mockClear()
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitemcheckbox', { name: 'Hide empty rows' }))
  expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_HIDE_EMPTY_TOGGLE)
  expect(trackEvent).toHaveBeenCalledTimes(1)
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
  // window May/Jun/Jul: both uncategorized rows have their spend inside it (income
  // actual at Jun, expense actual at May and Jul), so the assertions below actually
  // exercise both rows instead of silently matching zero or one of them
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-05-01' })
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

  // the uncategorized INCOME row is still an element row and is never editable; the
  // expense side is a totals line now, so only one element row carries this testid
  const uncatCells = screen.getAllByTestId('plan-cell-uncategorized:1')
  expect(uncatCells).toHaveLength(1)
  for (const cell of uncatCells) {
    expect(within(cell).queryByRole('button', { name: /limit/i })).not.toBeInTheDocument()
  }
})

it('scrolls a keyboard-selected row back into view, clearing the sticky balance row', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  const grid = await screen.findByTestId('plan-sheet')

  await user.click(screen.getByTestId('plan-cell-pe1:0'))

  // jsdom has no layout, so stage the geometry: a 200px-tall scroller whose sticky
  // balance row occupies the bottom 40px, and a selected cell sitting below both
  const rect = (top: number, bottom: number) => () => ({ top, bottom, height: bottom - top, left: 0, right: 0, width: 0, x: 0, y: top, toJSON: () => {} }) as DOMRect
  grid.getBoundingClientRect = rect(0, 200)
  const balance = screen.getByTestId('plan-balance-row')
  balance.getBoundingClientRect = rect(160, 200)
  const target = screen.getByTestId('plan-cell-cat-food:0')
  const targetCell = document.getElementById(target.id) as HTMLElement
  targetCell.getBoundingClientRect = rect(230, 260)

  grid.scrollTop = 0
  await user.click(screen.getByTestId('plan-cell-cat-food:0'))

  // 260 (cell bottom) - 160 (top of the sticky footer) = 100px of scrolling
  expect(grid.scrollTop).toBe(100)
})

it('arrow keys move the selection and shift the window at the edges', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  const grid = screen.getByTestId('plan-sheet')

  // window is Jun/Jul/Aug; select pe1's first visible column (Jun)
  await user.click(screen.getByTestId('plan-cell-pe1:0'))
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')

  // ArrowLeft from column 0 selects the name cell (col -1) first — see the dedicated
  // name-cell-reachability test for that step in isolation; ArrowLeft again, now at
  // -1, shifts the window back a month, keeping the row selected
  grid.focus()
  await user.keyboard('{ArrowLeft}{ArrowLeft}')
  await waitFor(() => expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-05-01'))
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  expect(within(pe1Row).getByTitle('Living').closest('[role="gridcell"]')).toHaveAttribute('aria-selected', 'true')

  // ArrowRight returns to the first month column of the now-shifted window
  grid.focus()
  await user.keyboard('{ArrowRight}')
  expect(await screen.findByTestId('plan-cell-pe1:0')).toHaveAttribute('data-month', '2026-05-01')
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')

  // ArrowDown walks to the next data row (pe1 is the only row in the Essentials folder,
  // so the flat list's next entry is the first loose expense row, Food)
  grid.focus()
  await user.keyboard('{ArrowDown}')
  expect(screen.getByTestId('plan-cell-cat-food:0')).toHaveAttribute('aria-selected', 'true')
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'false')

  // ArrowRight twice reaches the last visible column (col 2 of 3) without shifting
  grid.focus()
  await user.keyboard('{ArrowRight}{ArrowRight}')
  expect(screen.getByTestId('plan-cell-cat-food:2')).toHaveAttribute('aria-selected', 'true')
  expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-05-01')

  // a third ArrowRight from the last column shifts the window forward, keeping row+col
  grid.focus()
  await user.keyboard('{ArrowRight}')
  await waitFor(() => expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-06-01'))
  expect(await screen.findByTestId('plan-cell-cat-food:2')).toHaveAttribute('aria-selected', 'true')
})

it('Enter opens the editor on an editable cell and is inert on read-only cells', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  const grid = screen.getByTestId('plan-sheet')

  await user.click(screen.getByTestId('plan-cell-pe1:1'))
  grid.focus()
  await user.keyboard('{Enter}')
  expect(await screen.findByLabelText('Budget')).toBeInTheDocument()
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByLabelText('Budget')).not.toBeInTheDocument())

  // uncategorized rows are never editable, regardless of role
  const uncatCell = screen.getAllByTestId('plan-cell-uncategorized:1')[0]
  await user.click(uncatCell)
  grid.focus()
  await user.keyboard('{Enter}')
  expect(screen.queryByLabelText('Budget')).not.toBeInTheDocument()

  // children never carry their own limit
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  await user.click(within(pe1Row).getByText('Living'))
  const childCell = await screen.findByTestId('plan-cell-cat-rent:1')
  await user.click(childCell)
  grid.focus()
  await user.keyboard('{Enter}')
  expect(screen.queryByLabelText('Budget')).not.toBeInTheDocument()
})

it('cells expose the aria label and aria-selected', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  // window is Jun/Jul/Aug; pe1's second visible column is Jul (actual 45, planned 250)
  const cell = screen.getByTestId('plan-cell-pe1:1')
  expect(cell).toHaveAttribute('role', 'gridcell')
  expect(cell).toHaveAttribute('aria-selected', 'false')
  const monthLabel = formatPlanMonth('2026-07-01', 'en')
  const actualLabel = moneyFormat('45', fixtureUsd, { showCurrency: false, useNativePrecision: false })
  const plannedLabel = moneyFormat('250', fixtureUsd, { showCurrency: false, useNativePrecision: false })
  expect(cell).toHaveAttribute('aria-label', `Living, ${monthLabel}: actual ${actualLabel}, planned ${plannedLabel}`)

  await user.click(cell)
  expect(cell).toHaveAttribute('aria-selected', 'true')
})

it('keystrokes inside the popover editor reach it, not the grid: ArrowLeft moves the caret and Enter commits set-limit', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      body = await request.json()
      await delay('infinite')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  const grid = screen.getByTestId('plan-sheet')

  // pe1's second visible column (Jun/Jul/Aug window -> Jul)
  await user.click(screen.getByTestId('plan-cell-pe1:1'))
  grid.focus()
  await user.keyboard('{Enter}')
  const input = await screen.findByLabelText('Budget')

  // ArrowLeft while the popover input has focus must move the caret, not the
  // grid's window/selection — the grid must not intercept it.
  const monthBefore = useBudgetPeriodStore.getState().planFirstMonth
  await user.clear(input)
  await user.type(input, '12{ArrowLeft}3')
  expect(input).toHaveValue('132')
  expect(useBudgetPeriodStore.getState().planFirstMonth).toBe(monthBefore)

  // Enter inside the input must submit the form (commit), not be swallowed by
  // the grid's own Enter handling.
  await user.keyboard('{Enter}')
  await waitFor(() => expect(body).toEqual({ budgetId: 'b1', elementId: 'pe1', period: '2026-07-01', amount: '132' }))
})

it('ArrowLeft reaches the name cell (col -1) by keyboard, Enter there toggles expansion, and ArrowLeft again shifts the window', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  const grid = screen.getByTestId('plan-sheet')

  await user.click(screen.getByTestId('plan-cell-pe1:0'))
  grid.focus()
  await user.keyboard('{ArrowLeft}')
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  const nameCell = within(pe1Row).getByTitle('Living').closest('[role="gridcell"]') as HTMLElement
  expect(nameCell).toHaveAttribute('aria-selected', 'true')
  expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-06-01')

  // Enter on the name cell toggles the row's expansion (pe1/Living has children)
  expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument()
  await user.keyboard('{Enter}')
  expect(await screen.findByTestId('plan-cell-cat-rent:0')).toBeInTheDocument()

  // ArrowLeft again, still at -1, shifts the window back a month and keeps the selection at -1
  await user.keyboard('{ArrowLeft}')
  await waitFor(() => expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-05-01'))
  expect(within(pe1Row).getByTitle('Living').closest('[role="gridcell"]')).toHaveAttribute('aria-selected', 'true')
})

it('budget-mode envelope dialog still offers expense categories only', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  await user.click(await screen.findByRole('button', { name: 'create envelope Default folder' }))

  const dialog = await screen.findByRole('dialog', { name: 'New envelope' })
  expect(within(dialog).getByText('Food')).toBeInTheDocument()
  expect(within(dialog).queryByText('Salary')).not.toBeInTheDocument()
})

it('clicking a cell focuses the grid so arrow keys work immediately, no manual focus needed', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  await user.click(screen.getByTestId('plan-cell-pe1:0'))
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')

  // no grid.focus() call here — the click itself must have focused the grid
  await user.keyboard('{ArrowRight}')
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('aria-selected', 'true')
})

it('the selected cell gets a visible focus ring', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  const cell = screen.getByTestId('plan-cell-pe1:0')
  expect(cell.className).not.toMatch(/ring-2/)
  await user.click(cell)
  expect(cell.className).toMatch(/ring-2/)
})

it('grid structure: sections are rowgroups, the month header sits outside the grid, and aria-activedescendant tracks the selection', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  const grid = screen.getByTestId('plan-sheet')
  expect(grid).toHaveAttribute('role', 'grid')
  expect(screen.getByTestId('plan-section-income')).toHaveAttribute('role', 'rowgroup')
  expect(screen.getByTestId('plan-section-expense')).toHaveAttribute('role', 'rowgroup')
  expect(grid).not.toHaveAttribute('aria-activedescendant')

  const monthHeader = screen.getAllByRole('columnheader')[0]
  expect(grid.contains(monthHeader)).toBe(false)

  const cell = screen.getByTestId('plan-cell-pe1:0')
  await user.click(cell)
  expect(cell.id).not.toBe('')
  expect(grid).toHaveAttribute('aria-activedescendant', cell.id)
})

it('a failed plan fetch shows the error state instead of a blank area, and retry recovers', async () => {
  let hits = 0
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    http.get('*/api/v1/budget/get-budget-plan', () => {
      hits += 1
      return hits === 1
        ? HttpResponse.json({ success: false, message: 'boom', code: 0, exceptionType: 'x' }, { status: 500 })
        : HttpResponse.json({ success: true, message: '', data: { item: fixtureWirePlan } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-error')

  await user.click(screen.getByRole('button', { name: 'Try again' }))
  await screen.findByTestId('plan-sheet')
})

it('ArrowLeft at the name cell does not page the window past the budget start', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-01-01' })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  const grid = screen.getByTestId('plan-sheet')

  await user.click(screen.getByTestId('plan-cell-pe1:0'))
  grid.focus()
  await user.keyboard('{ArrowLeft}')
  expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-01-01')

  // at the name cell AND already at the budget start: a further ArrowLeft must not
  // page earlier (the prev nav button is disabled here too — same clamp, keyboard path)
  await user.keyboard('{ArrowLeft}')
  expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-01-01')
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  expect(within(pe1Row).getByTitle('Living').closest('[role="gridcell"]')).toHaveAttribute('aria-selected', 'true')
})

it('a folder with no elements still renders as a header-only folder in the expense area', async () => {
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const planWithEmptyFolder: BudgetPlanDto = {
    ...plan,
    structure: { ...plan.structure, folders: [...plan.structure.folders, { id: 'bf-empty', name: 'Empty Folder', position: 5 }] },
  }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(planWithEmptyFolder),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  expect(screen.getByTestId('plan-folder-bf-empty')).toBeInTheDocument()
  expect(within(screen.getByTestId('plan-section-expense')).getByText('Empty Folder')).toBeInTheDocument()
})

describe('fill handle', () => {
  beforeEach(() => {
    HTMLElement.prototype.setPointerCapture ??= () => {}
    HTMLElement.prototype.releasePointerCapture ??= () => {}
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      width: 110,
      height: 0,
      top: 0,
      left: 0,
      right: 0,
      bottom: 0,
      x: 0,
      y: 0,
      toJSON: () => {},
    } as DOMRect)
  })

  it('renders on any selected editable cell, including one with no limit set', async () => {
    usePlanHandlers()
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', { name: /plan/i }))
    await screen.findByTestId('plan-sheet')

    // window is Jun/Jul/Aug; pe1's col0 (Jun) has planned '200'
    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    expect(screen.getByTestId('plan-cell-pe1:0')).toContainElement(screen.getByTestId('fill-handle'))

    // uncategorized rows are never editable
    const uncatCell = screen.getAllByTestId('plan-cell-uncategorized:0')[0]
    await user.click(uncatCell)
    expect(screen.queryByTestId('fill-handle')).not.toBeInTheDocument()

    // pe1's col2 (Aug) has planned '' — it still renders (and edits) as 0.00, so it
    // is draggable too; gating on a set limit hid the handle on every 0.00 cell
    await user.click(screen.getByTestId('plan-cell-pe1:2'))
    expect(screen.getByTestId('plan-cell-pe1:2')).toContainElement(screen.getByTestId('fill-handle'))
  })

  it('dragging a cell with no limit set copies an explicit 0, not an empty amount', async () => {
    const bodies: unknown[] = []
    server.use(
      ...coreHandlers({ user: userWithBudget }),
      http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
      planHandler(),
      http.post('*/api/v1/budget/set-limit', async ({ request }) => {
        bodies.push(await request.json())
        await delay('infinite')
        return HttpResponse.json({ success: true, message: '', data: {} })
      }),
    )
    // window is May/Jun/Jul; tag1's col0 (May) has planned '' — it displays 0.00
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-05-01' })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', { name: /plan/i }))
    await screen.findByTestId('plan-sheet')

    await user.click(screen.getByTestId('plan-cell-tag1:0'))
    const handle = screen.getByTestId('fill-handle')

    fireEvent.pointerDown(handle, { clientX: 100, pointerId: 1 })
    fireEvent.pointerMove(handle, { clientX: 210, pointerId: 1 })
    fireEvent.pointerUp(handle, { pointerId: 1 })

    await waitFor(() => expect(bodies.length).toBeGreaterThan(0))
    bodies.forEach((b) => expect(b).toMatchObject({ amount: '0' }))
  })

  it('drag right copies the value into covered months, one set-limit per month', async () => {
    const bodies: unknown[] = []
    server.use(
      ...coreHandlers({ user: userWithBudget }),
      http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
      planHandler(),
      // the response never resolves within the test, so the invalidated refetch (which
      // would otherwise revert to the unchanged mock data) can't race the optimistic patch
      http.post('*/api/v1/budget/set-limit', async ({ request }) => {
        bodies.push(await request.json())
        await delay('infinite')
        return HttpResponse.json({ success: true, message: '', data: {} })
      }),
    )
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', { name: /plan/i }))
    await screen.findByTestId('plan-sheet')

    // window is Jun/Jul/Aug; drag pe1's col0 (Jun, planned '200') two columns right
    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    const handle = screen.getByTestId('fill-handle')

    fireEvent.pointerDown(handle, { clientX: 100, pointerId: 1 })
    fireEvent.pointerMove(handle, { clientX: 320, pointerId: 1 })
    const jul = screen.getByTestId('plan-cell-pe1:1')
    const aug = screen.getByTestId('plan-cell-pe1:2')
    expect(jul.className).toContain('fill-covered')
    expect(aug.className).toContain('fill-covered')
    fireEvent.pointerUp(handle, { clientX: 320, pointerId: 1 })

    await waitFor(() => expect(bodies).toHaveLength(2))
    expect(bodies).toEqual(
      expect.arrayContaining([
        { budgetId: 'b1', elementId: 'pe1', period: '2026-07-01', amount: '200' },
        { budgetId: 'b1', elementId: 'pe1', period: '2026-08-01', amount: '200' },
      ]),
    )
    expect(within(jul).getByTestId('cell-planned')).toHaveTextContent('200')
    expect(within(aug).getByTestId('cell-planned')).toHaveTextContent('200')

    // releasing without moving (targetCol === startCol) posts nothing
    bodies.length = 0
    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    const handle2 = screen.getByTestId('fill-handle')
    fireEvent.pointerDown(handle2, { clientX: 100, pointerId: 2 })
    fireEvent.pointerUp(handle2, { clientX: 100, pointerId: 2 })
    expect(bodies).toHaveLength(0)
  })

  it('Escape during the drag cancels without any request', async () => {
    const bodies: unknown[] = []
    server.use(
      ...coreHandlers({ user: userWithBudget }),
      http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
      planHandler(),
      http.post('*/api/v1/budget/set-limit', async ({ request }) => {
        bodies.push(await request.json())
        return HttpResponse.json({ success: true, message: '', data: {} })
      }),
    )
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', { name: /plan/i }))
    await screen.findByTestId('plan-sheet')

    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    const grid = screen.getByTestId('plan-sheet')
    grid.focus()
    const handle = screen.getByTestId('fill-handle')

    fireEvent.pointerDown(handle, { clientX: 100, pointerId: 1 })
    fireEvent.pointerMove(handle, { clientX: 210, pointerId: 1 })
    expect(screen.getByTestId('plan-cell-pe1:1').className).toContain('fill-covered')

    fireEvent.keyDown(grid, { key: 'Escape' })
    expect(screen.getByTestId('plan-cell-pe1:1').className).not.toContain('fill-covered')

    fireEvent.pointerUp(handle, { clientX: 210, pointerId: 1 })
    expect(bodies).toHaveLength(0)
  })

  it('ArrowRight mid-drag is swallowed: selection/window do not shift, drag state survives', async () => {
    const bodies: unknown[] = []
    server.use(
      ...coreHandlers({ user: userWithBudget }),
      http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
      planHandler(),
      http.post('*/api/v1/budget/set-limit', async ({ request }) => {
        bodies.push(await request.json())
        return HttpResponse.json({ success: true, message: '', data: {} })
      }),
    )
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', { name: /plan/i }))
    await screen.findByTestId('plan-sheet')

    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    const grid = screen.getByTestId('plan-sheet')
    grid.focus()
    const handle = screen.getByTestId('fill-handle')

    fireEvent.pointerDown(handle, { clientX: 100, pointerId: 1 })
    fireEvent.pointerMove(handle, { clientX: 210, pointerId: 1 })
    expect(screen.getByTestId('plan-cell-pe1:1').className).toContain('fill-covered')

    fireEvent.keyDown(grid, { key: 'ArrowRight' })
    // still mid-drag: the covered range is unchanged and the window has not paged
    expect(screen.getByTestId('plan-cell-pe1:1').className).toContain('fill-covered')
    expect(screen.getByTestId('plan-cell-pe1:2').className).not.toContain('fill-covered')
    expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-06-01')

    fireEvent.pointerUp(handle, { clientX: 210, pointerId: 1 })
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0]).toMatchObject({ budgetId: 'b1', elementId: 'pe1', period: '2026-07-01', amount: '200' })
  })

  it('drag far right clamps at the last visible column and posts exactly that many requests', async () => {
    const bodies: unknown[] = []
    server.use(
      ...coreHandlers({ user: userWithBudget }),
      http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
      planHandler(),
      http.post('*/api/v1/budget/set-limit', async ({ request }) => {
        bodies.push(await request.json())
        return HttpResponse.json({ success: true, message: '', data: {} })
      }),
    )
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', { name: /plan/i }))
    await screen.findByTestId('plan-sheet')

    // window is Jun/Jul/Aug (3 visible columns, jsdom floors to the width=0 case);
    // drag pe1's col0 with a huge deltaX that would target far past the last column
    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    const handle = screen.getByTestId('fill-handle')

    fireEvent.pointerDown(handle, { clientX: 100, pointerId: 1 })
    fireEvent.pointerMove(handle, { clientX: 100_000, pointerId: 1 })
    const jul = screen.getByTestId('plan-cell-pe1:1')
    const aug = screen.getByTestId('plan-cell-pe1:2')
    expect(jul.className).toContain('fill-covered')
    expect(aug.className).toContain('fill-covered')
    fireEvent.pointerUp(handle, { clientX: 100_000, pointerId: 1 })

    // lastVisibleCol (2) - startCol (0) = 2 requests, not one per pixel of drag
    await waitFor(() => expect(bodies).toHaveLength(2))
    expect(bodies).toEqual(
      expect.arrayContaining([
        { budgetId: 'b1', elementId: 'pe1', period: '2026-07-01', amount: '200' },
        { budgetId: 'b1', elementId: 'pe1', period: '2026-08-01', amount: '200' },
      ]),
    )
  })

  it('opening the LimitEditor popover then dragging the fill handle dismisses it, and the drag still commits', async () => {
    const bodies: unknown[] = []
    server.use(
      ...coreHandlers({ user: userWithBudget }),
      http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
      planHandler(),
      http.post('*/api/v1/budget/set-limit', async ({ request }) => {
        bodies.push(await request.json())
        return HttpResponse.json({ success: true, message: '', data: {} })
      }),
    )
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', { name: /plan/i }))
    await screen.findByTestId('plan-sheet')

    const cell = screen.getByTestId('plan-cell-pe1:0')
    await user.click(cell)
    await user.click(within(cell).getByRole('button', { name: 'limit Living' }))
    expect(document.querySelector('[data-slot="popover-content"]')).toBeInTheDocument()
    // Radix's DismissableLayer registers its document-level pointerdown listener in a
    // setTimeout(0) after the layer mounts, so the very next synchronous event misses it
    await new Promise((resolve) => setTimeout(resolve, 0))

    const handle = screen.getByTestId('fill-handle')
    fireEvent.pointerDown(handle, { clientX: 100, pointerId: 1 })
    fireEvent.pointerMove(handle, { clientX: 320, pointerId: 1 })
    expect(screen.getByTestId('plan-cell-pe1:1').className).toContain('fill-covered')
    fireEvent.pointerUp(handle, { clientX: 320, pointerId: 1 })
    // Popover defers outside-pointerdown dismissal to the click that follows it (so a
    // drag-select doesn't dismiss mid-gesture) — that detection needs the pointerdown to
    // actually reach Radix's document listener, which the deleted stopPropagation used to
    // block. A neutral click (not on a grid cell, so this assertion isn't riding on the
    // unrelated cell-focus side effect of ctx.select) stands in for wherever the drag's
    // real mouseup/click ultimately lands.
    fireEvent.click(document.body)
    // Radix's DismissableLayer dismisses the popover through this sequence — it must not
    // be swallowed by a stopPropagation on the handle's pointerdown listener
    await waitFor(() => expect(document.querySelector('[data-slot="popover-content"]')).not.toBeInTheDocument())

    await waitFor(() => expect(bodies).toHaveLength(2))
    expect(bodies).toEqual(
      expect.arrayContaining([
        { budgetId: 'b1', elementId: 'pe1', period: '2026-07-01', amount: '200' },
        { budgetId: 'b1', elementId: 'pe1', period: '2026-08-01', amount: '200' },
      ]),
    )
  })

  it('compact mode: the fill handle does not render on a selected editable cell', async () => {
    mockCompactViewport()
    usePlanHandlers()
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', { name: /plan/i }))
    await screen.findByTestId('plan-sheet')

    // window is Jun/Jul/Aug; pe1's col0 (Jun) has planned '200' — the exact cell
    // that shows the handle in wide mode
    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    expect(screen.queryByTestId('fill-handle')).not.toBeInTheDocument()
  })
})

describe('income/expense split', () => {
  it('renders a foldable Expenses header that collapses the whole expense area', async () => {
    usePlanHandlers()
    // window Jun/Jul/Aug: both uncategorized rows have their spend inside it (income
    // actual at Jun, expense actual at Jul), so neither is hidden by the zero-spend filter
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', { name: /plan/i }))
    await screen.findByTestId('plan-sheet')

    const expenseSection = screen.getByTestId('plan-section-expense')
    const expensesButton = within(expenseSection).getByRole('button', { name: 'Expenses' })

    // expense rows present before folding: a folder row and a loose row (the
    // uncategorized expense figure is a totals line, not a row in this section)
    expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()
    expect(document.querySelector('[data-row-id="cat-food:1"]')).toBeInTheDocument()
    // income rows present too
    expect(document.querySelector('[data-row-id="ie1:4"]')).toBeInTheDocument()

    await user.click(expensesButton)
    expect(document.querySelector('[data-row-id="pe1:0"]')).not.toBeInTheDocument()
    expect(document.querySelector('[data-row-id="cat-food:1"]')).not.toBeInTheDocument()
    // the uncategorized expense figure is a totals line, unaffected by the fold
    expect(within(screen.getByTestId('plan-totals')).getByText('Uncategorized')).toBeInTheDocument()
    // income unaffected by the expense fold
    expect(document.querySelector('[data-row-id="ie1:4"]')).toBeInTheDocument()

    await user.click(expensesButton)
    expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()
    expect(document.querySelector('[data-row-id="cat-food:1"]')).toBeInTheDocument()

    // keyboard nav must match: with the expense section folded, ArrowDown from the
    // last income row must not reach the (now excluded) expense rows
    const lastIncomeRow = document.querySelector('[data-row-id="uncategorized:3"]') as HTMLElement
    const lastIncomeCell = within(lastIncomeRow).getByTestId('plan-cell-uncategorized:0')
    await user.click(lastIncomeCell)
    expect(lastIncomeCell).toHaveAttribute('aria-selected', 'true')

    await user.click(expensesButton)
    const grid = screen.getByTestId('plan-sheet')
    grid.focus()
    await user.keyboard('{ArrowDown}')
    expect(lastIncomeCell).toHaveAttribute('aria-selected', 'true')
    expect(document.querySelector('[data-row-id="pe1:0"]')).not.toBeInTheDocument()
  })

  it('hide-empty count for expense loose rows sits on the Expenses header', async () => {
    usePlanHandlers()
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', { name: /plan/i }))
    await screen.findByTestId('plan-sheet')

    await user.click(screen.getByRole('button', { name: 'Configure' }))
    await user.click(await screen.findByRole('menuitemcheckbox', { name: 'Hide empty rows' }))
    expect(document.querySelector('[data-row-id="cat-dormant:1"]')).not.toBeInTheDocument()

    const expenseSection = screen.getByTestId('plan-section-expense')
    const header = within(expenseSection).getByRole('button', { name: 'Expenses' }).parentElement as HTMLElement
    const hiddenNotice = within(header).getByText('1 hidden')

    // it sits in the header, not as a trailing line after the last visible row
    // (uncategorized now renders in the totals block, so anchor on a loose row)
    const lastRow = within(expenseSection).getByTestId('plan-cell-cat-food:0')
    expect(hiddenNotice.compareDocumentPosition(lastRow) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

    await user.click(within(header).getByRole('button', { name: 'Show' }))
    expect(document.querySelector('[data-row-id="cat-dormant:1"]')).toBeInTheDocument()
    expect(within(expenseSection).queryByText('1 hidden')).not.toBeInTheDocument()
  })

  it('separates the bands with a gap, not a rule', async () => {
    usePlanHandlers()
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', { name: /plan/i }))
    await screen.findByTestId('plan-sheet')

    expect(screen.getByTestId('plan-section-income').classList.contains('plan-band-income')).toBe(true)
    const expenseSection = screen.getByTestId('plan-section-expense')
    expect(expenseSection.classList.contains('plan-band-expense')).toBe(true)
    // whitespace separates the two sections; the old border-t-2 rule is gone
    expect(expenseSection.classList.contains('mt-6')).toBe(true)
    expect(expenseSection.classList.contains('border-t-2')).toBe(false)
  })

  it('hides an uncategorized row whose visible cells are all zero, and it returns when the window covers its spend', async () => {
    const plan: BudgetPlanDto = JSON.parse(JSON.stringify(fixtureWirePlan))
    const incomeUncat = plan.structure.elements.find((el) => el.id === 'uncategorized' && el.type === 3)!
    incomeUncat.cells = [
      { actual: '8', planned: '' },
      { actual: '0', planned: '' },
      { actual: '0', planned: '' },
      { actual: '0', planned: '' },
    ]
    server.use(
      ...coreHandlers({ user: userWithBudget }),
      http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
      planHandler(plan),
    )
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-07-01' })
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', { name: /plan/i }))
    await screen.findByTestId('plan-sheet')
    const grid = screen.getByTestId('plan-sheet')

    // window Jul/Aug/Sep: income-uncat's only nonzero actual is month 0 (May), out of
    // view -> hidden
    expect(document.querySelector('[data-row-id="uncategorized:3"]')).not.toBeInTheDocument()

    // keyboard flat rows must skip it too: ArrowDown from the last income loose row
    // (Freelance) lands on the first expense row, never on the hidden uncategorized row
    await user.click(screen.getByTestId('plan-cell-cat-freelance:0'))
    grid.focus()
    await user.keyboard('{ArrowDown}')
    expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')

    // navigate back two months so 2026-05 enters the window -> row reappears
    await user.click(screen.getByRole('button', { name: 'Earlier months' }))
    await user.click(screen.getByRole('button', { name: 'Earlier months' }))
    await waitFor(() => expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-05-01'))
    expect(document.querySelector('[data-row-id="uncategorized:3"]')).toBeInTheDocument()
  })
})

it('scrolls the income/expenses/net trio and pins only the balance row', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  const totals = screen.getByTestId('plan-totals')
  const balance = screen.getByTestId('plan-balance-row')

  // the trio and the balance are separate elements; neither contains the other
  expect(totals).not.toContainElement(balance)
  expect(balance).not.toContainElement(totals)

  // only the balance row is pinned
  expect(balance.className).toContain('sticky')
  expect(totals.className).not.toContain('sticky')

  // the three totals rows, plus the uncategorized expense row slotted between
  // Expenses and Net
  const totalRows = within(totals).getAllByRole('row')
  expect(totalRows).toHaveLength(4)
  expect(within(totals).getByText('Income')).toBeInTheDocument()
  expect(within(totals).getByText('Expenses')).toBeInTheDocument()
  expect(within(totals).getByText('Net')).toBeInTheDocument()
  expect(within(balance).getByText('Balance')).toBeInTheDocument()

  // order: Income, Expenses, Uncategorized, Net — all four are totals lines
  expect(totalRows[1]).toHaveTextContent('Expenses')
  expect(totalRows[2]).toHaveTextContent('Uncategorized')
  expect(totalRows[3]).toHaveTextContent('Net')
  // it is a totals line, not a selectable element row
  expect(totalRows[2].querySelector('[data-row-id]')).toBeNull()
})

it('rules element rows flush with hairline dividers', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  // bands no longer space their children apart
  const income = screen.getByTestId('plan-section-income')
  expect(income.className).not.toContain('gap-1')

  // rows carry a hairline divider and are no longer rounded cards
  const row = screen.getByTestId('plan-cell-pe1:0').closest('[role="row"]') as HTMLElement
  expect(row.className).not.toContain('border-b')
  expect(row.className).not.toContain('rounded-md')

  // the divider lives on the [data-row-id] wrapper (a direct child of the band), not
  // the inner [role="row"] grid — that's what lets the last-child CSS rule in
  // index.css suppress the trailing hairline without touching FolderRows' own border.
  // jsdom does not apply index.css, so the suppression itself is not checkable here.
  const wrapper = screen.getByTestId('plan-cell-pe1:0').closest('[data-row-id]') as HTMLElement
  expect(wrapper.className).toContain('border-b')
  expect(wrapper).toContainElement(row)
})

it('leaves the current month column unmarked in the grid body, bold in the header only', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  // hover marks the single cell, never the whole column: no cross-cell attribute
  const sheet = screen.getByTestId('plan-sheet')
  expect(sheet.parentElement).not.toHaveAttribute('data-hover-col')
  expect(document.querySelector('[data-hover-col]')).toBeNull()

  // nothing in the body singles the current month out — no tint, no rules
  expect(document.querySelectorAll('.plan-current-month')).toHaveLength(0)
  sheet.querySelectorAll('[data-col]').forEach((c) => expect(c.className).not.toContain('bg-accent/40'))

  // the bold month name in the header is the only current-month cue
  const header = [...document.querySelectorAll('[role="columnheader"]')].find((h) => h.className.includes('font-bold'))
  expect(header).toBeDefined()
})

it('gives expanded child rows the same row-hover treatment as their parents', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  // pe1/Living has children; expanding it reveals cat-rent as a ChildRow
  await user.click(within(document.querySelector('[data-row-id="pe1:0"]') as HTMLElement).getByTitle('Living'))
  const childCell = await screen.findByTestId('plan-cell-cat-rent:0')
  const childRow = childCell.closest('[role="row"]') as HTMLElement

  // budget mode tints child rows on hover just like parents (BudgetTable.tsx);
  // plan mode must not diverge
  expect(childRow.className).toContain('plan-row')
})

it('shows plan row actions only in edit mode, with side-filtered move-to-folder', async () => {
  // pe1/Living (expense-sided) starts in 'bf1'/Essentials; ie1/Salaries (income) puts
  // 'bf-bonus' on the income side, so the filter is exercised both ways
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const planWithFolders: BudgetPlanDto = {
    ...plan,
    structure: {
      ...plan.structure,
      folders: [...plan.structure.folders, { id: 'bf-bonus', name: 'Bonuses Folder', position: 1 }],
      elements: plan.structure.elements.map((el) => (el.id === 'ie1' ? { ...el, folderId: 'bf-bonus' } : el)),
    },
  }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(planWithFolders),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  // read-only by default: no row menus
  expect(screen.queryByRole('button', { name: /element actions/i })).not.toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  await user.click(await screen.findByRole('button', { name: 'element actions Living' }))
  expect(await screen.findByRole('menuitem', { name: 'Change currency' })).toBeInTheDocument()
  await user.click(screen.getByRole('menuitem', { name: 'Move to folder…' }))

  // pe1/Living is expense-sided: expense + neutral folders only, never income ones
  const dialog = await screen.findByRole('dialog', { name: 'Move to folder…' })
  expect(within(dialog).getByRole('button', { name: 'Essentials' })).toBeInTheDocument()
  expect(within(dialog).getByRole('button', { name: 'No folder' })).toBeInTheDocument()
  expect(within(dialog).queryByRole('button', { name: 'Bonuses Folder' })).not.toBeInTheDocument()
})

it('picking a folder in the move dialog fires move-element with the right payload and closes the dialog', async () => {
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const planWithFolders: BudgetPlanDto = {
    ...plan,
    structure: {
      ...plan.structure,
      folders: [...plan.structure.folders, { id: 'bf-bonus', name: 'Bonuses Folder', position: 1 }],
      elements: plan.structure.elements.map((el) => (el.id === 'ie1' ? { ...el, folderId: 'bf-bonus' } : el)),
    },
  }
  let body: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(planWithFolders),
    http.post('*/api/v1/budget/move-element', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  await user.click(await screen.findByRole('button', { name: 'element actions Living' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Move to folder…' }))

  const dialog = await screen.findByRole('dialog', { name: 'Move to folder…' })
  await user.click(within(dialog).getByRole('button', { name: 'Essentials' }))

  await waitFor(() => expect(body).toEqual({ budgetId: 'b1', id: 'pe1', folderId: 'bf1', afterId: null }))
  expect(screen.queryByRole('dialog', { name: 'Move to folder…' })).not.toBeInTheDocument()
})

it('creates a plan folder with members, switching sides clears the selection', async () => {
  let folderBody: unknown
  const moves: unknown[] = []
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    http.post('*/api/v1/budget/create-folder', async ({ request }) => {
      folderBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { item: { id: 'nf1', name: 'Employment', position: 9 } } })
    }),
    http.post('*/api/v1/budget/move-element', async ({ request }) => {
      moves.push(await request.json())
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  await user.click(await screen.findByRole('button', { name: 'Create folder' }))
  const dialog = await screen.findByRole('dialog', { name: 'New folder' })

  // submit is blocked with no members
  await user.type(within(dialog).getByLabelText('Folder name'), 'Employment')
  expect(within(dialog).getByRole('button', { name: 'Create' })).toBeDisabled()

  // default side is expense; switch to income and pick an income element
  // (ie1's top-level name in the fixture is "Salaries" — its child category is "Salary")
  await user.click(within(dialog).getByRole('tab', { name: 'Income' }))
  await user.click(within(dialog).getByRole('checkbox', { name: 'Salaries' }))
  expect(within(dialog).getByRole('button', { name: 'Create' })).toBeEnabled()

  // flipping back to expense clears the income selection
  await user.click(within(dialog).getByRole('tab', { name: 'Expenses' }))
  expect(within(dialog).getByRole('button', { name: 'Create' })).toBeDisabled()

  await user.click(within(dialog).getByRole('tab', { name: 'Income' }))
  await user.click(within(dialog).getByRole('checkbox', { name: 'Salaries' }))
  await user.click(within(dialog).getByRole('button', { name: 'Create' }))

  await waitFor(() => expect(folderBody).toMatchObject({ name: 'Employment' }))
  await waitFor(() => expect(moves).toHaveLength(1))
  // the folder id is client-generated (uuidv7) and sent as-is on create-folder; the
  // move must target that same id, not whatever id the (irrelevant, mocked) response echoes back
  const clientFolderId = (folderBody as { id: string }).id
  expect(moves[0]).toMatchObject({ id: 'ie1', folderId: clientFolderId })
})

it('shows drag handles only in edit mode', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  expect(screen.queryByRole('button', { name: /^move / })).not.toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  expect(screen.getAllByRole('button', { name: /^move / }).length).toBeGreaterThan(0)
})

it('scopes every drag handle to its own band, so no drag can cross the income/expense divider', async () => {
  // An element's side comes from its TYPE and a folder's from its members, and
  // order-folders persists position only — so a cross-band drop would either be
  // rejected by the server (CodeBudgetFolderSideMixed) or silently snap back on
  // reload. Per-band SortableContexts are what make the drop impossible at all.
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const planWithFolders: BudgetPlanDto = {
    ...plan,
    structure: {
      ...plan.structure,
      folders: [...plan.structure.folders, { id: 'bf-bonus', name: 'Bonuses Folder', position: 1 }],
      elements: plan.structure.elements.map((el) => (el.id === 'ie1' ? { ...el, folderId: 'bf-bonus' } : el)),
    },
  }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(planWithFolders),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  const income = screen.getByTestId('plan-section-income')
  const expense = screen.getByTestId('plan-section-expense')

  // income handles: the Salaries row and its Bonuses folder — and nothing expense-sided
  expect(within(income).getByRole('button', { name: 'move Salaries' })).toBeInTheDocument()
  expect(within(income).getByRole('button', { name: 'move folder Bonuses Folder' })).toBeInTheDocument()
  expect(within(income).queryByRole('button', { name: 'move Living' })).not.toBeInTheDocument()

  // expense handles live in the other band entirely
  expect(within(expense).getByRole('button', { name: 'move Living' })).toBeInTheDocument()
  expect(within(expense).getByRole('button', { name: 'move folder Essentials' })).toBeInTheDocument()
  expect(within(expense).queryByRole('button', { name: 'move Salaries' })).not.toBeInTheDocument()
})

it('keeps the drag grip on its own root row, not stretched by expanded children', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  const grip = screen.getByRole('button', { name: 'move Living' })
  const wrapper = grip.closest('[data-plan-sortable]') as HTMLElement

  // grip and row share grid row 1, so the grip's h-full resolves against the ROOT
  // row's height. jsdom has no layout, so assert the mechanism rather than pixels:
  // a stretched grip would be the old flex + hand-tuned margin arrangement.
  expect(grip.className).toContain('row-start-1')
  expect(grip.className).toContain('h-full')
  expect(grip.className).not.toMatch(/\bmt-\d/)

  // expanding pe1/Living adds child rows INSIDE the same wrapper; the grip must not
  // be pulled to the middle of the whole expanded block
  expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument()
  await user.click(within(wrapper).getByText('Living'))
  expect(await screen.findByTestId('plan-cell-cat-rent:0')).toBeInTheDocument()
  const rootRow = wrapper.querySelector('[role="row"]') as HTMLElement
  expect(rootRow).toContainElement(screen.getByTestId('plan-cell-pe1:0'))
  expect(rootRow).not.toContainElement(screen.getByTestId('plan-cell-cat-rent:0'))
  expect(grip.parentElement).toBe(wrapper.firstElementChild)
})

it('keeps the uncategorized totals line undraggable', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  // uncategorized is a synthetic bucket with no position of its own
  expect(screen.queryByRole('button', { name: 'move Uncategorized' })).not.toBeInTheDocument()
})

it('edit mode keeps roving keyboard navigation and the fill handle working', async () => {
  // the sortable wrapper adds DOM depth around each row: selection, arrow-key
  // navigation and the Excel-style fill handle must all survive it
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  await user.click(screen.getByTestId('plan-cell-pe1:0'))
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')

  await user.keyboard('{ArrowRight}')
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('aria-selected', 'true')

  // the row is still reachable by its data-row-id anchor, unchanged by the wrapper:
  // the grip lives on the sortable OUTSIDE it, which is what keeps every existing
  // [data-row-id] query working
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  expect(pe1Row).toBeInTheDocument()
  expect(within(pe1Row).queryByRole('button', { name: 'move Living' })).not.toBeInTheDocument()
  const sortable = pe1Row.closest('[data-plan-sortable="pe1"]') as HTMLElement
  expect(within(sortable).getByRole('button', { name: 'move Living' })).toBeInTheDocument()

  // the fill handle still renders on the selected editable cell
  expect(within(screen.getByTestId('plan-cell-pe1:1')).getByTestId('fill-handle')).toBeInTheDocument()
})

it('a fill drag past the sortable activation distance still commits in edit mode', async () => {
  // dnd-kit's PointerSensor activates at 4px of movement, and the fill drag moves
  // horizontally well past that. The two stay separate because the sensor's
  // activator is bound to the grip alone — the fill handle's pointerdown never
  // reaches it — so the row must not tear loose from the grid mid-fill.
  let fillBody: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      fillBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  await user.click(screen.getByTestId('plan-cell-pe1:0'))
  const handle = within(screen.getByTestId('plan-cell-pe1:0')).getByTestId('fill-handle')

  // jsdom reports a 0-wide cell, so drive fillTargetCol with an explicit column width
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({ width: 100, height: 20, top: 0, left: 0, right: 100, bottom: 20, x: 0, y: 0, toJSON: () => ({}) } as DOMRect)
  fireEvent.pointerDown(handle, { pointerId: 1, clientX: 0, clientY: 0, button: 0, isPrimary: true })
  fireEvent.pointerMove(handle, { pointerId: 1, clientX: 100, clientY: 0 })
  fireEvent.pointerUp(handle, { pointerId: 1 })
  vi.restoreAllMocks()

  // and the fill itself still commits, so the guard did not break the gesture
  await waitFor(() => expect(fillBody).toMatchObject({ elementId: 'pe1' }))
})

it('the arrangement a drag anchors to matches the filtered (hideEmpty) rows actually on screen, never a hidden one', async () => {
  // Expense loose order: cat-food (hidden by hideEmpty — zeroed out below) -> tag1
  // (visible, the drop target) -> env-eur (visible, the dragged row, sits after tag1).
  // Dragging env-eur onto tag1 is the backward-drag case that exposes the bug: an
  // arrangement built from the UNFILTERED rows still has cat-food at index 0, and
  // moveElementInArrangement's insert-at-target-index math (elementMove.ts) lands the
  // moved row right after whatever preceded the target in that unfiltered list — here,
  // cat-food, a row hideEmpty has hidden from the user entirely. With the fix, cat-food
  // is excluded from the arrangement, so env-eur can only ever land first (afterId: null).
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const planWithHiddenLeadRow: BudgetPlanDto = {
    ...plan,
    structure: {
      ...plan.structure,
      elements: plan.structure.elements.map((el) => {
        if (el.id === 'cat-food') {
          return { ...el, cells: el.cells.map(() => ({ actual: '0', planned: '' })) }
        }
        return el
      }),
    },
  }
  let body: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(planWithHiddenLeadRow),
    http.post('*/api/v1/budget/move-element', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  useBudgetPeriodStore.setState({ planHideEmpty: true })
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  await screen.findByRole('button', { name: 'move vacation' })

  // cat-food is hidden (hideEmpty is on and it has no activity/plan anywhere in this
  // variant) — it must not be reachable by row queries, confirming the drag below truly
  // has no way to land on it through the DOM, yet the bug reaches it anyway internally.
  expect(screen.queryByRole('button', { name: 'move Food' })).not.toBeInTheDocument()

  // PlanSheet renders the income band's DndContext before the expense band's on every
  // commit, so regardless of how many renders happened while the page settled, the LAST
  // captured handler is always the current expense band's onDragEnd.
  const expenseDragEnd = capturedDragEnds[capturedDragEnds.length - 1]
  expenseDragEnd({ active: { id: 'env-eur' }, over: { id: 'tag1' } })

  await waitFor(() => expect(body).toBeDefined())
  expect(body).toMatchObject({ id: 'env-eur' })
  const afterId = (body as { afterId: string | null }).afterId
  expect(afterId).not.toBe('cat-food')
  expect(afterId).toBeNull()
})

it('a row can be dragged out of a folder onto the band loose container even when the loose list is empty', async () => {
  // pe1/Living is the sole member of the "Essentials" folder in the base fixture, and
  // the expense band's loose rows are non-empty there — so make them empty by moving
  // every loose expense element into the folder too, isolating the empty-loose-list case
  // Finding 2 covers: BudgetPage gives every bucket (including "no folder") a container
  // droppable, so a row can always be dragged out even onto empty space; PlanSheet lacked
  // that droppable entirely, making the gesture silently inert whenever loose was empty.
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const planWithEmptyLoose: BudgetPlanDto = {
    ...plan,
    structure: {
      ...plan.structure,
      elements: plan.structure.elements.map((el) =>
        el.id === 'cat-food' || el.id === 'tag1' || el.id === 'env-eur' || el.id === 'cat-dormant'
          ? { ...el, folderId: 'bf1' }
          : el,
      ),
    },
  }
  let body: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(planWithEmptyLoose),
    http.post('*/api/v1/budget/move-element', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  await screen.findByRole('button', { name: 'move Food' })

  // the expense band's loose list is empty (every loose element was moved into the
  // folder above) — a working escape hatch needs a drop TARGET to exist even with
  // nothing rendered in it. Without Finding 2's fix there is no such element at all.
  const expenseSection = screen.getByTestId('plan-section-expense')
  expect(within(expenseSection).getByTestId('plan-loose-drop')).toBeInTheDocument()

  expect(capturedDragEnds.length).toBeGreaterThanOrEqual(2)
  // see the sibling test above: the last captured handler is always the current
  // expense band's onDragEnd, since income always renders first within a commit.
  const expenseDragEnd = capturedDragEnds[capturedDragEnds.length - 1]
  // dropping directly on the loose-area container droppable (empty space, no row to
  // land on) — this id only exists once LooseRowsContainer's useDroppable is wired up
  expenseDragEnd({ active: { id: 'cat-food' }, over: { id: 'bfolder:null' } })

  await waitFor(() => expect(body).toBeDefined())
  expect(body).toMatchObject({ id: 'cat-food', folderId: null })
})

it('offers Edit and Delete on an envelope row, but not on a category or a tag', async () => {
  // the budget view's wire response strips income envelopes entirely, so the plan
  // sheet is the ONLY place ie1/Salaries can be managed at all
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  // ie1/Salaries is an income envelope (type 4)
  await user.click(await screen.findByRole('button', { name: 'element actions Salaries' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Delete' })).toBeInTheDocument()
  await user.keyboard('{Escape}')

  // pe1/Living is an expense envelope (type 0) — same four items
  await user.click(await screen.findByRole('button', { name: 'element actions Living' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Delete' })).toBeInTheDocument()
  await user.keyboard('{Escape}')

  // cat-food/Food is a category (type 1): currency + move only
  await user.click(await screen.findByRole('button', { name: 'element actions Food' }))
  expect(await screen.findByRole('menuitem', { name: 'Change currency' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Edit' })).not.toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Delete' })).not.toBeInTheDocument()
  await user.keyboard('{Escape}')

  // tag1/vacation is a tag (type 2): currency + move only
  await user.click(await screen.findByRole('button', { name: 'element actions vacation' }))
  expect(await screen.findByRole('menuitem', { name: 'Change currency' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Edit' })).not.toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Delete' })).not.toBeInTheDocument()
})

it('editing an income envelope opens the dialog on the income side and saves', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    http.post('*/api/v1/budget/update-envelope', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  await user.click(await screen.findByRole('button', { name: 'element actions Salaries' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))

  const dialog = await screen.findByRole('dialog', { name: 'Edit envelope' })
  expect(within(dialog).getByLabelText('Name')).toHaveValue('Salaries')

  // side='income' is derived from the element type: only income categories are
  // offered, never the expense ones the same fixture also carries
  expect(within(dialog).getByText('Salary')).toBeInTheDocument()
  expect(within(dialog).queryByText('Food')).not.toBeInTheDocument()

  await user.clear(within(dialog).getByLabelText('Name'))
  await user.type(within(dialog).getByLabelText('Name'), 'Wages')
  await user.click(within(dialog).getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(body).toMatchObject({ budgetId: 'b1', id: 'ie1', name: 'Wages' }))
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Edit envelope' })).not.toBeInTheDocument())
})

it('deleting an income envelope confirms first, then fires delete-envelope', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    http.post('*/api/v1/budget/delete-envelope', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  await user.click(await screen.findByRole('button', { name: 'element actions Salaries' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))

  const confirm = await screen.findByRole('dialog', { name: 'Delete envelope?' })
  await user.click(within(confirm).getByRole('button', { name: 'Delete' }))

  await waitFor(() => expect(body).toMatchObject({ budgetId: 'b1', id: 'ie1' }))
})

it('reopening the create-folder dialog after a successful create starts blank', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    http.post('*/api/v1/budget/create-folder', () =>
      HttpResponse.json({ success: true, message: '', data: { item: { id: 'nf1', name: 'Employment', position: 9 } } }),
    ),
    http.post('*/api/v1/budget/move-element', () => HttpResponse.json({ success: true, message: '', data: {} })),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  await user.click(await screen.findByRole('button', { name: 'Create folder' }))
  const dialog = await screen.findByRole('dialog', { name: 'New folder' })
  await user.type(within(dialog).getByLabelText('Folder name'), 'Employment')
  await user.click(within(dialog).getByRole('tab', { name: 'Income' }))
  await user.click(within(dialog).getByRole('checkbox', { name: 'Salaries' }))
  await user.click(within(dialog).getByRole('button', { name: 'Create' }))

  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'New folder' })).not.toBeInTheDocument())

  // the success path closes from the parent, so nothing local can reset the fields —
  // a surviving name + members would let a second submit duplicate the folder
  await user.click(await screen.findByRole('button', { name: 'Create folder' }))
  const reopened = await screen.findByRole('dialog', { name: 'New folder' })
  expect(within(reopened).getByLabelText('Folder name')).toHaveValue('')
  expect(within(reopened).getByRole('button', { name: 'Create' })).toBeDisabled()
})

it('rejects a too-short folder name inline instead of letting the server refuse it', async () => {
  let called = false
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    http.post('*/api/v1/budget/create-folder', () => {
      called = true
      return HttpResponse.json({ success: true, message: '', data: { item: { id: 'nf1', name: 'Ab', position: 9 } } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  await user.click(await screen.findByRole('button', { name: 'Create folder' }))
  const dialog = await screen.findByRole('dialog', { name: 'New folder' })
  await user.type(within(dialog).getByLabelText('Folder name'), 'Ab')
  await user.click(within(dialog).getByRole('tab', { name: 'Income' }))
  await user.click(within(dialog).getByRole('checkbox', { name: 'Salaries' }))
  await user.click(within(dialog).getByRole('button', { name: 'Create' }))

  expect(await within(dialog).findByText('Folder name must be 3-64 characters')).toBeInTheDocument()
  expect(called).toBe(false)
})
