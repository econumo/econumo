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

  await user.click(screen.getByRole('switch', { name: 'Hide empty rows' }))
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
  await user.click(screen.getByRole('switch', { name: 'Hide empty rows' }))
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

it('income + opens the envelope dialog constrained to income categories and submits side=income', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    http.post('*/api/v1/budget/create-envelope', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({
        success: true,
        message: '',
        data: {
          item: {
            id: 'env-new', type: 4, name: 'Bonuses', icon: 'payments', currencyId: 'cur-usd', folderId: null,
            position: 9, budgeted: '0', available: '0', spent: '0', budgetSpent: '0', ownerUserId: null, isArchived: 0, children: [],
          },
        },
      })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  await user.click(screen.getByRole('button', { name: 'create income envelope' }))
  const dialog = await screen.findByRole('dialog', { name: 'New envelope' })
  expect(within(dialog).getByText('Salary')).toBeInTheDocument()
  expect(within(dialog).queryByText('Food')).not.toBeInTheDocument()

  await user.type(within(dialog).getByLabelText('Name'), 'Bonuses')
  await user.click(within(dialog).getByRole('button', { name: 'Create' }))

  await waitFor(() => expect(body).toMatchObject({ name: 'Bonuses', side: 'income' }))
})

it('move-to-folder menu lists only same-side and neutral folders and calls move-element', async () => {
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const planWithFolders: BudgetPlanDto = {
    ...plan,
    structure: {
      folders: [
        ...plan.structure.folders, // bf1 'Essentials' -> expense (holds pe1)
        { id: 'bf2', name: 'Bonuses Folder', position: 1 }, // income (holds ie1)
        { id: 'bf3', name: 'Misc Folder', position: 2 }, // neutral (no members)
      ],
      elements: plan.structure.elements.map((el) => (el.id === 'ie1' ? { ...el, folderId: 'bf2' } : el)),
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

  // cat-food is a loose EXPENSE category row
  await user.click(screen.getByRole('button', { name: 'plan row actions Food' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Move to folder…' }))

  const dialog = await screen.findByRole('dialog', { name: 'Move to folder…' })
  expect(within(dialog).getByRole('button', { name: 'Essentials' })).toBeInTheDocument() // expense-sided
  expect(within(dialog).getByRole('button', { name: 'Misc Folder' })).toBeInTheDocument() // neutral
  expect(within(dialog).getByRole('button', { name: 'No folder' })).toBeInTheDocument()
  expect(within(dialog).queryByRole('button', { name: 'Bonuses Folder' })).not.toBeInTheDocument() // income-sided, excluded

  await user.click(within(dialog).getByRole('button', { name: 'Misc Folder' }))

  await waitFor(() => expect(body).toEqual({ budgetId: 'b1', id: 'cat-food', folderId: 'bf3', afterId: null }))
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

it('grid keydown ignores arrow keys while a row-actions dropdown menu is open', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('tab', { name: /plan/i }))
  await screen.findByTestId('plan-sheet')

  await user.click(screen.getByTestId('plan-cell-pe1:0'))
  await user.click(screen.getByRole('button', { name: 'plan row actions Living' }))
  await screen.findByRole('menu')

  await user.keyboard('{ArrowDown}')
  // had the grid handled this, selection would have jumped to the next row (Food); the
  // dropdown-menu-content guard means the keystroke never reached grid navigation
  expect(screen.queryByTestId('plan-cell-cat-food:0')).toHaveAttribute('aria-selected', 'false')
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  expect(within(pe1Row).getByTitle('Living').closest('[role="gridcell"]')).toHaveAttribute('aria-selected', 'true')
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

  it('renders only on the selected editable non-empty cell', async () => {
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

    // pe1's col2 (Aug) has planned ''
    await user.click(screen.getByTestId('plan-cell-pe1:2'))
    expect(screen.queryByTestId('fill-handle')).not.toBeInTheDocument()
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
})

describe('income/expense split', () => {
  it('renders a foldable Expenses header that collapses the whole expense area', async () => {
    usePlanHandlers()
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', { name: /plan/i }))
    await screen.findByTestId('plan-sheet')

    const expenseSection = screen.getByTestId('plan-section-expense')
    const expensesButton = within(expenseSection).getByRole('button', { name: 'Expenses' })

    // expense rows present before folding: a folder row, a loose row, uncategorized
    expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()
    expect(document.querySelector('[data-row-id="cat-food:1"]')).toBeInTheDocument()
    expect(document.querySelector('[data-row-id="uncategorized:1"]')).toBeInTheDocument()
    // income rows present too
    expect(document.querySelector('[data-row-id="ie1:4"]')).toBeInTheDocument()

    await user.click(expensesButton)
    expect(document.querySelector('[data-row-id="pe1:0"]')).not.toBeInTheDocument()
    expect(document.querySelector('[data-row-id="cat-food:1"]')).not.toBeInTheDocument()
    expect(document.querySelector('[data-row-id="uncategorized:1"]')).not.toBeInTheDocument()
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

    await user.click(screen.getByRole('switch', { name: 'Hide empty rows' }))
    expect(document.querySelector('[data-row-id="cat-dormant:1"]')).not.toBeInTheDocument()

    const expenseSection = screen.getByTestId('plan-section-expense')
    const header = within(expenseSection).getByRole('button', { name: 'Expenses' }).parentElement as HTMLElement
    const hiddenNotice = within(header).getByText('1 hidden')

    // it sits in the header, not as a trailing line after the last visible row
    const lastRow = within(expenseSection).getByTestId('plan-cell-uncategorized:0')
    expect(hiddenNotice.compareDocumentPosition(lastRow) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

    await user.click(within(header).getByRole('button', { name: 'Show' }))
    expect(document.querySelector('[data-row-id="cat-dormant:1"]')).toBeInTheDocument()
    expect(within(expenseSection).queryByText('1 hidden')).not.toBeInTheDocument()
  })

  it('bands and divider are distinguishable', async () => {
    usePlanHandlers()
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('tab', { name: /plan/i }))
    await screen.findByTestId('plan-sheet')

    expect(screen.getByTestId('plan-section-income').classList.contains('plan-band-income')).toBe(true)
    const expenseSection = screen.getByTestId('plan-section-expense')
    expect(expenseSection.classList.contains('plan-band-expense')).toBe(true)
    expect(expenseSection.classList.contains('border-t-2')).toBe(true)
  })
})
