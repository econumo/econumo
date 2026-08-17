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
import { toast } from 'sonner'
import { balanceRow, formatPlanMonth, makePlanExchange, planTotals } from './planMath'
import { moneyFormat } from '@/lib/money'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})
vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

// jsdom cannot drive real dnd-kit pointer drags (no layout), so onDragEnd is
// captured here and fired directly with a synthetic {active, over} pair — the
// same shape dnd-kit itself would report. PlanSheet mounts one DndContext per
// band, income before expense, on every render — so this array only grows
// (never resets), but its LAST entry is always the current expense band's
// handler and the one before it the current income band's, regardless of how
// many renders happened first.
let capturedDragEnds: ((event: { active: { id: string }; over: { id: string } | null }) => void)[] = []
interface CapturedDragContext {
  onDragStart: (event: { active: { id: string } }) => void
  onDragEnd: (event: { active: { id: string }; over: { id: string } | null }) => void
}
let capturedDragContexts: CapturedDragContext[] = []
vi.mock('@dnd-kit/core', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@dnd-kit/core')>()
  return {
    ...actual,
    DndContext: ({
      onDragStart,
      onDragEnd,
      children,
    }: {
      onDragStart: (event: never) => void
      onDragEnd: (event: never) => void
      children: ReactNode
    }) => {
      capturedDragEnds.push(onDragEnd as never)
      capturedDragContexts.push({ onDragStart, onDragEnd } as never)
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

function renderPage(initialPath: '/plan' | '/budget' = '/plan') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/plan', element: <BudgetPage key="plan" mode="plan" /> },
      { path: '/budget', element: <BudgetPage key="budget" mode="budget" /> },
    ],
    { initialEntries: [initialPath] },
  )
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
  capturedDragContexts = []
  useBudgetPeriodStore.setState({
    selectedDate: '2026-07-01',
    unfoldedElements: {},
    foldBudgetId: null,
    planFirstMonth: null,
    planFolds: {},
    planHideEmpty: false,
  })
})

it('/plan renders the sheet: months, income on top, cells', async () => {
  usePlanHandlers()
  renderPage()
  await screen.findByText(/jul/i)
  const income = screen.getByTestId('plan-section-income')
  const firstExpense = screen.getByTestId('plan-section-expense')
  expect(income.compareDocumentPosition(firstExpense) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  // no member-less folder in the fixture -> no neutral band between them
  expect(screen.queryByTestId('plan-section-neutral')).not.toBeInTheDocument()
  const cell = screen.getAllByTestId('plan-cell-pe1:0')[0]
  expect(within(cell).getByTestId('cell-actual')).toBeInTheDocument()
  expect(within(cell).getByTestId('cell-planned')).toBeInTheDocument()
})

it('overspend turns the actual red in a past month and with no plan set; never on income', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-05-01' })
  renderPage()
  await screen.findByText(/may/i)
  // July: 125 spent, no plan stored — a past month by the time this runs (fixture months are 2026)
  const foodJuly = screen.getAllByTestId('plan-cell-cat-food:2')[0]
  expect(within(foodJuly).getByTestId('cell-actual')).toHaveClass('text-destructive')
  // May: 120 spent against a 150 plan in a past month — under, so green
  const foodMay = screen.getAllByTestId('plan-cell-cat-food:0')[0]
  expect(within(foodMay).getByTestId('cell-actual')).not.toHaveClass('text-destructive')
  expect(within(foodMay).getByTestId('cell-actual')).toHaveClass('text-income')
  // income over an unset plan is neither
  const freelanceMay = screen.getAllByTestId('plan-cell-cat-freelance:0')[0]
  expect(within(freelanceMay).getByTestId('cell-actual')).not.toHaveClass('text-destructive')
  expect(within(freelanceMay).getByTestId('cell-actual')).not.toHaveClass('text-income')
  // June: 2000 vs 2000 for Salaries (income) — plain; env-eur June 40 vs 100 — green
  const eurJune = screen.getAllByTestId('plan-cell-env-eur:1')[0]
  expect(within(eurJune).getByTestId('cell-actual')).toHaveClass('text-income')
})

it('arrows shift the window by one month, clamped at the budget start', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-01-01' })
  const user = userEvent.setup()
  renderPage()
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

it('the plan window position survives a remount', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  const { unmount } = renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Later months' }))
  const monthAfterNav = useBudgetPeriodStore.getState().planFirstMonth
  expect(monthAfterNav).not.toBeNull()
  unmount()

  renderPage()
  expect(useBudgetPeriodStore.getState().planFirstMonth).toBe(monthAfterNav)
  await screen.findByTestId('plan-sheet')
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
  renderPage()
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

  // indexes 2 and 3 are the Uncategorized and Transfers lines slotted between Expenses and Net
  const rows = within(totalsBlock).getAllByRole('row')
  const [incomeRow, expensesRow, , , netRow] = rows
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
  await screen.findByTestId('plan-sheet')

  expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Essentials' }))
  expect(document.querySelector('[data-row-id="pe1:0"]')).not.toBeInTheDocument()
  expect(useBudgetPeriodStore.getState().planFolds.bf1).toBe(true)

  unmount()
  renderPage()
  await screen.findByTestId('plan-sheet')
  expect(document.querySelector('[data-row-id="pe1:0"]')).not.toBeInTheDocument()
})

it('clicking anywhere on a folder header row toggles the fold, but its own controls keep their action', async () => {
  // put the dormant row inside the Essentials folder so its header carries a "Show" notice
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const planWithDormantInFolder: BudgetPlanDto = {
    ...plan,
    structure: {
      ...plan.structure,
      elements: plan.structure.elements.map((el) => (el.id === 'cat-dormant' ? { ...el, folderId: 'bf1' } : el)),
    },
  }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(planWithDormantInFolder),
  )
  useBudgetPeriodStore.setState({ planHideEmpty: true })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')

  const folder = screen.getByTestId('plan-folder-bf1')
  const nameButton = within(folder).getByRole('button', { name: 'Essentials' })
  const header = nameButton.parentElement!.parentElement as HTMLElement
  expect(within(header).getByText('1 hidden')).toBeInTheDocument()
  expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()

  // the blank part of the header row folds…
  await user.click(header)
  expect(document.querySelector('[data-row-id="pe1:0"]')).not.toBeInTheDocument()
  expect(nameButton).toHaveAttribute('aria-expanded', 'false')
  // …and unfolds
  await user.click(header)
  expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()

  // "Show" reveals the hidden row without touching the fold
  await user.click(within(header).getByRole('button', { name: 'Show' }))
  expect(document.querySelector('[data-row-id="cat-dormant:1"]')).toBeInTheDocument()
  expect(nameButton).toHaveAttribute('aria-expanded', 'true')

  // the edit-mode grip is a drag handle, not a fold toggle
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  await user.click(await screen.findByRole('button', { name: 'move folder Essentials' }))
  expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()
  expect(within(screen.getByTestId('plan-folder-bf1')).getByRole('button', { name: 'Essentials' })).toHaveAttribute('aria-expanded', 'true')
})

it('edit mode: a folder header menu renames the folder, and deletes it only when it has no members', async () => {
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const planWithEmptyFolder: BudgetPlanDto = {
    ...plan,
    structure: { ...plan.structure, folders: [...plan.structure.folders, { id: 'bf-empty', name: 'Fun', position: 5 }] },
  }
  let renameBody: unknown
  let deleteBody: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(planWithEmptyFolder),
    http.post('*/api/v1/budget/update-folder', async ({ request }) => {
      renameBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/budget/delete-folder', async ({ request }) => {
      deleteBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')

  // read-only: no folder menus
  expect(screen.queryByRole('button', { name: /budget folder actions/ })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  // Essentials has a member: rename offered, delete not
  await user.click(await screen.findByRole('button', { name: 'budget folder actions Essentials' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Delete folder' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('menuitem', { name: 'Edit' }))
  const rename = await screen.findByRole('dialog', { name: 'Rename folder' })
  const input = within(rename).getByDisplayValue('Essentials')
  await user.clear(input)
  await user.type(input, 'Basics')
  await user.click(within(rename).getByRole('button', { name: 'Update' }))
  await waitFor(() => expect(renameBody).toEqual({ budgetId: 'b1', id: 'bf1', name: 'Basics' }))
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Rename folder' })).not.toBeInTheDocument())
  // opening the menu / picking an item must not have folded the folder
  expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()

  // the empty folder offers delete, behind a confirmation
  await user.click(screen.getByRole('button', { name: 'budget folder actions Fun' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete folder' }))
  expect(deleteBody).toBeUndefined()
  const confirm = await screen.findByRole('dialog', { name: 'Delete folder?' })
  expect(within(confirm).getByText('Are you sure you want to delete the folder “Fun”?')).toBeInTheDocument()
  await user.click(within(confirm).getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(deleteBody).toEqual({ budgetId: 'b1', id: 'bf-empty' }))
})

it('hide-empty removes dormant rows, shows the per-section count, Show reveals them', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
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
  await screen.findByTestId('plan-sheet')

  // guest role: pe1 would normally be editable for the owner, but not here
  const pe1Cell = screen.getByTestId('plan-cell-pe1:1')
  expect(within(pe1Cell).queryByRole('button', { name: /limit/i })).not.toBeInTheDocument()

  // children never carry their own limit, regardless of role
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  await user.click(within(pe1Row).getByRole('button', { name: 'Expand' }))
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

  // ArrowRight on the name cell of a row with children first unfolds it (pe1/Living
  // has a child); the next ArrowRight returns to the first month column of the
  // now-shifted window
  grid.focus()
  await user.keyboard('{ArrowRight}')
  expect(await screen.findByTestId('plan-cell-cat-rent:0')).toBeInTheDocument()
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

  // children never carry their own limit (and are not selectable at all)
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  await user.click(within(pe1Row).getByRole('button', { name: 'Expand' }))
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

it('ArrowLeft reaches the name cell (col -1) by keyboard, Space there toggles expansion, and ArrowLeft again shifts the window', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const grid = screen.getByTestId('plan-sheet')

  await user.click(screen.getByTestId('plan-cell-pe1:0'))
  grid.focus()
  await user.keyboard('{ArrowLeft}')
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  const nameCell = within(pe1Row).getByTitle('Living').closest('[role="gridcell"]') as HTMLElement
  expect(nameCell).toHaveAttribute('aria-selected', 'true')
  expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-06-01')

  // Space on the name cell toggles the row's expansion (pe1/Living has children)
  expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument()
  await user.keyboard(' ')
  expect(await screen.findByTestId('plan-cell-cat-rent:0')).toBeInTheDocument()
  await user.keyboard(' ')
  await waitFor(() => expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument())

  // ArrowLeft again, still at -1, shifts the window back a month and keeps the selection at -1
  await user.keyboard('{ArrowLeft}')
  await waitFor(() => expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-05-01'))
  expect(within(pe1Row).getByTitle('Living').closest('[role="gridcell"]')).toHaveAttribute('aria-selected', 'true')
})

// The name is a selection target, not a fold toggle: only the chevron unfolds the
// children, so a click meant to highlight the row never springs the breakdown open.
it('clicking an envelope name selects it without expanding; only the chevron toggles the children', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  const nameCell = within(pe1Row).getByTitle('Living').closest('[role="gridcell"]') as HTMLElement

  await user.click(within(pe1Row).getByTitle('Living'))
  expect(nameCell).toHaveAttribute('aria-selected', 'true')
  expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument()

  const chevron = within(pe1Row).getByRole('button', { name: 'Expand' })
  expect(chevron).toHaveAttribute('aria-expanded', 'false')
  await user.click(chevron)
  expect(await screen.findByTestId('plan-cell-cat-rent:0')).toBeInTheDocument()
  expect(within(pe1Row).getByRole('button', { name: 'Collapse' })).toHaveAttribute('aria-expanded', 'true')
  await user.click(within(pe1Row).getByRole('button', { name: 'Collapse' }))
  await waitFor(() => expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument())
})

it('ArrowRight on a collapsed envelope name cell expands it and ArrowLeft collapses it; otherwise the keys navigate as before', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const grid = screen.getByTestId('plan-sheet')
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  const nameCell = within(pe1Row).getByTitle('Living').closest('[role="gridcell"]') as HTMLElement

  await user.click(screen.getByTestId('plan-cell-pe1:0'))
  grid.focus()
  await user.keyboard('{ArrowLeft}')
  expect(nameCell).toHaveAttribute('aria-selected', 'true')

  // collapsed + ArrowRight: expand, selection stays on the name cell
  await user.keyboard('{ArrowRight}')
  expect(await screen.findByTestId('plan-cell-cat-rent:0')).toBeInTheDocument()
  expect(nameCell).toHaveAttribute('aria-selected', 'true')
  // expanded + ArrowRight: the usual move to the first month column
  await user.keyboard('{ArrowRight}')
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')
  expect(screen.getByTestId('plan-cell-cat-rent:0')).toBeInTheDocument()

  await user.keyboard('{ArrowLeft}')
  expect(nameCell).toHaveAttribute('aria-selected', 'true')
  // expanded + ArrowLeft: collapse, the window does not page
  await user.keyboard('{ArrowLeft}')
  await waitFor(() => expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument())
  expect(nameCell).toHaveAttribute('aria-selected', 'true')
  expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-06-01')
  // collapsed + ArrowLeft: pages the window back as before
  await user.keyboard('{ArrowLeft}')
  await waitFor(() => expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-05-01'))

  // a row without children never folds on ArrowRight/ArrowLeft — plain navigation
  await user.click(screen.getByTestId('plan-cell-cat-food:0'))
  grid.focus()
  await user.keyboard('{ArrowLeft}{ArrowRight}')
  expect(screen.getByTestId('plan-cell-cat-food:0')).toHaveAttribute('aria-selected', 'true')
})

it('folder headers are selectable: ArrowLeft/ArrowRight fold/unfold them, Up/Down step through them keeping the column', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const grid = screen.getByTestId('plan-sheet')
  const folder = screen.getByTestId('plan-folder-bf1')
  const nameButton = within(folder).getByRole('button', { name: 'Essentials' })
  const headerCell = nameButton.closest('[role="gridcell"]') as HTMLElement
  const header = nameButton.parentElement!.parentElement as HTMLElement

  // clicking the header row still folds (as before) AND selects the folder
  await user.click(header)
  expect(document.querySelector('[data-row-id="pe1:0"]')).not.toBeInTheDocument()
  expect(headerCell).toHaveAttribute('aria-selected', 'true')

  // ArrowRight unfolds a folded folder; a second ArrowRight is a no-op
  grid.focus()
  await user.keyboard('{ArrowRight}')
  expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()
  expect(headerCell).toHaveAttribute('aria-selected', 'true')
  await user.keyboard('{ArrowRight}')
  expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()
  expect(headerCell).toHaveAttribute('aria-selected', 'true')
  expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-06-01')

  // ArrowDown lands on the folder's first member row (name column, where the click put it)
  await user.keyboard('{ArrowDown}')
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  expect(within(pe1Row).getByTitle('Living').closest('[role="gridcell"]')).toHaveAttribute('aria-selected', 'true')
  await user.keyboard('{ArrowUp}')
  expect(headerCell).toHaveAttribute('aria-selected', 'true')

  // ArrowLeft folds an unfolded folder; a second ArrowLeft is a no-op (no window paging)
  await user.keyboard('{ArrowLeft}')
  expect(document.querySelector('[data-row-id="pe1:0"]')).not.toBeInTheDocument()
  await user.keyboard('{ArrowLeft}')
  expect(document.querySelector('[data-row-id="pe1:0"]')).not.toBeInTheDocument()
  expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-06-01')
  // Space toggles it back open
  await user.keyboard(' ')
  expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()

  // the column survives a trip through the header: Up from pe1's Jul cell selects
  // the folder, Down comes back to the same Jul cell
  await user.click(screen.getByTestId('plan-cell-pe1:1'))
  grid.focus()
  await user.keyboard('{ArrowUp}')
  expect(headerCell).toHaveAttribute('aria-selected', 'true')
  expect(screen.getByTestId('plan-cell-pe1:1')).not.toHaveAttribute('aria-selected', 'true')
  await user.keyboard('{ArrowDown}')
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('aria-selected', 'true')
})

// Enter on the highlighted name cell opens the element's own edit dialog — the same
// one the settings pages / row menu use — gated by the right the backend checks:
// budget role for envelopes, ownership for categories and tags.
async function selectNameCell(user: ReturnType<typeof userEvent.setup>, rowId: string) {
  const grid = screen.getByTestId('plan-sheet')
  const row = document.querySelector(`[data-row-id="${rowId}"]`) as HTMLElement
  const [nameCell, firstMonthCell] = within(row).getAllByRole('gridcell')
  // click a month cell first: ArrowLeft from the name cell would page the window instead
  await user.click(firstMonthCell)
  grid.focus()
  await user.keyboard('{ArrowLeft}')
  expect(nameCell).toHaveAttribute('aria-selected', 'true')
}

it('Enter on a highlighted envelope, category, or tag opens its edit dialog when the user may edit it', async () => {
  let envelopeBody: unknown
  let categoryBody: unknown
  let tagBody: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    http.post('*/api/v1/budget/update-envelope', async ({ request }) => {
      envelopeBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/category/update-category', async ({ request }) => {
      categoryBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
    http.post('*/api/v1/tag/update-tag', async ({ request }) => {
      tagBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')

  // envelope (owner role) -> the envelope dialog, prefilled, and it saves through update-envelope
  await selectNameCell(user, 'pe1:0')
  await user.keyboard('{Enter}')
  const envelopeDialog = await screen.findByRole('dialog', { name: 'Edit envelope' })
  expect(within(envelopeDialog).getByDisplayValue('Living')).toBeInTheDocument()
  await user.click(within(envelopeDialog).getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(envelopeBody).toMatchObject({ budgetId: 'b1', id: 'pe1', name: 'Living' }))
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Edit envelope' })).not.toBeInTheDocument())

  // Enter must not also toggle the envelope's expansion
  expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument()

  // a modal opened from the keyboard has no trigger to hand focus back to, so the
  // sheet must reclaim it itself — otherwise the arrow keys are dead after closing
  await waitFor(() => expect(screen.getByTestId('plan-sheet')).toHaveFocus())
  await user.keyboard('{ArrowDown}')
  const foodRow = document.querySelector('[data-row-id="cat-food:1"]') as HTMLElement
  expect(within(foodRow).getAllByRole('gridcell')[0]).toHaveAttribute('aria-selected', 'true')

  // own category -> the category dialog, prefilled, saving through update-category
  await selectNameCell(user, 'cat-food:1')
  await user.keyboard('{Enter}')
  const categoryDialog = await screen.findByRole('dialog', { name: 'Edit category' })
  const nameInput = within(categoryDialog).getByDisplayValue('Food')
  await user.clear(nameInput)
  await user.type(nameInput, 'Groceries')
  await user.click(within(categoryDialog).getByRole('button', { name: /update/i }))
  await waitFor(() => expect(categoryBody).toMatchObject({ id: 'cat-food', name: 'Groceries', icon: 'restaurant' }))
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Edit category' })).not.toBeInTheDocument())

  // own tag -> the tag dialog, saving through update-tag
  await selectNameCell(user, 'tag1:2')
  await user.keyboard('{Enter}')
  const tagDialog = await screen.findByRole('dialog', { name: 'Edit tag' })
  expect(within(tagDialog).getByDisplayValue('vacation')).toBeInTheDocument()
  await user.click(within(tagDialog).getByRole('button', { name: /update/i }))
  await waitFor(() => expect(tagBody).toMatchObject({ id: 'tag1', name: 'vacation' }))
})

// Only rows that can carry a limit — envelopes, root categories, tags — are selectable.
// An expanded envelope's children are read-only breakdown lines: a click on one does
// not move the highlight, and the arrow keys step straight over them.
it('child rows are not selectable by click or keyboard', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01', foldBudgetId: 'b1', unfoldedElements: { pe1: true } })
  const user = userEvent.setup()
  renderPage()
  const childCell = await screen.findByTestId('plan-cell-cat-rent:0')
  const grid = screen.getByTestId('plan-sheet')
  const childRow = childCell.closest('[role="row"]') as HTMLElement
  const parentCell = screen.getByTestId('plan-cell-pe1:0')

  await user.click(parentCell)
  expect(parentCell).toHaveAttribute('aria-selected', 'true')

  // clicking a child cell leaves the parent highlighted; child cells never expose aria-selected
  await user.click(childCell)
  expect(parentCell).toHaveAttribute('aria-selected', 'true')
  for (const cell of within(childRow).getAllByRole('gridcell')) {
    expect(cell).not.toHaveAttribute('aria-selected')
  }

  // ArrowDown from the expanded parent skips its children and lands on the next root row
  grid.focus()
  await user.keyboard('{ArrowDown}')
  expect(screen.getByTestId('plan-cell-cat-food:0')).toHaveAttribute('aria-selected', 'true')
  await user.keyboard('{ArrowUp}')
  expect(parentCell).toHaveAttribute('aria-selected', 'true')
})

// The archive is history, not a workspace: an archived row shows only when it has a
// value — a nonzero actual or a set plan — in a VISIBLE month, and the whole section
// goes when none does. Values in the fetched-but-offscreen buffer months don't count,
// so paging the window can hide or reveal a row. Same rule as the budget view's
// Archive section, and independent of the density toggle.
it('archived rows show only with a value in a visible month; the section disappears otherwise', async () => {
  const archived = (id: string, name: string, cells: { actual: string; planned: string }[]) => ({
    id, type: 1, name, icon: 'delete', currencyId: 'cur-usd', isArchived: 1, folderId: null, position: 9, ownerUserId: 'u1', cells, children: [],
  })
  const plan = {
    ...fixtureWirePlan,
    structure: {
      ...fixtureWirePlan.structure,
      elements: [
        ...fixtureWirePlan.structure.elements,
        // fixture months are May..Aug; the initial window below is Jun..Aug, so May is a buffer month
        archived('arch-may', 'Only May', [{ actual: '18.53', planned: '' }, { actual: '0', planned: '' }, { actual: '0', planned: '' }, { actual: '0', planned: '' }]),
        archived('arch-aug', 'Only Aug', [{ actual: '0', planned: '' }, { actual: '0', planned: '' }, { actual: '0', planned: '' }, { actual: '0', planned: '40' }]),
        archived('arch-none', 'Nothing', [{ actual: '0', planned: '' }, { actual: '0.00', planned: '' }, { actual: '0', planned: '' }, { actual: '0', planned: '' }]),
      ],
    },
  }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(plan),
  )
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')

  // Jun..Aug: only the row with an August plan has a visible value
  const section = await screen.findByTestId('plan-section-archived')
  expect(within(section).getByTitle('Only Aug')).toBeInTheDocument()
  expect(within(section).queryByTitle('Only May')).not.toBeInTheDocument()
  expect(within(section).queryByTitle('Nothing')).not.toBeInTheDocument()

  // May..Jul: the May spend comes on screen, the August plan leaves it
  await user.click(screen.getByRole('button', { name: 'Earlier months' }))
  await waitFor(() => expect(within(screen.getByTestId('plan-section-archived')).getByTitle('Only May')).toBeInTheDocument())
  expect(within(screen.getByTestId('plan-section-archived')).queryByTitle('Only Aug')).not.toBeInTheDocument()

  // a window with no archived values at all drops the section entirely: Sep..Nov
  // (only May and Aug carry values, and neither is visible then)
  for (let i = 0; i < 4; i++) {
    await user.click(screen.getByRole('button', { name: 'Later months' }))
  }
  await waitFor(() => expect(screen.queryByTestId('plan-section-archived')).not.toBeInTheDocument())
})

it('Enter without the right to edit explains why in a toast instead of opening a dialog: guest role for envelopes, foreign owner for categories/tags; uncategorized stays silent', async () => {
  const guestAccess = [{ user: fixtureOwner, role: 'guest', isAccepted: 1 }]
  const guestBudget = { ...fixtureWireBudget, meta: { ...fixtureWireBudget.meta, access: guestAccess } }
  const guestPlan = {
    ...fixtureWirePlan,
    meta: { ...fixtureWirePlan.meta, access: guestAccess },
    structure: {
      ...fixtureWirePlan.structure,
      elements: fixtureWirePlan.structure.elements.map((el) =>
        el.id === 'cat-food' || el.id === 'tag1' ? { ...el, ownerUserId: 'u2' } : el,
      ),
    },
  }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: guestBudget } })),
    planHandler(guestPlan),
  )
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')

  const expected: [string, string | null][] = [
    ['pe1:0', "You can't edit this envelope — your role in this budget is read-only."],
    ['cat-food:1', "You can't edit this category — it belongs to another user."],
    ['tag1:2', "You can't edit this tag — it belongs to another user."],
    ['uncategorized:3', null],
  ]
  for (const [rowId, message] of expected) {
    vi.mocked(toast.error).mockClear()
    await selectNameCell(user, rowId)
    await user.keyboard('{Enter}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    if (message) {
      // a fixed id so repeated Enter presses replace the toast instead of stacking
      expect(toast.error).toHaveBeenCalledWith(message, { id: 'plan-edit-no-access' })
    } else {
      expect(toast.error).not.toHaveBeenCalled()
    }
  }
  // and Enter did not fall back to toggling the envelope's expansion either
  expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument()
})

it('budget-mode envelope dialog still offers expense categories only', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
  )
  const user = userEvent.setup()
  renderPage('/budget')
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
  await screen.findByTestId('plan-error')

  await user.click(screen.getByRole('button', { name: 'Try again' }))
  await screen.findByTestId('plan-sheet')
})

it('ArrowLeft at the name cell does not page the window past the budget start', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-01-01' })
  const user = userEvent.setup()
  renderPage()
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

it('a folder with no elements renders header-only in its own band between income and expenses', async () => {
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
  await screen.findByTestId('plan-sheet')

  const income = screen.getByTestId('plan-section-income')
  const neutral = screen.getByTestId('plan-section-neutral')
  const expense = screen.getByTestId('plan-section-expense')
  expect(within(neutral).getByTestId('plan-folder-bf-empty')).toBeInTheDocument()
  expect(within(neutral).getByText('Empty Folder')).toBeInTheDocument()
  expect(within(expense).queryByText('Empty Folder')).not.toBeInTheDocument()
  expect(within(income).queryByText('Empty Folder')).not.toBeInTheDocument()
  expect(income.compareDocumentPosition(neutral) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(neutral.compareDocumentPosition(expense) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

  // the expanded empty folder carries the same hint the budget view shows; folding hides it
  const note = 'This folder is empty. Move a category, tag, or envelope here, or create a new envelope.'
  const folder = screen.getByTestId('plan-folder-bf-empty')
  expect(within(folder).getByText(note)).toBeInTheDocument()
  await user.click(within(folder).getByRole('button', { name: 'Empty Folder' }))
  expect(within(folder).queryByText(note)).not.toBeInTheDocument()
  // a folder WITH members shows no hint
  expect(within(screen.getByTestId('plan-folder-bf1')).queryByText(note)).not.toBeInTheDocument()
})

it('in edit mode, empty folders reorder among themselves inside the neutral band', async () => {
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const planWithEmptyFolders: BudgetPlanDto = {
    ...plan,
    structure: {
      ...plan.structure,
      folders: [
        ...plan.structure.folders,
        { id: 'bf-e1', name: 'Empty One', position: 5 },
        { id: 'bf-e2', name: 'Empty Two', position: 6 },
      ],
    },
  }
  let body: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(planWithEmptyFolders),
    http.post('*/api/v1/budget/move-folder', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  // the empty folders' grips live in the neutral band, in neither side's band
  const neutral = screen.getByTestId('plan-section-neutral')
  expect(within(neutral).getByRole('button', { name: 'move folder Empty One' })).toBeInTheDocument()
  expect(within(neutral).getByRole('button', { name: 'move folder Empty Two' })).toBeInTheDocument()
  expect(within(screen.getByTestId('plan-section-expense')).queryByRole('button', { name: /move folder Empty/ })).not.toBeInTheDocument()

  // bands mount their DndContexts in DOM order: income, neutral, expense
  const neutralCtx = capturedDragContexts[capturedDragContexts.length - 2]
  neutralCtx.onDragStart({ active: { id: 'pfolder:bf-e1' } })
  neutralCtx.onDragEnd({ active: { id: 'pfolder:bf-e1' }, over: { id: 'pfolder:bf-e2' } })
  await waitFor(() => expect(body).toEqual({ budgetId: 'b1', id: 'bf-e1', afterId: 'bf-e2' }))
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

  it('renders on a hovered editable cell while another cell is selected, and a drag from it selects the source cell', async () => {
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
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('plan-sheet')

    // nothing selected yet: hovering an editable cell already offers the handle
    const pe1Jul = screen.getByTestId('plan-cell-pe1:1')
    await user.hover(pe1Jul)
    expect(within(pe1Jul).getByTestId('fill-handle')).toBeInTheDocument()
    await user.unhover(pe1Jul)
    expect(screen.queryByTestId('fill-handle')).not.toBeInTheDocument()

    // select tag1's Jun cell, then hover pe1's Jul cell: the hovered cell offers the
    // handle without stealing the selection
    const tag1Jun = screen.getByTestId('plan-cell-tag1:0')
    await user.click(tag1Jun)
    expect(within(tag1Jun).getByTestId('fill-handle')).toBeInTheDocument()
    await user.hover(pe1Jul)
    expect(tag1Jun).toHaveAttribute('aria-selected', 'true')
    expect(within(pe1Jul).getByTestId('fill-handle')).toBeInTheDocument()

    // a non-editable cell offers nothing on hover
    const uncat = screen.getAllByTestId('plan-cell-uncategorized:0')[0]
    await user.hover(uncat)
    expect(within(uncat).queryByTestId('fill-handle')).not.toBeInTheDocument()

    // drag from the hovered cell's handle: the pointer leaves the source cell mid-drag,
    // yet the handle (which holds the pointer capture) survives until release
    await user.hover(pe1Jul)
    const handle = within(pe1Jul).getByTestId('fill-handle')
    fireEvent.pointerDown(handle, { clientX: 100, pointerId: 1 })
    fireEvent.pointerMove(handle, { clientX: 210, pointerId: 1 })
    fireEvent.mouseLeave(pe1Jul)
    expect(screen.getByTestId('plan-cell-pe1:2').className).toContain('fill-covered')
    expect(within(pe1Jul).getByTestId('fill-handle')).toBe(handle)
    fireEvent.pointerUp(handle, { clientX: 210, pointerId: 1 })

    await waitFor(() => expect(bodies).toHaveLength(1))
    // pe1's Jul plan is 250 (fetched months May..Aug, index 2)
    expect(bodies[0]).toEqual({ budgetId: 'b1', elementId: 'pe1', period: '2026-08-01', amount: '250' })
    // once copied, the copied (source) cell is the selection
    expect(pe1Jul).toHaveAttribute('aria-selected', 'true')
    expect(tag1Jun).not.toHaveAttribute('aria-selected', 'true')
    expect(screen.getByTestId('plan-sheet')).toHaveFocus()
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
    await screen.findByTestId('plan-sheet')

    // window is Jun/Jul/Aug; pe1's col0 (Jun) has planned '200' — the exact cell
    // that shows the handle in wide mode
    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    expect(screen.queryByTestId('fill-handle')).not.toBeInTheDocument()
  })
})

describe('clipboard and keyboard fill', () => {
  function useCapturingHandlers(bodies: unknown[]) {
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
  }

  function clipboard(text = ''): { setData: ReturnType<typeof vi.fn>; getData: () => string } {
    return { setData: vi.fn(), getData: () => text }
  }

  it('copy writes the selected cell: planned amount, 0 for an unset cell, the name on the name cell', async () => {
    usePlanHandlers()
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('plan-sheet')
    const grid = screen.getByTestId('plan-sheet')

    // nothing selected: the event is left alone
    const idle = clipboard()
    const idleEvent = fireEvent.copy(grid, { clipboardData: idle })
    expect(idle.setData).not.toHaveBeenCalled()
    expect(idleEvent).toBe(true)

    // Jun/Jul/Aug window; pe1 Jun planned '200'
    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    const jun = clipboard()
    expect(fireEvent.copy(grid, { clipboardData: jun })).toBe(false)
    expect(jun.setData).toHaveBeenCalledWith('text/plain', '200')

    // pe1 Aug planned '' reads as 0 everywhere else, so it copies as 0
    await user.click(screen.getByTestId('plan-cell-pe1:2'))
    const aug = clipboard()
    fireEvent.copy(grid, { clipboardData: aug })
    expect(aug.setData).toHaveBeenCalledWith('text/plain', '0')

    // name cell copies the element name; a non-editable row copies too
    fireEvent.keyDown(grid, { key: 'ArrowLeft' })
    fireEvent.keyDown(grid, { key: 'ArrowLeft' })
    fireEvent.keyDown(grid, { key: 'ArrowLeft' })
    const name = clipboard()
    fireEvent.copy(grid, { clipboardData: name })
    expect(name.setData).toHaveBeenCalledWith('text/plain', 'Living')

    await user.click(screen.getAllByTestId('plan-cell-uncategorized:0')[0])
    const uncat = clipboard()
    fireEvent.copy(grid, { clipboardData: uncat })
    expect(uncat.setData).toHaveBeenCalledTimes(1)
  })

  it('paste writes a single amount into the selected editable cell through set-limit', async () => {
    const bodies: unknown[] = []
    useCapturingHandlers(bodies)
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('plan-sheet')
    const grid = screen.getByTestId('plan-sheet')

    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    expect(fireEvent.paste(grid, { clipboardData: clipboard(' 150 ') })).toBe(false)
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0]).toEqual({ budgetId: 'b1', elementId: 'pe1', period: '2026-06-01', amount: '150' })
    expect(within(screen.getByTestId('plan-cell-pe1:0')).getByTestId('cell-planned')).toHaveTextContent('150')
    expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_PASTE_CELL)

    // an empty clipboard clears the limit, same as an emptied editor
    fireEvent.paste(grid, { clipboardData: clipboard('') })
    await waitFor(() => expect(bodies).toHaveLength(2))
    expect(bodies[1]).toEqual({ budgetId: 'b1', elementId: 'pe1', period: '2026-06-01', amount: null })
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('paste of non-numeric text, or onto a non-editable / name cell, writes nothing', async () => {
    const bodies: unknown[] = []
    useCapturingHandlers(bodies)
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('plan-sheet')
    const grid = screen.getByTestId('plan-sheet')

    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    fireEvent.paste(grid, { clipboardData: clipboard('hello') })
    expect(toast.error).toHaveBeenCalledTimes(1)
    expect(vi.mocked(toast.error).mock.calls[0][1]).toMatchObject({ id: 'plan-paste-blocked' })

    // uncategorized is never editable
    await user.click(screen.getAllByTestId('plan-cell-uncategorized:0')[0])
    fireEvent.paste(grid, { clipboardData: clipboard('150') })
    expect(toast.error).toHaveBeenCalledTimes(2)
    expect(vi.mocked(toast.error).mock.calls[1][1]).toMatchObject({ id: 'plan-paste-blocked' })

    // name cell: silently ignored (nothing sensible to write)
    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    fireEvent.keyDown(grid, { key: 'ArrowLeft' })
    fireEvent.paste(grid, { clipboardData: clipboard('150') })
    expect(toast.error).toHaveBeenCalledTimes(2)

    // no selection at all
    fireEvent.paste(grid, { clipboardData: clipboard('150') })
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(bodies).toHaveLength(0)
  })

  it('paste inside the open LimitEditor input is left to the input, not the grid', async () => {
    const bodies: unknown[] = []
    useCapturingHandlers(bodies)
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('plan-sheet')

    const cell = screen.getByTestId('plan-cell-pe1:0')
    await user.click(cell)
    await user.click(within(cell).getByRole('button', { name: 'limit Living' }))
    const input = document.querySelector<HTMLInputElement>('[data-slot="popover-content"] input')
    expect(input).not.toBeNull()
    expect(fireEvent.paste(input as HTMLInputElement, { clipboardData: clipboard('150') })).toBe(true)
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(bodies).toHaveLength(0)
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('Shift+ArrowRight extends a fill range from the selected cell; releasing Shift copies the value into it', async () => {
    const bodies: unknown[] = []
    useCapturingHandlers(bodies)
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('plan-sheet')
    const grid = screen.getByTestId('plan-sheet')

    // Jun/Jul/Aug; pe1 Jun planned '200'
    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    const jun = screen.getByTestId('plan-cell-pe1:0')
    const jul = screen.getByTestId('plan-cell-pe1:1')
    const aug = screen.getByTestId('plan-cell-pe1:2')

    fireEvent.keyDown(grid, { key: 'ArrowRight', shiftKey: true })
    expect(jul.className).toContain('fill-covered')
    expect(aug.className).not.toContain('fill-covered')
    // the selection stays on the source cell
    expect(jun).toHaveAttribute('aria-selected', 'true')

    fireEvent.keyDown(grid, { key: 'ArrowRight', shiftKey: true })
    expect(aug.className).toContain('fill-covered')
    // at the last visible column: clamps, never pages the window
    fireEvent.keyDown(grid, { key: 'ArrowRight', shiftKey: true })
    expect(useBudgetPeriodStore.getState().planFirstMonth).toBe('2026-06-01')
    expect(jun).toHaveAttribute('aria-selected', 'true')

    // a plain arrow mid-fill is swallowed like the pointer drag
    fireEvent.keyDown(grid, { key: 'ArrowDown' })
    expect(jun).toHaveAttribute('aria-selected', 'true')
    expect(bodies).toHaveLength(0)

    fireEvent.keyUp(grid, { key: 'Shift' })
    await waitFor(() => expect(bodies).toHaveLength(2))
    expect(bodies).toEqual(
      expect.arrayContaining([
        { budgetId: 'b1', elementId: 'pe1', period: '2026-07-01', amount: '200' },
        { budgetId: 'b1', elementId: 'pe1', period: '2026-08-01', amount: '200' },
      ]),
    )
    expect(jul.className).not.toContain('fill-covered')
    expect(within(jul).getByTestId('cell-planned')).toHaveTextContent('200')
    expect(within(aug).getByTestId('cell-planned')).toHaveTextContent('200')
  })

  it('Shift+ArrowLeft shrinks the range; a range shrunk to nothing writes nothing on release', async () => {
    const bodies: unknown[] = []
    useCapturingHandlers(bodies)
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('plan-sheet')
    const grid = screen.getByTestId('plan-sheet')

    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    const jul = screen.getByTestId('plan-cell-pe1:1')
    const aug = screen.getByTestId('plan-cell-pe1:2')

    fireEvent.keyDown(grid, { key: 'ArrowRight', shiftKey: true })
    fireEvent.keyDown(grid, { key: 'ArrowRight', shiftKey: true })
    expect(aug.className).toContain('fill-covered')
    fireEvent.keyDown(grid, { key: 'ArrowLeft', shiftKey: true })
    expect(aug.className).not.toContain('fill-covered')
    expect(jul.className).toContain('fill-covered')
    fireEvent.keyDown(grid, { key: 'ArrowLeft', shiftKey: true })
    expect(jul.className).not.toContain('fill-covered')
    // never left of the source
    fireEvent.keyDown(grid, { key: 'ArrowLeft', shiftKey: true })
    expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')

    fireEvent.keyUp(grid, { key: 'Shift' })
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(bodies).toHaveLength(0)
    // and the grid is back to plain navigation
    fireEvent.keyDown(grid, { key: 'ArrowRight' })
    expect(jul).toHaveAttribute('aria-selected', 'true')
  })

  it('Escape or losing focus cancels a keyboard fill without any request', async () => {
    const bodies: unknown[] = []
    useCapturingHandlers(bodies)
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('plan-sheet')
    const grid = screen.getByTestId('plan-sheet')
    const jul = screen.getByTestId('plan-cell-pe1:1')

    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    fireEvent.keyDown(grid, { key: 'ArrowRight', shiftKey: true })
    expect(jul.className).toContain('fill-covered')
    fireEvent.keyDown(grid, { key: 'Escape' })
    expect(jul.className).not.toContain('fill-covered')
    fireEvent.keyUp(grid, { key: 'Shift' })

    fireEvent.keyDown(grid, { key: 'ArrowRight', shiftKey: true })
    expect(jul.className).toContain('fill-covered')
    fireEvent.blur(grid)
    expect(jul.className).not.toContain('fill-covered')
    fireEvent.keyUp(grid, { key: 'Shift' })

    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(bodies).toHaveLength(0)
  })

  it('Shift+ArrowRight on the name cell or a non-editable cell starts nothing and does not move the selection', async () => {
    const bodies: unknown[] = []
    useCapturingHandlers(bodies)
    useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('plan-sheet')
    const grid = screen.getByTestId('plan-sheet')

    const uncat = screen.getAllByTestId('plan-cell-uncategorized:0')[0]
    await user.click(uncat)
    fireEvent.keyDown(grid, { key: 'ArrowRight', shiftKey: true })
    expect(uncat).toHaveAttribute('aria-selected', 'true')
    expect(document.querySelector('.fill-covered')).toBeNull()

    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    fireEvent.keyDown(grid, { key: 'ArrowLeft' })
    fireEvent.keyDown(grid, { key: 'ArrowRight', shiftKey: true })
    expect(screen.getByTestId('plan-cell-pe1:0')).not.toHaveAttribute('aria-selected', 'true')
    expect(document.querySelector('.fill-covered')).toBeNull()

    fireEvent.keyUp(grid, { key: 'Shift' })
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(bodies).toHaveLength(0)
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
    renderPage()
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
    await screen.findByTestId('plan-sheet')
    const grid = screen.getByTestId('plan-sheet')

    // window Jul/Aug/Sep: income-uncat's only nonzero actual is month 0 (May), out of
    // view -> hidden
    expect(document.querySelector('[data-row-id="uncategorized:3"]')).not.toBeInTheDocument()

    // keyboard flat rows must skip it too: ArrowDown from the last income loose row
    // (Freelance) lands on the expense side (the Essentials folder header, then its
    // first row), never on the hidden uncategorized row
    await user.click(screen.getByTestId('plan-cell-cat-freelance:0'))
    grid.focus()
    await user.keyboard('{ArrowDown}')
    expect(screen.getByRole('button', { name: 'Essentials' }).closest('[role="gridcell"]')).toHaveAttribute('aria-selected', 'true')
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
  renderPage()
  await screen.findByTestId('plan-sheet')

  const totals = screen.getByTestId('plan-totals')
  const balance = screen.getByTestId('plan-balance-row')

  // the trio and the balance are separate elements; neither contains the other
  expect(totals).not.toContainElement(balance)
  expect(balance).not.toContainElement(totals)

  // only the balance row is pinned
  expect(balance.className).toContain('sticky')
  expect(totals.className).not.toContain('sticky')

  // the three totals rows, plus the Uncategorized and Transfers lines slotted
  // between Expenses and Net
  const totalRows = within(totals).getAllByRole('row')
  expect(totalRows).toHaveLength(5)
  expect(within(totals).getByText('Income')).toBeInTheDocument()
  expect(within(totals).getByText('Expenses')).toBeInTheDocument()
  expect(within(totals).getByText('Net')).toBeInTheDocument()
  expect(within(balance).getByText('Balance')).toBeInTheDocument()

  // order: Income, Expenses, Uncategorized, Transfers, Net — all five are totals lines
  expect(totalRows[1]).toHaveTextContent('Expenses')
  expect(totalRows[2]).toHaveTextContent('Uncategorized')
  expect(totalRows[3]).toHaveTextContent('Transfers')
  expect(totalRows[4]).toHaveTextContent('Net')
  // they are totals lines, not selectable element rows
  expect(totalRows[2].querySelector('[data-row-id]')).toBeNull()
  expect(totalRows[3].querySelector('[data-row-id]')).toBeNull()
})

it('transfers line: signed net per month, a tooltip with the in/out split, and a link only where money crossed', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')

  // window Jun/Jul/Aug; the fixture's June crossed 50 in / 150 out
  const junLink = screen.getByTestId('plan-totals-transfers-link-0')
  expect(junLink).toHaveTextContent('-100.00')
  expect(junLink.className).toContain('text-destructive')
  expect(junLink).toHaveAttribute('title', 'In 50.00 · Out 150.00. Show transactions')

  // nothing crossed in July: plain text, no link
  expect(screen.queryByTestId('plan-totals-transfers-link-1')).not.toBeInTheDocument()
  expect(screen.getByTestId('plan-totals-transfers-1')).toHaveTextContent('0.00')

  // Net and Balance carry the June transfers (math core, not hand-derived)
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const ex = makePlanExchange(plan, [fixtureUsd, fixtureEur])
  const totals = planTotals(plan, ex)
  expect(totals[1].transfersNet).toBe('-100')
  const netRow = within(screen.getByTestId('plan-totals')).getAllByRole('row')[4]
  expect(within(netRow).getByText(moneyFormat(totals[1].effectiveNet, fixtureUsd, { showCurrency: false, useNativePrecision: false }))).toBeInTheDocument()
})

it('clicking a totals link opens the transaction list for THAT column\'s month', async () => {
  usePlanHandlers()
  const seen: URL[] = []
  server.use(
    http.get('*/api/v1/budget/get-transaction-list', ({ request }) => {
      seen.push(new URL(request.url))
      return HttpResponse.json({ success: true, message: '', data: { items: [] } })
    }),
  )
  useBudgetPeriodStore.setState({ planFirstMonth: '2026-06-01', selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')

  // Transfers, June (column 0) — not the budget page's selected July period
  await user.click(screen.getByTestId('plan-totals-transfers-link-0'))
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText('Transfers')).toBeInTheDocument()
  await waitFor(() => expect(seen).toHaveLength(1))
  expect(seen[0].searchParams.get('transfers')).toBe('1')
  expect(seen[0].searchParams.get('periodStart')).toBe('2026-06-01')
  expect(seen[0].searchParams.get('uncategorized')).toBeNull()
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())

  // Uncategorized, whichever visible column has spend: the fixture's uncategorized
  // expense row lands in July (column 1)
  await user.click(screen.getByTestId('plan-totals-uncategorized-link-1'))
  await screen.findByRole('dialog')
  await waitFor(() => expect(seen).toHaveLength(2))
  expect(seen[1].searchParams.get('uncategorized')).toBe('1')
  expect(seen[1].searchParams.get('periodStart')).toBe('2026-07-01')
  expect(seen[1].searchParams.get('transfers')).toBeNull()
})

it('rules element rows flush with hairline dividers', async () => {
  usePlanHandlers()
  renderPage()
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
  renderPage()
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
  await screen.findByTestId('plan-sheet')

  // pe1/Living has children; expanding it reveals cat-rent as a ChildRow
  await user.click(within(document.querySelector('[data-row-id="pe1:0"]') as HTMLElement).getByRole('button', { name: 'Expand' }))
  const childCell = await screen.findByTestId('plan-cell-cat-rent:0')
  const childRow = childCell.closest('[role="row"]') as HTMLElement

  // budget mode tints child rows on hover just like parents (BudgetTable.tsx);
  // plan mode must not diverge
  expect(childRow.className).toContain('plan-row')

  // the child row's grid must share the parent row's horizontal padding — the fixed
  // name column absorbs the indent, so the month cells line up under the parent's
  // (extra padding on the row itself would shrink the 1fr month tracks and shift them)
  const parentRow = screen.getByTestId('plan-cell-pe1:0').closest('[role="row"]') as HTMLElement
  expect(parentRow.className).toContain('px-2')
  expect(childRow.className).toContain('px-2')
  expect(childRow.className).not.toMatch(/\bpl-\d/)
  expect(childRow.style.gridTemplateColumns).toBe(parentRow.style.gridTemplateColumns)
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
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  await user.click(await screen.findByRole('button', { name: 'Create folder' }))
  const dialog = await screen.findByRole('dialog', { name: 'New folder' })

  // submit is blocked with no members
  await user.type(within(dialog).getByLabelText('Folder name'), 'Employment')
  expect(within(dialog).getByRole('button', { name: 'Create' })).toBeDisabled()

  // income is the default side: grouping income is the reason this dialog exists
  expect(within(dialog).getByRole('tab', { name: 'Income' })).toHaveAttribute('aria-selected', 'true')
  expect(within(dialog).getByRole('tab', { name: 'Expenses' })).toHaveAttribute('aria-selected', 'false')

  // pick an income element (ie1's top-level name is "Salaries"; its child is "Salary")
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
  await user.click(within(wrapper).getByRole('button', { name: 'Expand' }))
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

it('holds the dropped order locally instead of snapping back until the refetch lands', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    // never resolves: the row must stay where it was dropped while the call is in flight,
    // which is exactly the window where the old code snapped it back
    http.post('*/api/v1/budget/move-element', async () => {
      await delay('infinite')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  await screen.findByRole('button', { name: 'move Food' })

  // the expense band's LOOSE rows (cat-food, tag1, env-eur) are the reorderable set
  const looseOrder = () =>
    [...document.querySelectorAll('[data-testid="plan-section-expense"] [data-row-id]')]
      .map((r) => r.getAttribute('data-row-id'))
      .filter((id) => id === 'cat-food:1' || id === 'tag1:2' || id === 'env-eur:0')
  expect(looseOrder()[0]).toBe('cat-food:1')

  // drop the first loose row onto the last one
  const expenseDragEnd = capturedDragEnds[capturedDragEnds.length - 1]
  expenseDragEnd({ active: { id: 'cat-food' }, over: { id: 'env-eur' } })

  // the reorder shows immediately, while the move-element call is still in flight
  await waitFor(() => expect(looseOrder()[0]).not.toBe('cat-food:1'))
  expect(looseOrder()).toContain('cat-food:1')
})

it('measures the grid with a callback ref so the loader cannot skip the measurement', async () => {
  // The sheet early-returns a loader while the plan fetches, so the grid node does not
  // exist on first commit. A mount effect ran against a null ref and never re-ran,
  // leaving the window stuck at the fallback column count until a later resize.
  // jsdom reports clientWidth 0, so the column count cannot be asserted here — instead
  // pin the mechanism: the observer must be attached when the NODE mounts.
  const observed: Element[] = []
  const RealRO = globalThis.ResizeObserver
  class SpyRO extends RealRO {
    observe(target: Element) {
      observed.push(target)
      super.observe(target)
    }
  }
  globalThis.ResizeObserver = SpyRO as unknown as typeof ResizeObserver
  try {
    usePlanHandlers()
    renderPage()
    const grid = await screen.findByTestId('plan-sheet')

    // the grid itself must be observed. With a mount effect it never is: the effect
    // runs on the loader commit, when containerRef.current is still null.
    expect(observed).toContain(grid)
  } finally {
    globalThis.ResizeObserver = RealRO
  }
})

it('closes every row with the currency, then the actions menu in edit mode', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')

  // the currency closes the row at rest — the name cell no longer carries it
  const row = screen.getByTestId('plan-cell-pe1:0').closest('[role="row"]')!
  expect(row.querySelector('[role="gridcell"]')!.textContent).not.toContain('$')
  expect(row.lastElementChild!.textContent).toContain('$')

  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  const menu = await screen.findByRole('button', { name: 'element actions Living' })

  // the menu joins the currency in that same trailing track, not the name cell
  const editRow = menu.closest('[role="row"]')!
  expect(editRow.lastElementChild).toContainElement(menu)
  expect(editRow.lastElementChild!.textContent).toContain('$')
  expect(menu.closest('[role="gridcell"]')).toBeNull()
})

it('collapses folder contents while a folder drag is in flight, and still drops correctly', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    http.post('*/api/v1/budget/order-folders', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  // pe1/Living sits inside the "Essentials" folder and is visible at rest
  expect(screen.getByTestId('plan-cell-pe1:0')).toBeInTheDocument()
  const folderHeader = screen.getByRole('button', { name: 'Essentials' })
  expect(folderHeader).toHaveAttribute('aria-expanded', 'true')

  const expenseCtx = capturedDragContexts[capturedDragContexts.length - 1]
  expenseCtx.onDragStart({ active: { id: 'pfolder:bf1' } })

  // rows hide so the headers reorder as compact blocks, but the folder's own fold
  // state is untouched — the chevron must not claim the user collapsed it
  await waitFor(() => expect(screen.queryByTestId('plan-cell-pe1:0')).not.toBeInTheDocument())
  expect(screen.getByRole('button', { name: 'Essentials' })).toHaveAttribute('aria-expanded', 'true')

  // the plan fixture ships a single folder, so there is nothing to reorder against —
  // this covers the collapse lifecycle, not the reorder itself (which
  // 'reorders folders within a band' already covers)
  expenseCtx.onDragEnd({ active: { id: 'pfolder:bf1' }, over: null })

  // contents come back once the drag ends
  await waitFor(() => expect(screen.getByTestId('plan-cell-pe1:0')).toBeInTheDocument())
  expect(body).toBeUndefined()
})

it('does not bounce after the move resolves but before the refetch returns', async () => {
  let planRequests = 0
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    // the FIRST plan fetch resolves normally; the refetch triggered by the move never
    // returns, holding open the exact window where the old code cleared the local order
    // and rendered the stale server list for a frame
    http.get('*/api/v1/budget/get-budget-plan', async () => {
      planRequests += 1
      if (planRequests > 1) {
        await delay('infinite')
      }
      return HttpResponse.json({ success: true, message: '', data: { item: fixtureWirePlan } })
    }),
    // resolves immediately, so onSuccess/onSettled both fire while the refetch is pending
    http.post('*/api/v1/budget/move-element', () => HttpResponse.json({ success: true, message: '', data: {} })),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  await screen.findByRole('button', { name: 'move Food' })

  const looseOrder = () =>
    [...document.querySelectorAll('[data-testid="plan-section-expense"] [data-row-id]')]
      .map((r) => r.getAttribute('data-row-id'))
      .filter((id) => id === 'cat-food:1' || id === 'tag1:2' || id === 'env-eur:0')
  expect(looseOrder()[0]).toBe('cat-food:1')

  const expenseDragEnd = capturedDragEnds[capturedDragEnds.length - 1]
  expenseDragEnd({ active: { id: 'cat-food' }, over: { id: 'env-eur' } })

  await waitFor(() => expect(looseOrder()[0]).not.toBe('cat-food:1'))

  // the mutation has settled and its refetch is in flight; the dropped order must hold
  await waitFor(() => expect(planRequests).toBeGreaterThan(1))
  const settled = looseOrder()
  expect(settled[0]).not.toBe('cat-food:1')
  expect(settled).toContain('cat-food:1')
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
