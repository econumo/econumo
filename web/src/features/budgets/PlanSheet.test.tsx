import type { ReactNode } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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
import { balanceRow, formatPlanMonth, makePlanExchange, planGroupSums, planTotals } from './planMath'
import { isZero } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})
vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

// jsdom cannot drive real dnd-kit pointer drags (no layout), so the handlers are
// captured here and fired directly with a synthetic {active, over} pair — the
// same shape dnd-kit itself would report. PlanSheet mounts one DndContext per
// section in DOM order (income, savings when present, neutral when present,
// expense) on every render — so these arrays only grow (never reset), but the
// LAST entry is always the current expense section's.
let capturedDragEnds: ((event: { active: { id: string }; over: { id: string } | null }) => void)[] = []
interface CapturedDragContext {
  onDragStart: (event: { active: { id: string } }) => void
  onDragOver: (event: { active: { id: string }; over: { id: string } | null }) => void
  onDragEnd: (event: { active: { id: string }; over: { id: string } | null }) => void
  onDragCancel: () => void
}
let capturedDragContexts: CapturedDragContext[] = []
// a test that needs dnd-kit's own pointer sensor mounts the real context instead
let realDndContext = false
vi.mock('@dnd-kit/core', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@dnd-kit/core')>()
  const { createElement } = await import('react')
  return {
    ...actual,
    DndContext: (props: {
      onDragStart: (event: never) => void
      onDragOver: (event: never) => void
      onDragEnd: (event: never) => void
      onDragCancel: () => void
      children: ReactNode
    }) => {
      const { onDragStart, onDragOver, onDragEnd, onDragCancel, children } = props
      capturedDragEnds.push(onDragEnd as never)
      capturedDragContexts.push({ onDragStart, onDragOver, onDragEnd, onDragCancel } as never)
      return realDndContext ? createElement(actual.DndContext, props as never) : children
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

// a tablet: compact but not a phone, which gets the single month view instead of the grid
function mockCompactViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023'), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
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

// The plan fixtures span May-Aug 2026 and several cases read "today" straight
// off the system clock: the default three-month window, the bold current-month
// header, past-vs-future cell styling. Left on the real clock they quietly rot
// the moment the calendar leaves that window. Only Date is faked, so
// userEvent's and waitFor's real timers still run.
beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 7, 15, 12, 0, 0))
  vi.clearAllMocks()
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
  capturedDragEnds = []
  capturedDragContexts = []
  realDndContext = false
  useBudgetPeriodStore.setState({
    selectedDate: '2026-07-01',
    unfoldedElements: {},
    foldBudgetId: null,
    planFolds: {},
    planNameWidth: null,
    planSumsShown: {},
  })
})

afterEach(() => {
  vi.useRealTimers()
})

it('/plan renders the sheet: months, income on top, cells', async () => {
  usePlanHandlers()
  renderPage()
  await screen.findByTestId('plan-month-header')
  const income = screen.getByTestId('plan-section-income')
  const firstExpense = screen.getByTestId('plan-section-expense')
  expect(income.compareDocumentPosition(firstExpense) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  // no member-less folder in the fixture -> no neutral band between them
  expect(screen.queryByTestId('plan-section-neutral')).not.toBeInTheDocument()
  const cell = screen.getAllByTestId('plan-cell-pe1:0')[0]
  expect(within(cell).getByTestId('cell-actual')).toBeInTheDocument()
  expect(within(cell).getByTestId('cell-planned')).toBeInTheDocument()
})

it('a row is one line: actual · plan up to the selected month, plan alone after it', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')
  // fixture pe1 (expense): Jun = history, Jul = selected, Aug = future
  const jun = screen.getByTestId('plan-cell-pe1:0')
  const aug = screen.getByTestId('plan-cell-pe1:2')
  expect(within(jun).getByTestId('cell-actual')).toBeInTheDocument()
  expect(within(jun).getByTestId('cell-planned')).toBeInTheDocument()
  expect(within(aug).queryByTestId('cell-actual')).not.toBeInTheDocument()
  // Aug is unplanned: blank, never 0.00
  expect(within(aug).getByTestId('cell-planned')).toHaveTextContent(/^$/)
  // no currency symbol column, no savings balance line
  expect(within(grid).queryByText('$')).not.toBeInTheDocument()
  expect(screen.queryByTestId('cell-closing')).not.toBeInTheDocument()
})

it('an unplanned month is blank, and only an over-plan actual is red', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  for (const actual of screen.getAllByTestId('cell-actual')) {
    expect(actual.className).not.toContain('text-income')
  }
  for (const planned of screen.getAllByTestId('cell-planned')) {
    expect(planned.textContent).not.toBe('0.00')
  }
})

it('clicking an actual opens that row and month in the transactions list', async () => {
  let params: URLSearchParams | undefined
  usePlanHandlers()
  server.use(
    http.get('*/api/v1/budget/get-transaction-list', ({ request }) => {
      params = new URL(request.url).searchParams
      return HttpResponse.json({ success: true, message: '', data: { items: [] } })
    }),
  )
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  // the history column: its month, not the selected one
  await user.click(within(screen.getByTestId('plan-cell-pe1:0')).getByTestId('cell-actual'))
  expect(await screen.findByRole('dialog')).toBeInTheDocument()
  await waitFor(() => expect(params?.get('envelopeId')).toBe('pe1'))
  expect(params?.get('periodStart')).toBe('2026-06-01')
})

it('clicking an actual selects its cell, so the keyboard picks up there once the list closes', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByTestId('plan-cell-cat-food:0'))
  await user.click(within(screen.getByTestId('plan-cell-pe1:1')).getByTestId('cell-actual'))
  await screen.findByRole('dialog')
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('aria-selected', 'true')
  expect(screen.getByTestId('plan-cell-cat-food:0')).toHaveAttribute('aria-selected', 'false')
  // the grid takes focus back, so typing edits the selected cell straight away
  await waitFor(() => expect(screen.getByTestId('plan-sheet')).toHaveFocus())
  await user.keyboard('7')
  expect(await screen.findByRole('textbox', { name: /living/i })).toHaveValue('7')
})

it('the grid takes focus back when the Transfers list closes', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByTestId('plan-cell-cat-food:0'))
  await user.click(screen.getByRole('button', { name: /^transactions .* 2026-06-01$/i }))
  await screen.findByRole('dialog')
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  await waitFor(() => expect(screen.getByTestId('plan-sheet')).toHaveFocus())
})

it('a desktop-width grid shows the history month\'s actual', async () => {
  const widthSpy = vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(1080)
  try {
    usePlanHandlers()
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
    renderPage()
    await screen.findByTestId('plan-sheet')
    // more than the jsdom fallback's three columns, so the real width is in effect
    await waitFor(() => expect(within(screen.getByTestId('plan-month-header')).getAllByRole('columnheader').length).toBeGreaterThan(3))
    const jun = screen.getByTestId('plan-cell-pe1:0')
    expect(jun).toHaveAttribute('data-month', '2026-06-01')
    expect(within(jun).getByTestId('cell-actual')).toBeInTheDocument()
  } finally {
    widthSpy.mockRestore()
  }
})

it('overspend turns the actual red, also with no plan set; never on income, and nothing turns green', async () => {
  usePlanHandlers()
  // May (history), Jun (selected), Jul
  useBudgetPeriodStore.setState({ selectedDate: '2026-06-01' })
  renderPage()
  await screen.findByTestId('plan-month-header')
  const actual = (testId: string) => within(screen.getByTestId(testId)).getByTestId('cell-actual')
  // May: vacation spent 20 with nothing planned
  expect(actual('plan-cell-tag1:0')).toHaveClass('text-expense')
  // pe1 Jun: 60 of 200; cat-food May: 120 of 150 — under, plain (no green any more)
  expect(actual('plan-cell-pe1:1')).not.toHaveClass('text-expense')
  expect(actual('plan-cell-cat-food:0')).not.toHaveClass('text-expense')
  expect(actual('plan-cell-cat-food:0')).not.toHaveClass('text-income')
  // income over an unset plan is neither
  expect(actual('plan-cell-cat-freelance:0')).not.toHaveClass('text-expense')
  // env-eur Jun: 40 of 100 — under, plain
  expect(actual('plan-cell-env-eur:1')).not.toHaveClass('text-income')
  // Jul is after the selected month: its 125 of nothing is not shown at all
  expect(within(screen.getByTestId('plan-cell-cat-food:2')).queryByTestId('cell-actual')).not.toBeInTheDocument()
})

it("the month row is the Plan view's only month selector, centred on the selected month", async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  // no second row of months above the grid: the Budget view's strip is not shown
  expect(screen.queryByRole('tablist', { name: 'period' })).not.toBeInTheDocument()
  const header = screen.getByTestId('plan-month-header')
  // the Budget/Plan words sit at the start of the month row
  expect(within(header).getByRole('tab', { name: /plan/i, selected: true })).toBeInTheDocument()
  const cols = within(header).getAllByRole('columnheader')
  // jsdom width 0 -> 3 visible: Jun (history), Jul (selected), Aug
  expect(cols.map((c) => c.getAttribute('data-month'))).toEqual(['2026-06-01', '2026-07-01', '2026-08-01'])
  expect(cols[1]).toHaveAttribute('data-selected-col', 'true')
})

it('clicking a month in the month row selects it; the arrows move the window one month', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const header = screen.getByTestId('plan-month-header')
  const months = () => within(screen.getByTestId('plan-month-header')).getAllByRole('columnheader').map((c) => c.getAttribute('data-month'))
  // Jun selected: May, Jun, Jul
  expect(months()).toEqual(['2026-05-01', '2026-06-01', '2026-07-01'])
  await user.click(within(header).getAllByRole('columnheader')[2].querySelector('button')!)
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-07-01')
  await waitFor(() => expect(months()).toEqual(['2026-06-01', '2026-07-01', '2026-08-01']))
  await user.click(within(screen.getByTestId('plan-month-header')).getByRole('button', { name: /later months/i }))
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-08-01')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_CHANGE_WINDOW)
  await user.click(within(screen.getByTestId('plan-month-header')).getByRole('button', { name: /earlier months/i }))
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-07-01')
})

it('the Budget view keeps its month strip', async () => {
  usePlanHandlers()
  renderPage('/budget')
  expect(await screen.findByRole('tablist', { name: 'period' })).toBeInTheDocument()
})

it('moves the selected month when the cursor walks past the last column', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')
  grid.focus()
  // select the first row's last month column, then step right off the edge
  fireEvent.keyDown(grid, { key: 'ArrowDown' })
  fireEvent.keyDown(grid, { key: 'ArrowRight' })
  fireEvent.keyDown(grid, { key: 'ArrowRight' })
  fireEvent.keyDown(grid, { key: 'ArrowRight' })
  await waitFor(() => expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-08-01'))
  expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_CHANGE_WINDOW)
})

it('keeps the selected month when switching from Plan to Budget', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('tab', { name: /budget/i }))
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-06-01')
})

it('the keyboard pages the window one month at a time, clamped at the budget start', async () => {
  usePlanHandlers()
  // the budget's start month: no history column, the selected month comes first
  useBudgetPeriodStore.setState({ selectedDate: '2026-01-01' })
  const user = userEvent.setup()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')
  const headerCols = () => within(screen.getByTestId('plan-month-header')).getAllByRole('columnheader')
  const months = () => headerCols().map((c) => c.getAttribute('data-month'))
  expect(months()).toEqual(['2026-01-01', '2026-02-01', '2026-03-01'])
  expect(headerCols()[0]).toHaveAttribute('data-selected-col', 'true')

  await user.click(screen.getByTestId('plan-cell-cat-freelance:0'))
  grid.focus()
  // ArrowLeft on the first month: nothing before the start to page to, and the
  // selection stays on that month
  fireEvent.keyDown(grid, { key: 'ArrowLeft' })
  fireEvent.keyDown(grid, { key: 'ArrowLeft' })
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-01-01')
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.BUDGET_PLAN_CHANGE_WINDOW)
  expect(screen.getByTestId('plan-cell-cat-freelance:0')).toHaveAttribute('aria-selected', 'true')

  // off the last column the window itself moves by one month, even from the clamped
  // start window: Feb..Apr, with Mar selected after its history month
  for (let i = 0; i < 3; i++) {
    fireEvent.keyDown(grid, { key: 'ArrowRight' })
  }
  await waitFor(() => expect(months()).toEqual(['2026-02-01', '2026-03-01', '2026-04-01']))
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-03-01')
  expect(headerCols()[1]).toHaveAttribute('data-selected-col', 'true')
  expect(screen.getByTestId('plan-cell-cat-freelance:2')).toHaveAttribute('aria-selected', 'true')

  // and back: off the first month the window returns to the start, the selection
  // staying in the first column
  for (let i = 0; i < 3; i++) {
    fireEvent.keyDown(grid, { key: 'ArrowLeft' })
  }
  await waitFor(() => expect(months()).toEqual(['2026-01-01', '2026-02-01', '2026-03-01']))
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-02-01')
  expect(screen.getByTestId('plan-cell-cat-freelance:0')).toHaveAttribute('aria-selected', 'true')
})

it('one visible month (narrow screen): the selected month is the only column, and the arrows page it', async () => {
  // under the three-month floor planVisibleCount collapses the grid to one column
  const widthSpy = vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(300)
  try {
    usePlanHandlers()
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
    const user = userEvent.setup()
    renderPage()
    const grid = await screen.findByTestId('plan-sheet')
    const headerCols = () => within(screen.getByTestId('plan-month-header')).getAllByRole('columnheader')
    const months = () => headerCols().map((c) => c.getAttribute('data-month'))
    await waitFor(() => expect(months()).toEqual(['2026-07-01']))
    expect(headerCols()[0]).toHaveAttribute('data-selected-col', 'true')

    await user.click(screen.getByTestId('plan-cell-cat-freelance:0'))
    grid.focus()
    fireEvent.keyDown(grid, { key: 'ArrowRight' })
    await waitFor(() => expect(months()).toEqual(['2026-08-01']))
    expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-08-01')

    // the only column is the first one too: ← pages back a month
    fireEvent.keyDown(grid, { key: 'ArrowLeft' })
    await waitFor(() => expect(months()).toEqual(['2026-07-01']))
    expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-07-01')
  } finally {
    widthSpy.mockRestore()
  }
})

it('an ended budget: the keyboard never pages past the end month, and pages back from it', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () =>
      HttpResponse.json({
        success: true,
        message: '',
        data: { item: { ...fixtureWireBudget, meta: { ...fixtureWireBudget.meta, endedAt: '2026-08-01 00:00:00' } } },
      }),
    ),
    planHandler(),
  )
  // the end month selected: the window ends there, so Aug is the last column
  useBudgetPeriodStore.setState({ selectedDate: '2026-08-01' })
  const user = userEvent.setup()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')
  const headerCols = () => within(screen.getByTestId('plan-month-header')).getAllByRole('columnheader')
  const months = () => headerCols().map((c) => c.getAttribute('data-month'))
  await waitFor(() => expect(months()).toEqual(['2026-06-01', '2026-07-01', '2026-08-01']))
  expect(headerCols()[2]).toHaveAttribute('data-selected-col', 'true')

  await user.click(screen.getByTestId('plan-cell-cat-freelance:2'))
  grid.focus()
  // past the last column at the end month: nothing to page to
  fireEvent.keyDown(grid, { key: 'ArrowRight' })
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-08-01')
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.BUDGET_PLAN_CHANGE_WINDOW)
  expect(months()).toEqual(['2026-06-01', '2026-07-01', '2026-08-01'])
  expect(screen.getByTestId('plan-cell-cat-freelance:2')).toHaveAttribute('aria-selected', 'true')

  // walk to the first month, then once more: the window moves back by one month
  for (let i = 0; i < 3; i++) {
    fireEvent.keyDown(grid, { key: 'ArrowLeft' })
  }
  await waitFor(() => expect(months()).toEqual(['2026-05-01', '2026-06-01', '2026-07-01']))
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-06-01')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_CHANGE_WINDOW)
})

it('the plan window position survives a remount', async () => {
  usePlanHandlers()
  const { unmount } = renderPage()
  await screen.findByTestId('plan-sheet')
  act(() => useBudgetPeriodStore.getState().stepPeriod(1))
  unmount()

  renderPage()
  await screen.findByTestId('plan-sheet')
  const cols = within(screen.getByTestId('plan-month-header')).getAllByRole('columnheader')
  expect(cols.map((c) => c.getAttribute('data-month'))).toEqual(['2026-07-01', '2026-08-01', '2026-09-01'])
})

it('stepPeriod moves the selected month across years and fires BUDGET_PLAN_CHANGE_WINDOW only', () => {
  useBudgetPeriodStore.setState({ selectedDate: '2026-12-01' })
  useBudgetPeriodStore.getState().stepPeriod(1)
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2027-01-01')
  useBudgetPeriodStore.getState().stepPeriod(-2)
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-11-01')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_CHANGE_WINDOW)
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.BUDGET_CHANGE_DATE)
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
  useBudgetPeriodStore.setState({ selectedDate: '2026-08-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')

  // pe1's second visible column (Jul/Aug/Sep window -> Aug); Enter edits it in place
  const cell = screen.getByTestId('plan-cell-pe1:1')
  await user.click(cell)
  await user.keyboard('{Enter}')
  const input = await screen.findByRole('textbox', { name: 'Plan for Living, August' })
  await user.clear(input)
  await user.type(input, '350')
  await user.keyboard('{Enter}')

  await waitFor(() => expect(body).toEqual({ budgetId: 'b1', elementId: 'pe1', period: '2026-08-01', amount: '350' }))
  expect(within(cell).getByTestId('cell-planned')).toHaveTextContent('350')
})

it('totals block renders one effective value per cell for income/expenses/transfers, and the running balance', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')

  const totalsBlock = screen.getByTestId('plan-totals')
  expect(within(totalsBlock).getByText('Income')).toBeInTheDocument()
  expect(within(totalsBlock).getByText('Expenses')).toBeInTheDocument()
  expect(within(totalsBlock).getByText('Transfers')).toBeInTheDocument()
  expect(within(screen.getByTestId('plan-balance-row')).getByText('Balance')).toBeInTheDocument()

  // window is Jun/Jul/Aug (visible=3 in jsdom, firstMonth pinned above); the last
  // visible column (index 2) is Aug, the fixture's last fetched month (index 3).
  // Both the totals rows and the running balance are computed here via the math
  // core, not hand-derived.
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const ex = makePlanExchange(plan, [fixtureUsd, fixtureEur])
  const totals = planTotals(plan, ex)
  const balance = balanceRow(plan, totals, ex)

  const rows = within(totalsBlock).getAllByRole('row')
  const [incomeRow, expensesRow, transfersRow] = rows
  const augTotals = totals[2]

  expect(within(incomeRow).getByText(moneyFormat(augTotals.effectiveIncome, fixtureUsd, { showCurrency: false, useNativePrecision: false }))).toBeInTheDocument()
  expect(within(incomeRow).queryByTestId('cell-actual')).not.toBeInTheDocument()
  expect(within(incomeRow).queryByTestId('cell-planned')).not.toBeInTheDocument()

  expect(within(expensesRow).getByText(moneyFormat(augTotals.effectiveExpense, fixtureUsd, { showCurrency: false, useNativePrecision: false }))).toBeInTheDocument()
  expect(within(expensesRow).queryByTestId('cell-actual')).not.toBeInTheDocument()
  expect(within(expensesRow).queryByTestId('cell-planned')).not.toBeInTheDocument()

  // Aug is the last visible column (index 2); nothing crossed, so assert the cell by
  // testid — the formatted 0.00 repeats across the row
  expect(within(transfersRow).getByTestId('plan-totals-transfers-2')).toHaveTextContent(
    moneyFormat(augTotals.transfersNet, fixtureUsd, { showCurrency: false, useNativePrecision: false }),
  )
  expect(within(transfersRow).queryByTestId('cell-actual')).not.toBeInTheDocument()
  expect(within(transfersRow).queryByTestId('cell-planned')).not.toBeInTheDocument()

  // effectiveNet is no longer a footer line of its own; the running balance chains on it
  const expected = moneyFormat(balance[3], fixtureUsd, { showCurrency: false, useNativePrecision: false })
  expect(screen.getByTestId('plan-balance-2')).toHaveTextContent(expected)
})

it('an open section or folder line shows its sums only after Σ; a folded one always shows them', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const line = () => screen.getByTestId('plan-section-line-expense')
  const folderLine = () => within(screen.getByTestId('plan-folder-bf1')).getAllByRole('row')[0]
  // open: no figures, only the month cells (lines and tint stay)
  expect(within(line()).queryAllByTestId(/^plan-sum-/)).toHaveLength(0)
  expect(within(folderLine()).queryAllByTestId(/^plan-sum-/)).toHaveLength(0)
  // Σ shows that line's sums, and only that line's
  await user.click(within(line()).getByRole('button', { name: /show sums.*expenses/i }))
  expect(within(line()).getAllByTestId(/^plan-sum-/)).toHaveLength(3)
  expect(within(folderLine()).queryAllByTestId(/^plan-sum-/)).toHaveLength(0)
  expect(useBudgetPeriodStore.getState().planSumsShown).toEqual({ expense: true })
  expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_TOGGLE_SUMS)
  // pressed again, they go
  await user.click(within(line()).getByRole('button', { name: /hide sums.*expenses/i }))
  expect(within(line()).queryAllByTestId(/^plan-sum-/)).toHaveLength(0)
  // folded: the sums are the only figures left, so they show, and there is no Σ
  await user.click(within(folderLine()).getByRole('button', { name: 'Essentials' }))
  expect(within(folderLine()).getAllByTestId(/^plan-sum-/)).toHaveLength(3)
  expect(within(folderLine()).queryByRole('button', { name: /sums/i })).not.toBeInTheDocument()
})

it('section and folder sums read actual · plan up to the selected month, plan alone after it', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', planSumsShown: { bf1: true } })
  renderPage()
  await screen.findByTestId('plan-sheet')
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const ex = makePlanExchange(plan, [fixtureUsd, fixtureEur])
  const fmt = (v: string) => moneyFormat(v, fixtureUsd, { showCurrency: false, useNativePrecision: false })
  // window Jun/Jul/Aug = fetched months 1..3; Essentials (bf1) holds pe1 alone
  const pe1 = plan.structure.elements.find((el) => el.id === 'pe1')!
  const sums = planGroupSums([pe1], plan.months, (m) => plan.months.indexOf(m), ex)
  const folderLine = within(screen.getByTestId('plan-folder-bf1')).getAllByRole('row')[0]
  const cell = (col: number) => within(folderLine).getByTestId(`plan-sum-${col}`)
  const plannedText = (v: string) => (isZero(v) ? '' : fmt(v))
  // two separate figures, the dot between them; one line, never wrapped
  expect(cell(0).textContent).toBe(`${fmt(sums[1].actual)}·${plannedText(sums[1].planned)}`)
  expect(cell(1).textContent).toBe(`${fmt(sums[2].actual)}·${plannedText(sums[2].planned)}`)
  // Aug is after the selected July: its plan alone, no actual
  expect(cell(2).textContent).toBe(plannedText(sums[3].planned))
})

it('a past cell with an actual and no plan reads the actual alone, with no dangling dot', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  // Food in July (column 1): 125 spent, nothing planned
  const cell = screen.getByTestId('plan-cell-cat-food:1')
  expect(within(cell).getByTestId('cell-actual')).toHaveTextContent('125.00')
  expect(cell).not.toHaveTextContent('·')
  // with both figures the dot separates them
  expect(screen.getByTestId('plan-cell-cat-food:0')).toHaveTextContent(/130\.00\s*·\s*150\.00/)
})

it('a planned month with nothing spent yet reads "— · plan"; an empty one stays blank', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  // Freelance in June (history): nothing received against a 500 plan
  const jun = screen.getByTestId('plan-cell-cat-freelance:0')
  expect(within(jun).queryByTestId('cell-actual')).not.toBeInTheDocument()
  expect(within(jun).getByTestId('cell-no-actual')).toHaveTextContent('—')
  expect(jun).toHaveTextContent(/—\s*·\s*500\.00/)
  // the dash is a mark, not a link to an empty transactions list
  expect(within(jun).queryByRole('button', { name: /—/ })).not.toBeInTheDocument()
  // Salaries in the selected July: the same
  expect(within(screen.getByTestId('plan-cell-ie1:1')).getByTestId('cell-no-actual')).toBeInTheDocument()
  // Unused: no actual and no plan, so nothing at all, not 0.00
  const dormant = screen.getByTestId('plan-cell-cat-dormant:0')
  expect(within(dormant).queryByTestId('cell-actual')).not.toBeInTheDocument()
  expect(within(dormant).queryByTestId('cell-no-actual')).not.toBeInTheDocument()
  expect(dormant).not.toHaveTextContent(/\S/)
})

it('the plan figure never wraps or shrinks; only the actual may be cut, its full value in the tooltip', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  const cell = screen.getByTestId('plan-cell-cat-food:0')
  expect(within(cell).getByTestId('cell-planned')).toHaveClass('shrink-0', 'whitespace-nowrap')
  const actual = within(cell).getByTestId('cell-actual')
  expect(actual).toHaveClass('min-w-0', 'truncate')
  expect(actual.getAttribute('title')).toContain('130.00')
  expect(cell).toHaveClass('whitespace-nowrap')
})

it('a section\'s last line draws no bottom rule over the next section\'s top rule, through the drag wrappers too', async () => {
  // no Uncategorized rows: each section then ends inside a folder group and a row's drag wrapper
  const plan = {
    ...fixtureWirePlan,
    structure: { ...fixtureWirePlan.structure, elements: fixtureWirePlan.structure.elements.filter((el) => el.id !== 'uncategorized') },
  }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(plan),
  )
  const css = readFileSync(resolve(process.cwd(), 'src/index.css'), 'utf8')
  const rule = css.match(/\.plan-band[^{]*\{[^}]*\}/)![0]
  const style = document.createElement('style')
  style.textContent = `.border-b { border-bottom: 1px solid; }\n${rule}`
  document.head.appendChild(style)
  try {
    renderPage()
    await screen.findByTestId('plan-sheet')
    for (const id of ['plan-section-income', 'plan-section-expense']) {
      const lines = Array.from(screen.getByTestId(id).querySelectorAll<HTMLElement>('[data-plan-line]'))
      const last = lines[lines.length - 1]
      // the drag wrapper sits between the band and the line
      expect(last.parentElement).not.toBe(screen.getByTestId(id))
      expect(getComputedStyle(last).borderBottomWidth).toMatch(/^0(px)?$/)
      for (const line of lines.slice(0, -1)) {
        expect(getComputedStyle(line).borderBottomWidth).toBe('1px')
      }
    }
  } finally {
    style.remove()
  }
})

it('the Archived line sums its rows: their actuals, never their plans', async () => {
  const plan = {
    ...fixtureWirePlan,
    structure: {
      ...fixtureWirePlan.structure,
      elements: [
        ...fixtureWirePlan.structure.elements,
        {
          id: 'arch-1', type: 1, name: 'Old hobby', icon: 'delete', currencyId: 'cur-usd', isArchived: 1, folderId: null, position: 9, ownerUserId: 'u1',
          cells: [{ actual: '0', planned: '' }, { actual: '18.53', planned: '40' }, { actual: '0', planned: '' }, { actual: '0', planned: '' }],
          children: [],
        },
      ],
    },
  }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(plan),
  )
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', planSumsShown: { archived: true } })
  renderPage()
  const line = await screen.findByTestId('plan-section-line-archived')
  // June: 18.53 spent; the archived row's 40 plan is not part of the plan
  expect(within(line).getByTestId('plan-sum-0').textContent).toBe('18.53')
})

it('with folders, an expense section names its folder-less rows under a No folder line that folds like a folder', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', planSumsShown: { __no_folder__: true } })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const expense = screen.getByTestId('plan-section-expense')
  const noFolder = within(expense).getByTestId('plan-folder-__no_folder__')
  const button = within(noFolder).getByRole('button', { name: 'No folder' })
  // after the real folders, with its own sums
  expect(screen.getByTestId('plan-folder-bf1').compareDocumentPosition(noFolder) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(within(noFolder).getAllByTestId(/^plan-sum-/)).toHaveLength(3)
  // its rows sit a folder's step in, like the rows of a real folder
  const food = document.querySelector('[data-row-id="cat-food:1"]') as HTMLElement
  expect(noFolder).toContainElement(food)
  expect(within(food).getByTitle('Food').closest('[role="rowheader"]')!.className).toContain('pl-9')

  // the line is no stop: ArrowDown from Essentials' last row lands on Food, the
  // first folder-less row, in the same month
  await user.click(screen.getByTestId('plan-cell-pe1:1'))
  screen.getByTestId('plan-sheet').focus()
  await user.keyboard('{ArrowDown}')
  expect(button.closest('[role="rowheader"]')).not.toHaveAttribute('aria-selected')
  expect(screen.getByTestId('plan-cell-cat-food:1')).toHaveAttribute('aria-selected', 'true')

  // folding hides the folder-less rows only; Uncategorized stays. The click on the
  // line folds it and leaves the selection where it was
  await user.click(screen.getByTestId('plan-cell-pe1:1'))
  await user.click(button)
  expect(useBudgetPeriodStore.getState().planFolds.__no_folder__).toBe(true)
  expect(document.querySelector('[data-row-id="cat-food:1"]')).not.toBeInTheDocument()
  expect(document.querySelector('[data-row-id="uncategorized:1"]')).toBeInTheDocument()
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('aria-selected', 'true')
  // and the keyboard skips the folded rows
  screen.getByTestId('plan-sheet').focus()
  await user.keyboard('{ArrowDown}')
  expect(within(document.querySelector('[data-row-id="uncategorized:1"]') as HTMLElement).getAllByRole('gridcell')[1]).toHaveAttribute('aria-selected', 'true')
})

it('an income section names its folder-less rows only when it has a folder, and shares the Budget view fold key', async () => {
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const { unmount } = renderPage()
  await screen.findByTestId('plan-sheet')
  // no income folder: the loose income rows stand alone at the top step
  const income = screen.getByTestId('plan-section-income')
  expect(within(income).queryByText('No folder')).not.toBeInTheDocument()
  const freelance = document.querySelector('[data-row-id="cat-freelance:3"]') as HTMLElement
  expect(within(freelance).getByTitle('Freelance').closest('[role="rowheader"]')!.className).toContain('pl-6')
  unmount()

  server.use(
    planHandler({
      ...plan,
      structure: {
        ...plan.structure,
        folders: [...plan.structure.folders, { id: 'bf-bonus', name: 'Bonuses Folder', position: 1 }],
        elements: plan.structure.elements.map((el) => (el.id === 'ie1' ? { ...el, folderId: 'bf-bonus' } : el)),
      },
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const noFolder = within(screen.getByTestId('plan-section-income')).getByTestId('plan-folder-__income__no_folder__')
  expect(noFolder).toContainElement(document.querySelector('[data-row-id="cat-freelance:3"]') as HTMLElement)
  await user.click(within(noFolder).getByRole('button', { name: 'No folder' }))
  expect(useBudgetPeriodStore.getState().planFolds.__income__no_folder__).toBe(true)
  expect(document.querySelector('[data-row-id="cat-freelance:3"]')).not.toBeInTheDocument()
})

it('totals: Income, Expenses, Savings, Total savings, then a sticky Balance; Transfers only when non-zero', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  const lines = within(screen.getByTestId('plan-totals')).getAllByTestId(/^plan-total-/).map((l) => l.getAttribute('data-testid'))
  expect(lines[0]).toBe('plan-total-income')
  expect(lines[1]).toBe('plan-total-expenses')
  // the fixture's June crossed the budget boundary, and June is on screen
  expect(lines).toContain('plan-total-transfers')
  expect(within(screen.getByTestId('plan-balance-row')).getByTestId('plan-total-balance')).toBeInTheDocument()
})

it('totals: no Transfers line when nothing crossed the boundary in the visible months', async () => {
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler({ ...plan, transfers: [] }),
  )
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  const lines = within(screen.getByTestId('plan-totals')).getAllByTestId(/^plan-total-/).map((l) => l.getAttribute('data-testid'))
  expect(lines).toEqual(['plan-total-income', 'plan-total-expenses'])
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
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')

  const folder = screen.getByTestId('plan-folder-bf1')
  const nameButton = within(folder).getByRole('button', { name: 'Essentials' })
  const header = nameButton.parentElement!.parentElement as HTMLElement
  expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()

  // the blank part of the header row folds…
  await user.click(header)
  expect(document.querySelector('[data-row-id="pe1:0"]')).not.toBeInTheDocument()
  expect(nameButton).toHaveAttribute('aria-expanded', 'false')
  // …and unfolds
  await user.click(header)
  expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()

  // the ⋮ menu opens its menu, not a fold
  await user.click(within(folder).getByRole('button', { name: 'menu Essentials' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
  await user.keyboard('{Escape}')
  expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()
  expect(nameButton).toHaveAttribute('aria-expanded', 'true')
})

it('a folder ⋮ menu renames the folder, and deletes it only when it has no members', async () => {
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

  // Essentials has a member: rename offered, delete greyed out with the reason
  await user.click(await screen.findByRole('button', { name: 'menu Essentials' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: /Delete folder/ })).toHaveAttribute('aria-disabled', 'true')
  expect(screen.getByRole('menuitem', { name: /Delete folder/ })).toHaveTextContent('(not empty)')
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
  await user.click(screen.getByRole('button', { name: 'menu Fun' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete folder' }))
  expect(deleteBody).toBeUndefined()
  const confirm = await screen.findByRole('dialog', { name: 'Delete folder?' })
  expect(within(confirm).getByText('Are you sure you want to delete the folder “Fun”?')).toBeInTheDocument()
  await user.click(within(confirm).getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(deleteBody).toEqual({ budgetId: 'b1', id: 'bf-empty' }))
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
  useBudgetPeriodStore.setState({ selectedDate: '2026-06-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')

  // guest role: pe1 would normally be editable for the owner, but not here — no
  // fill handle, and Enter opens no in-cell editor
  const pe1Cell = screen.getByTestId('plan-cell-pe1:1')
  await user.click(pe1Cell)
  expect(within(pe1Cell).queryByTestId('fill-handle')).not.toBeInTheDocument()
  await user.keyboard('{Enter}')
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

  // children never carry their own limit, regardless of role
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  await user.click(within(pe1Row).getByRole('button', { name: 'Expand' }))
  const childCell = await screen.findByTestId('plan-cell-cat-rent:1')
  expect(within(childCell).queryByRole('button')).not.toBeInTheDocument()

  // both uncategorized rows are element rows — income and expense — and neither is
  // ever editable
  const uncatCells = screen.getAllByTestId('plan-cell-uncategorized:1')
  expect(uncatCells).toHaveLength(2)
  for (const cell of uncatCells) {
    await user.click(cell)
    expect(within(cell).queryByTestId('fill-handle')).not.toBeInTheDocument()
    await user.keyboard('{Enter}')
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  }
})

it('scrolls a keyboard-selected row back into view, clearing the sticky balance row', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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

it('scrolls a keyboard-selected row back into view below the sticky month header', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')

  await user.click(screen.getByTestId('plan-cell-pe1:0'))

  // the sticky header covers the scroller's top 30px; the selected cell is half under it
  const rect = (top: number, bottom: number) => () => ({ top, bottom, height: bottom - top, left: 0, right: 0, width: 0, x: 0, y: top, toJSON: () => {} }) as DOMRect
  grid.getBoundingClientRect = rect(0, 200)
  screen.getByTestId('plan-month-header').getBoundingClientRect = rect(0, 30)
  screen.getByTestId('plan-balance-row').getBoundingClientRect = rect(160, 200)
  const target = screen.getByTestId('plan-cell-cat-food:0')
  const targetCell = document.getElementById(target.id) as HTMLElement
  targetCell.getBoundingClientRect = rect(10, 40)

  grid.scrollTop = 100
  await user.click(screen.getByTestId('plan-cell-cat-food:0'))

  // 30 (header bottom) - 10 (cell top) = 20px back up
  expect(grid.scrollTop).toBe(80)
})

it('arrow keys move the selection and shift the window at the edges', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const grid = screen.getByTestId('plan-sheet')

  // window is Jun/Jul/Aug; select pe1's first visible column (Jun)
  await user.click(screen.getByTestId('plan-cell-pe1:0'))
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')

  // ArrowLeft on column 0 shifts the window back a month, keeping the row and the
  // column: there is no stop left of the months
  grid.focus()
  await user.keyboard('{ArrowLeft}')
  await waitFor(() => expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-06-01'))
  expect(await screen.findByTestId('plan-cell-pe1:0')).toHaveAttribute('data-month', '2026-05-01')
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')
  // the arrows never fold a row: Living's breakdown stays closed
  expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument()

  // ArrowDown walks on: pe1 is the only row in the Essentials folder, and the No
  // folder line is no stop, so the next row is its first, Food
  grid.focus()
  await user.keyboard('{ArrowDown}')
  expect(screen.getByTestId('plan-cell-cat-food:0')).toHaveAttribute('aria-selected', 'true')
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'false')

  // ArrowRight twice reaches the last visible column (col 2 of 3) without shifting
  grid.focus()
  await user.keyboard('{ArrowRight}{ArrowRight}')
  expect(screen.getByTestId('plan-cell-cat-food:2')).toHaveAttribute('aria-selected', 'true')
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-06-01')

  // a third ArrowRight from the last column shifts the window forward, keeping row+col
  grid.focus()
  await user.keyboard('{ArrowRight}')
  await waitFor(() => expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-07-01'))
  expect(await screen.findByTestId('plan-cell-cat-food:2')).toHaveAttribute('aria-selected', 'true')
})

it('Enter opens the in-cell editor on an editable cell and is inert on read-only cells', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const grid = screen.getByTestId('plan-sheet')

  await user.click(screen.getByTestId('plan-cell-pe1:1'))
  grid.focus()
  await user.keyboard('{Enter}')
  expect(await screen.findByRole('textbox', { name: 'Plan for Living, July' })).toBeInTheDocument()
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('textbox')).not.toBeInTheDocument())

  // uncategorized rows are never editable, regardless of role
  const uncatCell = screen.getAllByTestId('plan-cell-uncategorized:1')[0]
  await user.click(uncatCell)
  grid.focus()
  await user.keyboard('{Enter}')
  await user.keyboard('5')
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()

  // children never carry their own limit (and are not selectable at all)
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  await user.click(within(pe1Row).getByRole('button', { name: 'Expand' }))
  const childCell = await screen.findByTestId('plan-cell-cat-rent:1')
  await user.click(childCell)
  grid.focus()
  await user.keyboard('{Enter}')
  expect(screen.queryByRole('textbox', { name: /^Plan for/ })).not.toBeInTheDocument()
})

it('cells expose the aria label and aria-selected', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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

it('keystrokes inside the in-cell editor reach it, not the grid: ArrowLeft moves the caret and Enter commits set-limit', async () => {
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
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const grid = screen.getByTestId('plan-sheet')

  // pe1's second visible column (Jun/Jul/Aug window -> Jul)
  await user.click(screen.getByTestId('plan-cell-pe1:1'))
  grid.focus()
  await user.keyboard('{Enter}')
  const input = await screen.findByRole('textbox', { name: 'Plan for Living, July' })

  // ArrowLeft while the editor has focus must move the caret, not the grid's
  // window/selection — the grid must not intercept it.
  const monthBefore = useBudgetPeriodStore.getState().selectedDate
  await user.clear(input)
  await user.type(input, '12{ArrowLeft}3')
  expect(input).toHaveValue('132')
  expect(useBudgetPeriodStore.getState().selectedDate).toBe(monthBefore)

  // Enter inside the input commits, not swallowed by the grid's own Enter handling
  await user.keyboard('{Enter}')
  await waitFor(() => expect(body).toEqual({ budgetId: 'b1', elementId: 'pe1', period: '2026-07-01', amount: '132' }))
})

// The keyboard cursor lives in month cells only. A name is a rowheader, never a
// cell the selection lands on: ← on the first month pages the window instead.
it('a name is not a stop: a rowheader the keyboard never lands on; ← on the first month pages instead, and Space folds nothing', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const grid = screen.getByTestId('plan-sheet')
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  const name = within(pe1Row).getByTitle('Living').closest('[role="rowheader"]') as HTMLElement
  expect(name).not.toHaveAttribute('id')
  expect(name).not.toHaveAttribute('aria-selected')
  expect(within(pe1Row).getAllByRole('gridcell').every((c) => c.hasAttribute('data-col'))).toBe(true)

  await user.click(screen.getByTestId('plan-cell-pe1:0'))
  grid.focus()
  await user.keyboard('{ArrowLeft}')
  await waitFor(() => expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-06-01'))
  expect(await screen.findByTestId('plan-cell-pe1:0')).toHaveAttribute('data-month', '2026-05-01')
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')
  expect(grid.getAttribute('aria-activedescendant')).toBe(screen.getByTestId('plan-cell-pe1:0').id)

  // Space folds nothing and opens no editor
  await user.keyboard(' ')
  expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument()
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')
})

// The name is not a selection target and not a fold toggle: only the chevron unfolds
// the children, and a click on the name leaves the selection where it was.
it('clicking a name selects nothing and does not expand; only the chevron toggles the children', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement

  await user.click(within(pe1Row).getByTitle('Living'))
  expect(grid.querySelector('[role="gridcell"][aria-selected="true"]')).toBeNull()
  expect(grid).not.toHaveAttribute('aria-activedescendant')
  expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument()

  // with a month cell selected, a click on a name keeps that selection
  await user.click(screen.getByTestId('plan-cell-cat-food:1'))
  await user.click(within(pe1Row).getByTitle('Living'))
  expect(screen.getByTestId('plan-cell-cat-food:1')).toHaveAttribute('aria-selected', 'true')
  expect(grid.querySelectorAll('[role="gridcell"][aria-selected="true"]')).toHaveLength(1)

  const chevron = within(pe1Row).getByRole('button', { name: 'Expand' })
  expect(chevron).toHaveAttribute('aria-expanded', 'false')
  await user.click(chevron)
  expect(await screen.findByTestId('plan-cell-cat-rent:0')).toBeInTheDocument()
  expect(within(pe1Row).getByRole('button', { name: 'Collapse' })).toHaveAttribute('aria-expanded', 'true')
  await user.click(within(pe1Row).getByRole('button', { name: 'Collapse' }))
  await waitFor(() => expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument())
})

it('← and → never fold a row: they walk the months whether its breakdown is open or closed', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const grid = screen.getByTestId('plan-sheet')
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement

  // closed: the arrows move between Living's months and leave it closed
  await user.click(screen.getByTestId('plan-cell-pe1:1'))
  grid.focus()
  await user.keyboard('{ArrowLeft}')
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')
  await user.keyboard('{ArrowRight}')
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('aria-selected', 'true')
  expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument()

  // opened by the chevron: the arrows still walk, and it stays open
  await user.click(within(pe1Row).getByRole('button', { name: 'Expand' }))
  expect(await screen.findByTestId('plan-cell-cat-rent:0')).toBeInTheDocument()
  grid.focus()
  await user.keyboard('{ArrowLeft}')
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')
  await user.keyboard('{ArrowRight}')
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('aria-selected', 'true')
  expect(screen.getByTestId('plan-cell-cat-rent:0')).toBeInTheDocument()
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-07-01')
})

it('section and folder lines are not stops: a click folds a folder without selecting it, ↑/↓ step over the lines and skip folded rows', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const grid = screen.getByTestId('plan-sheet')
  const folder = screen.getByTestId('plan-folder-bf1')
  const nameButton = within(folder).getByRole('button', { name: 'Essentials' })
  const row = nameButton.closest('[role="row"]') as HTMLElement
  const header = nameButton.parentElement!.parentElement as HTMLElement
  expect(nameButton.closest('[role="rowheader"]')).not.toHaveAttribute('aria-selected')
  expect(nameButton.closest('[role="rowheader"]')).not.toHaveAttribute('id')
  // the income side's Uncategorized closes the income section; its July cell
  const incomeUncatJul = () => within(document.querySelector('[data-row-id="uncategorized:3"]') as HTMLElement).getAllByRole('gridcell')[1]

  // ↓ from the income section's last row crosses the Expenses section line and the
  // Essentials folder line straight to Living, keeping the column; ↓ again steps over
  // the No folder line to Food
  await user.click(incomeUncatJul())
  grid.focus()
  await user.keyboard('{ArrowDown}')
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('aria-selected', 'true')
  await user.keyboard('{ArrowDown}')
  expect(screen.getByTestId('plan-cell-cat-food:1')).toHaveAttribute('aria-selected', 'true')
  await user.keyboard('{ArrowUp}{ArrowUp}')
  expect(incomeUncatJul()).toHaveAttribute('aria-selected', 'true')

  // a click anywhere on the folder line folds it and leaves the selection alone
  await user.click(header)
  expect(useBudgetPeriodStore.getState().planFolds.bf1).toBe(true)
  expect(document.querySelector('[data-row-id="pe1:0"]')).not.toBeInTheDocument()
  expect(incomeUncatJul()).toHaveAttribute('aria-selected', 'true')
  expect(row).not.toHaveAttribute('data-crosshair')

  // the folded folder's rows are skipped, and the arrows never unfold it
  grid.focus()
  await user.keyboard('{ArrowDown}')
  expect(screen.getByTestId('plan-cell-cat-food:1')).toHaveAttribute('aria-selected', 'true')
  await user.keyboard('{ArrowRight}{ArrowLeft}{ArrowUp}')
  expect(useBudgetPeriodStore.getState().planFolds.bf1).toBe(true)
  expect(incomeUncatJul()).toHaveAttribute('aria-selected', 'true')

  // a click on the line unfolds it again
  await user.click(header)
  expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()
})

// Editing an element lives in the row ⋮ menu: Enter on a month cell only ever edits
// that month's plan.
it('the row ⋮ menu edits an envelope, category, or tag; Enter on a month cell never opens their dialog', async () => {
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
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')

  // Enter on Living's July cell edits the plan, not the envelope
  await user.click(screen.getByTestId('plan-cell-pe1:1'))
  grid.focus()
  await user.keyboard('{Enter}')
  expect(await screen.findByRole('textbox', { name: 'Plan for Living, July' })).toBeInTheDocument()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  await user.keyboard('{Escape}')

  // envelope -> the envelope dialog, prefilled, and it saves through update-envelope
  await user.click(screen.getByRole('button', { name: 'menu Living' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  const envelopeDialog = await screen.findByRole('dialog', { name: 'Edit envelope' })
  expect(within(envelopeDialog).getByDisplayValue('Living')).toBeInTheDocument()
  await user.click(within(envelopeDialog).getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(envelopeBody).toMatchObject({ budgetId: 'b1', id: 'pe1', name: 'Living' }))
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Edit envelope' })).not.toBeInTheDocument())

  // own category -> the category dialog, prefilled, saving through update-category
  await user.click(screen.getByRole('button', { name: 'menu Food' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  const categoryDialog = await screen.findByRole('dialog', { name: 'Edit category' })
  const nameInput = within(categoryDialog).getByDisplayValue('Food')
  await user.clear(nameInput)
  await user.type(nameInput, 'Groceries')
  await user.click(within(categoryDialog).getByRole('button', { name: /update/i }))
  await waitFor(() => expect(categoryBody).toMatchObject({ id: 'cat-food', name: 'Groceries', icon: 'restaurant' }))
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Edit category' })).not.toBeInTheDocument())

  // own tag -> the tag dialog, saving through update-tag
  await user.click(screen.getByRole('button', { name: 'menu vacation' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
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
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', foldBudgetId: 'b1', unfoldedElements: { pe1: true } })
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

  // ArrowDown from the expanded parent skips its children (and the No folder line):
  // the next root row
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
// Archive section.
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
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')

  // Jun..Aug: only the row with an August plan has a visible value
  const section = await screen.findByTestId('plan-section-archived')
  expect(within(section).getByTitle('Only Aug')).toBeInTheDocument()
  expect(within(section).queryByTitle('Only May')).not.toBeInTheDocument()
  expect(within(section).queryByTitle('Nothing')).not.toBeInTheDocument()

  // May..Jul: the May spend comes on screen, the August plan leaves it
  act(() => useBudgetPeriodStore.getState().stepPeriod(-1))
  await waitFor(() => expect(within(screen.getByTestId('plan-section-archived')).getByTitle('Only May')).toBeInTheDocument())
  expect(within(screen.getByTestId('plan-section-archived')).queryByTitle('Only Aug')).not.toBeInTheDocument()

  // a window with no archived values at all drops the section entirely: Sep..Nov
  // (only May and Aug carry values, and neither is visible then)
  for (let i = 0; i < 4; i++) {
    act(() => useBudgetPeriodStore.getState().stepPeriod(1))
  }
  await waitFor(() => expect(screen.queryByTestId('plan-section-archived')).not.toBeInTheDocument())
})

it('without the right to edit, Enter on a month cell explains nothing and opens nothing: guest role, foreign categories and tags', async () => {
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
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')

  // the name was the only place Enter opened an element's dialog (or its no-access
  // toast); a read-only month cell takes Enter as nothing at all
  for (const cellId of ['plan-cell-pe1:1', 'plan-cell-cat-food:1', 'plan-cell-tag1:1']) {
    await user.click(screen.getByTestId(cellId))
    grid.focus()
    await user.keyboard('{Enter}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    expect(screen.getByTestId(cellId)).toHaveAttribute('aria-selected', 'true')
  }
  expect(toast.error).not.toHaveBeenCalled()
  expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument()
})

it('budget-mode envelope dialog still offers expense categories only', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
  )
  const user = userEvent.setup()
  renderPage('/budget')
  await user.click(within(await screen.findByTestId('column-headers')).getByRole('button', { name: 'menu Expenses' }))
  await user.click(await screen.findByRole('menuitem', { name: 'New envelope' }))

  const dialog = await screen.findByRole('dialog', { name: 'New envelope' })
  expect(within(dialog).getByText('Food')).toBeInTheDocument()
  expect(within(dialog).queryByText('Salary')).not.toBeInTheDocument()
})

it('clicking a cell focuses the grid so arrow keys work immediately, no manual focus needed', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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

it('grid structure: sections are rowgroups, the sticky month header is the grid\'s first row, and aria-activedescendant tracks the selection', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')

  const grid = screen.getByTestId('plan-sheet')
  expect(grid).toHaveAttribute('role', 'grid')
  expect(screen.getByTestId('plan-section-income')).toHaveAttribute('role', 'rowgroup')
  expect(screen.getByTestId('plan-section-expense')).toHaveAttribute('role', 'rowgroup')
  expect(grid).not.toHaveAttribute('aria-activedescendant')

  // inside the scroller, so it shares the rows' width and stays put while they scroll
  const headerRow = screen.getByTestId('plan-month-header')
  expect(grid.firstElementChild).toBe(headerRow)
  expect(headerRow).toHaveClass('sticky', 'top-0')
  expect(headerRow).toHaveAttribute('role', 'row')

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

it('ArrowLeft on the first month does not page the window past the budget start', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-02-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const grid = screen.getByTestId('plan-sheet')

  // Jan (the budget start) is the history column, the first month on screen
  await user.click(screen.getByTestId('plan-cell-pe1:0'))
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('data-month', '2026-01-01')
  grid.focus()
  // the prev nav button is disabled here too: same clamp, keyboard path
  await user.keyboard('{ArrowLeft}{ArrowLeft}')
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-02-01')
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('data-month', '2026-01-01')
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.BUDGET_PLAN_CHANGE_WINDOW)
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

it('empty folders reorder among themselves inside the neutral band', async () => {
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
  renderPage()
  await screen.findByTestId('plan-sheet')

  // the empty folders' grips live in the neutral band, in neither side's band
  const neutral = screen.getByTestId('plan-section-neutral')
  expect(within(neutral).getByRole('button', { name: 'move folder Empty One' })).toBeInTheDocument()
  expect(within(neutral).getByRole('button', { name: 'move folder Empty Two' })).toBeInTheDocument()
  expect(within(screen.getByTestId('plan-section-expense')).queryByRole('button', { name: /move folder Empty/ })).not.toBeInTheDocument()

  // bands mount their DndContexts in DOM order: income, neutral, expense
  // folders reorder by their plain ids, as in the Budget view
  const neutralCtx = () => capturedDragContexts[capturedDragContexts.length - 2]
  act(() => neutralCtx().onDragStart({ active: { id: 'bf-e1' } }))
  act(() => neutralCtx().onDragEnd({ active: { id: 'bf-e1' }, over: { id: 'bf-e2' } }))
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
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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

  it('a fill drag on a row with a hover drag grip fills and never starts a row drag', async () => {
    // dnd-kit's PointerSensor activates at 4px of movement, and the fill drag moves
    // horizontally well past that. The two stay separate because the sensor's
    // activator is bound to the grip alone — the fill handle's pointerdown never
    // reaches it — so the row must not tear loose from the grid mid-fill.
    realDndContext = true
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
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('plan-sheet')
    const dragged = (id: string) => screen.getByRole('button', { name: `move ${id}` }).parentElement!.className.includes('opacity-40')

    await user.click(screen.getByTestId('plan-cell-pe1:0'))
    const handle = within(screen.getByTestId('plan-cell-pe1:0')).getByTestId('fill-handle')
    fireEvent.pointerDown(handle, { pointerId: 1, clientX: 0, clientY: 0, button: 0, isPrimary: true })
    fireEvent.pointerMove(handle, { pointerId: 1, clientX: 110, clientY: 0 })
    fireEvent.pointerMove(document, { pointerId: 1, clientX: 110, clientY: 0 })
    expect(dragged('pe1')).toBe(false)
    fireEvent.pointerUp(handle, { pointerId: 1 })
    await waitFor(() => expect(fillBody).toMatchObject({ elementId: 'pe1' }))

    // the same gesture on the grip does start a row drag: the check above can see one
    const grip = screen.getByRole('button', { name: 'move cat-food' })
    fireEvent.pointerDown(grip, { pointerId: 2, clientX: 0, clientY: 0, button: 0, isPrimary: true })
    fireEvent.pointerMove(document, { pointerId: 2, clientX: 0, clientY: 40 })
    await waitFor(() => expect(dragged('cat-food')).toBe(true))
    fireEvent.pointerUp(document, { pointerId: 2 })
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
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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
    useBudgetPeriodStore.setState({ selectedDate: '2026-06-01' })
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
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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
    expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-07-01')

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
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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

  it('compact mode: the fill handle does not render on a selected editable cell', async () => {
    mockCompactViewport()
    usePlanHandlers()
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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

  it('copy writes the selected cell: planned amount, 0 for an unset cell; a name is never copied', async () => {
    usePlanHandlers()
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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

    // the keyboard cannot reach the name: ← from the first month pages the window
    // and the copy is still the month cell's amount (pe1 May planned '200')
    fireEvent.keyDown(grid, { key: 'ArrowLeft' })
    fireEvent.keyDown(grid, { key: 'ArrowLeft' })
    fireEvent.keyDown(grid, { key: 'ArrowLeft' })
    await waitFor(() => expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('data-month', '2026-05-01'))
    const paged = clipboard()
    fireEvent.copy(grid, { clipboardData: paged })
    expect(paged.setData).toHaveBeenCalledWith('text/plain', '200')
    expect(paged.setData).not.toHaveBeenCalledWith('text/plain', 'Living')

    // a non-editable row copies too

    await user.click(screen.getAllByTestId('plan-cell-uncategorized:0')[0])
    const uncat = clipboard()
    fireEvent.copy(grid, { clipboardData: uncat })
    expect(uncat.setData).toHaveBeenCalledTimes(1)
  })

  it('paste writes a single amount into the selected editable cell through set-limit', async () => {
    const bodies: unknown[] = []
    useCapturingHandlers(bodies)
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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

  it('paste of non-numeric text, or onto a non-editable cell, writes nothing', async () => {
    const bodies: unknown[] = []
    useCapturingHandlers(bodies)
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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

    // a click on a name selects nothing, so the paste still targets the
    // uncategorized cell and is refused the same way
    await user.click(within(document.querySelector('[data-row-id="pe1:0"]') as HTMLElement).getByTitle('Living'))
    fireEvent.paste(grid, { clipboardData: clipboard('150') })
    expect(toast.error).toHaveBeenCalledTimes(3)

    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(bodies).toHaveLength(0)
  })

  it('paste inside the open in-cell editor is left to the input, not the grid', async () => {
    const bodies: unknown[] = []
    useCapturingHandlers(bodies)
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('plan-sheet')

    const cell = screen.getByTestId('plan-cell-pe1:0')
    await user.click(cell)
    await user.keyboard('{Enter}')
    const input = (await screen.findByRole('textbox', { name: 'Plan for Living, June' })) as HTMLInputElement
    expect(fireEvent.paste(input as HTMLInputElement, { clipboardData: clipboard('150') })).toBe(true)
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(bodies).toHaveLength(0)
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('Shift+ArrowRight extends a fill range from the selected cell; releasing Shift copies the value into it', async () => {
    const bodies: unknown[] = []
    useCapturingHandlers(bodies)
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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
    expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-07-01')
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
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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

  it('Shift+ArrowRight on a non-editable cell starts nothing and does not move the selection', async () => {
    const bodies: unknown[] = []
    useCapturingHandlers(bodies)
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('plan-sheet')
    const grid = screen.getByTestId('plan-sheet')

    const uncat = screen.getAllByTestId('plan-cell-uncategorized:0')[0]
    await user.click(uncat)
    fireEvent.keyDown(grid, { key: 'ArrowRight', shiftKey: true })
    expect(uncat).toHaveAttribute('aria-selected', 'true')
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
    useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('plan-sheet')

    const expenseSection = screen.getByTestId('plan-section-expense')
    const expensesButton = within(expenseSection).getByRole('button', { name: 'Expenses' })

    // expense rows present before folding: a folder row, a loose row, and the
    // uncategorized expense row that closes the band
    expect(document.querySelector('[data-row-id="pe1:0"]')).toBeInTheDocument()
    expect(document.querySelector('[data-row-id="cat-food:1"]')).toBeInTheDocument()
    expect(document.querySelector('[data-row-id="uncategorized:1"]')).toBeInTheDocument()
    // income rows present too
    expect(document.querySelector('[data-row-id="ie1:4"]')).toBeInTheDocument()

    await user.click(expensesButton)
    expect(document.querySelector('[data-row-id="pe1:0"]')).not.toBeInTheDocument()
    expect(document.querySelector('[data-row-id="cat-food:1"]')).not.toBeInTheDocument()
    // uncategorized is an element row of the band, so it folds away with it
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

  it('separates the sections with a heavier rule, not a gap, so the selected month tint never breaks', async () => {
    usePlanHandlers()
    renderPage()
    await screen.findByTestId('plan-sheet')

    const income = screen.getByTestId('plan-section-income')
    expect(income.classList.contains('plan-band-income')).toBe(true)
    // the month header's own rule sits right above the first section
    expect(income.classList.contains('border-t')).toBe(false)
    const expenseSection = screen.getByTestId('plan-section-expense')
    expect(expenseSection.classList.contains('plan-band-expense')).toBe(true)
    // heavier than the rows' hairlines, so the blocks read apart
    expect(expenseSection.classList.contains('border-t-2')).toBe(true)
    expect(screen.getByTestId('plan-totals').classList.contains('border-t-2')).toBe(true)
    for (const cls of ['mt-6', 'py-1']) {
      expect(expenseSection.classList.contains(cls)).toBe(false)
    }
    // the section opens on its line
    expect(expenseSection.firstElementChild).toBe(screen.getByTestId('plan-section-line-expense'))
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
    useBudgetPeriodStore.setState({ selectedDate: '2026-08-01' })
    const user = userEvent.setup()
    renderPage()
    await screen.findByTestId('plan-sheet')
    const grid = screen.getByTestId('plan-sheet')

    // window Jul/Aug/Sep: income-uncat's only nonzero actual is month 0 (May), out of
    // view -> hidden
    expect(document.querySelector('[data-row-id="uncategorized:3"]')).not.toBeInTheDocument()

    // keyboard flat rows must skip it too: ArrowDown from the last income loose row
    // (Freelance) lands on the expense side's first row (past the section and the
    // Essentials folder lines, which are no stops), never on the hidden uncategorized row
    await user.click(screen.getByTestId('plan-cell-cat-freelance:0'))
    grid.focus()
    await user.keyboard('{ArrowDown}')
    expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')

    // navigate back two months so 2026-05 enters the window -> row reappears
    act(() => useBudgetPeriodStore.getState().stepPeriod(-1))
    act(() => useBudgetPeriodStore.getState().stepPeriod(-1))
    await waitFor(() => expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-06-01'))
    expect(document.querySelector('[data-row-id="uncategorized:3"]')).toBeInTheDocument()
  })
})

it('scrolls the income/expenses/net trio and pins only the balance row', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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

  // the footer is exactly Income, Expenses, Transfers — uncategorized lives in the
  // bands as an element row, and Net is dropped (Balance below says the same thing)
  const totalRows = within(totals).getAllByRole('row')
  expect(totalRows).toHaveLength(3)
  expect(totalRows[0]).toHaveTextContent('Income')
  expect(totalRows[1]).toHaveTextContent('Expenses')
  expect(totalRows[2]).toHaveTextContent('Transfers')
  expect(within(totals).queryByText('Uncategorized')).not.toBeInTheDocument()
  expect(within(totals).queryByText('Net')).not.toBeInTheDocument()
  expect(within(balance).getByText('Balance')).toBeInTheDocument()

  // they are totals lines, not selectable element rows
  expect(totalRows[2].querySelector('[data-row-id]')).toBeNull()
})

it('transfers line: signed net per month, a tooltip with the in/out split, and a link only where money crossed', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')

  // window Jun/Jul/Aug; the fixture's June crossed 50 in / 150 out
  const junLink = screen.getByTestId('plan-totals-transfers-link-0')
  expect(junLink).toHaveTextContent('-100.00')
  // money leaving the budget is no problem in itself: only a negative Balance is red
  expect(junLink.closest('[data-col]')!.className).not.toContain('text-expense')
  expect(junLink).toHaveAttribute('title', 'In 50.00 · Out 150.00. Show transactions')

  // nothing crossed in July: plain text, no link
  expect(screen.queryByTestId('plan-totals-transfers-link-1')).not.toBeInTheDocument()
  expect(screen.getByTestId('plan-totals-transfers-1')).toHaveTextContent('0.00')

  // Balance carries the June transfers (math core, not hand-derived): it chains on
  // effectiveNet, which is where the Net line's value went
  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const ex = makePlanExchange(plan, [fixtureUsd, fixtureEur])
  const totals = planTotals(plan, ex)
  expect(totals[1].transfersNet).toBe('-100')
  // June is fetched index 1, and the window starts at June, so it is visible column 0
  const expected = balanceRow(plan, totals, ex)[1]
  expect(screen.getByTestId('plan-balance-0')).toHaveTextContent(
    moneyFormat(expected, fixtureUsd, { showCurrency: false, useNativePrecision: false }),
  )
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
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
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

  // Transfers is the only totals line that drills down: uncategorized is an element
  // row in the bands now, and Net is gone from the footer entirely
  expect(screen.queryByTestId('plan-totals-uncategorized-link-1')).not.toBeInTheDocument()
  expect(within(screen.getByTestId('plan-totals')).queryByText('Uncategorized')).not.toBeInTheDocument()
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

it('tints the selected month top to bottom, header and body alike, and marks a hovered cell alone', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')

  // hover marks the single cell, never the whole column: no cross-cell attribute
  const sheet = screen.getByTestId('plan-sheet')
  expect(sheet.parentElement).not.toHaveAttribute('data-hover-col')
  expect(document.querySelector('[data-hover-col]')).toBeNull()
  const hovered = screen.getByTestId('plan-cell-pe1:0')
  await user.hover(hovered)
  expect(hovered.className).toContain('outline-border')
  expect(screen.getByTestId('plan-cell-pe1:2').className).not.toContain('outline-border')

  // Jul is the second column: every line's cell in it carries the tint, no other does
  const tinted = [...sheet.querySelectorAll('[data-col]')].filter((c) => c.className.includes('bg-accent/40'))
  expect(tinted.length).toBeGreaterThan(5)
  tinted.forEach((c) => expect(c).toHaveAttribute('data-col', '1'))
  sheet.querySelectorAll('[data-col="1"]').forEach((c) => expect(c.className).toContain('bg-accent/40'))

  // the header marks the selected month by tint alone, never by weight
  const headers = within(screen.getByTestId('plan-month-header')).getAllByRole('columnheader')
  expect(headers.filter((h) => h.className.includes('bg-accent/40')).map((h) => h.getAttribute('data-month'))).toEqual(['2026-07-01'])
  headers.forEach((h) => expect(h.className).not.toMatch(/font-(bold|semibold)/))
})

it('the Balance line carries no bold: a full-colour label and regular figures, red only when negative', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')

  const line = screen.getByTestId('plan-total-balance')
  expect(line.outerHTML).not.toMatch(/font-(bold|semibold)/)
  expect(within(line).getByText('Balance').parentElement!.className).not.toContain('text-muted-foreground')
  // the fixture's balance stays positive, so no month reads red
  for (let col = 0; col < 3; col++) {
    expect(screen.getByTestId(`plan-balance-${col}`).closest('[data-col]')!.className).not.toContain('text-expense')
  }
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
  const parentRow = screen.getByTestId('plan-cell-pe1:0').closest('[role="row"]') as HTMLElement
  expect(childRow.className).toContain('hover:bg-accent/50')
  expect(parentRow.className).toContain('hover:bg-accent/50')

  // both lines share the same padding; the indent lives inside the fixed-width name
  // column, so the month cells line up under the parent's
  const lineClasses = (row: HTMLElement) => row.className.split(' ').filter((c) => /^p[lrx]-/.test(c))
  expect(lineClasses(childRow)).toEqual(lineClasses(parentRow))
  const childName = childRow.querySelector('[role="gridcell"]') as HTMLElement
  const parentName = parentRow.querySelector('[role="rowheader"]') as HTMLElement
  expect(childName.className).toContain('w-[var(--plan-name-col,14rem)]')
  expect(parentName.className).toContain('w-[var(--plan-name-col,14rem)]')
  expect(childName.className).toMatch(/\bpl-1[79]\b/)
})

it('a row ⋮ menu offers Change currency and a side-filtered Move to folder, with no mode to switch on', async () => {
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

  // the menu shows on hover: present in the DOM, hidden until the pointer is over the line
  const trigger = await screen.findByRole('button', { name: 'menu Living' })
  expect(trigger).toHaveClass('opacity-0', 'group-hover/line:opacity-100')
  await user.click(trigger)
  expect(await screen.findByRole('menuitem', { name: 'Change currency' })).toBeInTheDocument()
  await user.click(screen.getByRole('menuitem', { name: 'Move to folder…' }))

  // pe1/Living is expense-sided: expense + neutral folders only, never income ones
  const dialog = await screen.findByRole('dialog', { name: 'Move to folder…' })
  expect(within(dialog).getByRole('button', { name: 'Essentials' })).toBeInTheDocument()
  expect(within(dialog).getByRole('button', { name: 'No folder' })).toBeInTheDocument()
  expect(within(dialog).queryByRole('button', { name: 'Bonuses Folder' })).not.toBeInTheDocument()
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Move to folder…' })).not.toBeInTheDocument())

  // ie1/Salaries is income: income folders only
  await user.click(screen.getByRole('button', { name: 'menu Salaries' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Move to folder…' }))
  const incomeDialog = await screen.findByRole('dialog', { name: 'Move to folder…' })
  expect(within(incomeDialog).getByRole('button', { name: 'Bonuses Folder' })).toBeInTheDocument()
  expect(within(incomeDialog).queryByRole('button', { name: 'Essentials' })).not.toBeInTheDocument()
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


  await user.click(await screen.findByRole('button', { name: 'menu Living' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Move to folder…' }))

  const dialog = await screen.findByRole('dialog', { name: 'Move to folder…' })
  await user.click(within(dialog).getByRole('button', { name: 'Essentials' }))

  await waitFor(() => expect(body).toEqual({ budgetId: 'b1', id: 'pe1', folderId: 'bf1', afterId: null }))
  expect(screen.queryByRole('dialog', { name: 'Move to folder…' })).not.toBeInTheDocument()
})

it('a section ⋮ menu creates a folder on that section\'s side', async () => {
  let folderBody: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    http.post('*/api/v1/budget/create-folder', async ({ request }) => {
      folderBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { item: { id: 'nf1', name: 'Employment', position: 9 } } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')

  await user.click(within(screen.getByTestId('plan-section-line-income')).getByRole('button', { name: 'menu Income' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Create folder' }))
  const dialog = await screen.findByRole('dialog', { name: 'New folder' })
  await user.type(within(dialog).getByLabelText('Folder name'), 'Employment')
  await user.click(within(dialog).getByRole('button', { name: 'Create' }))

  // the side travels with the folder, so it stays an income folder while empty
  await waitFor(() => expect(folderBody).toMatchObject({ budgetId: 'b1', name: 'Employment', side: 'income' }))
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'New folder' })).not.toBeInTheDocument())
})

it('tablet: drag handles show only in Edit structure', async () => {
  usePlanHandlers()
  mockCompactViewport()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')

  expect(screen.queryByRole('button', { name: /^move / })).not.toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('button', { name: 'Edit structure' }))

  expect(screen.getAllByRole('button', { name: /^move / }).length).toBeGreaterThan(0)
})

it('rows show the hover drag grip, folders the folder grip', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  expect(screen.getAllByRole('button', { name: /^move / }).length).toBeGreaterThan(0)
  expect(screen.getAllByRole('button', { name: /^move folder / }).length).toBeGreaterThan(0)
})

it('a guest gets no drag grips: only who may configure the budget drags', async () => {
  const guestBudget = {
    ...fixtureWireBudget,
    meta: {
      ...fixtureWireBudget.meta,
      ownerUserId: 'u9',
      access: [
        { user: { id: 'u9', avatar: 'face:sky', name: 'Owner' }, role: 'owner', isAccepted: 1 },
        { user: { id: fixtureUser.id, avatar: 'face:emerald', name: 'Ada' }, role: 'guest', isAccepted: 1 },
      ],
    },
  }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: guestBudget } })),
    planHandler(),
  )
  renderPage()
  await screen.findByTestId('plan-sheet')
  await screen.findByRole('button', { name: 'menu Living' })
  expect(screen.queryByRole('button', { name: /^move / })).not.toBeInTheDocument()
})

it('scopes every drag handle to its own band, so no drag can cross the income/expense divider', async () => {
  // An element's side comes from its TYPE and a folder's from its members, and
  // order-folders persists position only — so a cross-band drop would either be
  // rejected by the server (CodeBudgetFolderSideMixed) or silently snap back on
  // reload. One DndContext per section is what makes the drop impossible at all.
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
  renderPage()
  await screen.findByTestId('plan-sheet')

  const income = screen.getByTestId('plan-section-income')
  const expense = screen.getByTestId('plan-section-expense')

  // income handles: the Salaries row and its Bonuses folder — and nothing expense-sided
  expect(within(income).getByRole('button', { name: 'move ie1' })).toBeInTheDocument()
  expect(within(income).getByRole('button', { name: 'move folder Bonuses Folder' })).toBeInTheDocument()
  expect(within(income).queryByRole('button', { name: 'move pe1' })).not.toBeInTheDocument()

  // expense handles live in the other band entirely
  expect(within(expense).getByRole('button', { name: 'move pe1' })).toBeInTheDocument()
  expect(within(expense).getByRole('button', { name: 'move folder Essentials' })).toBeInTheDocument()
  expect(within(expense).queryByRole('button', { name: 'move ie1' })).not.toBeInTheDocument()
})

it('an unfolded envelope gives each category its own grip, under the row grip', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')

  const grip = screen.getByRole('button', { name: 'move pe1' })
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  // the grip sits beside the row, outside its [data-row-id], at the row's top
  expect(pe1Row).not.toContainElement(grip)
  expect(grip.parentElement).toContainElement(pe1Row)
  expect(grip.className).toContain('top-3')

  expect(screen.queryByRole('button', { name: 'move cat-rent' })).not.toBeInTheDocument()
  await user.click(within(pe1Row).getByRole('button', { name: 'Expand' }))
  const childGrip = await screen.findByRole('button', { name: 'move cat-rent' })
  // the category list is the envelope's drop zone, and the row grip stays on its row
  expect(screen.getByTestId('envelope-drop-pe1')).toContainElement(childGrip)
  expect(screen.getByTestId('envelope-drop-pe1')).toContainElement(screen.getByTestId('plan-cell-cat-rent:0'))
  expect(grip.parentElement).toContainElement(childGrip)
})

it('a folded envelope takes a category on its row', async () => {
  usePlanHandlers()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  expect(within(pe1Row).getByTestId('envelope-head-drop-pe1')).toBeInTheDocument()
  // a category has nothing to take in
  expect(screen.queryByTestId('envelope-head-drop-cat-food')).not.toBeInTheDocument()
})

it('keeps the uncategorized totals line undraggable', async () => {
  usePlanHandlers()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await screen.findByRole('button', { name: 'move pe1' })

  // uncategorized is a synthetic bucket with no position of its own
  expect(screen.queryByRole('button', { name: 'move uncategorized' })).not.toBeInTheDocument()
})

it('tablet edit mode keeps roving keyboard navigation working through the drag wrapper', async () => {
  // the drag wrapper adds DOM depth around each row: selection and arrow-key
  // navigation must survive it (a tablet has no fill handle)
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  mockCompactViewport()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('button', { name: 'Edit structure' }))

  await user.click(screen.getByTestId('plan-cell-pe1:0'))
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')
  // edit mode owns the tap: no item sheet
  expect(screen.queryByTestId('element-sheet')).not.toBeInTheDocument()

  await user.keyboard('{ArrowRight}')
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('aria-selected', 'true')

  // the row is still reachable by its data-row-id anchor, unchanged by the wrapper:
  // the grip lives OUTSIDE it, which is what keeps every [data-row-id] query working
  const pe1Row = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  expect(within(pe1Row).queryByRole('button', { name: 'move pe1' })).not.toBeInTheDocument()
  expect(within(pe1Row.parentElement!).getByRole('button', { name: 'move pe1' })).toBeInTheDocument()
})

it('dragging an expense category onto an envelope moves it into the envelope', async () => {
  usePlanHandlers()
  const calls: unknown[] = []
  server.use(http.post('*/api/v1/budget/move-element', async ({ request }) => {
    calls.push(await request.json())
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  renderPage()
  await screen.findByTestId('plan-sheet')
  // contexts mount income, savings (when present), expense: the expense one is last
  const expense = capturedDragContexts[capturedDragContexts.length - 1]
  act(() => {
    expense.onDragStart({ active: { id: 'cat-food' } })
    expense.onDragEnd({ active: { id: 'cat-food' }, over: { id: 'benv:pe1' } })
  })
  // cat-food is a loose expense category, pe1 the expense envelope "Living"
  await waitFor(() => expect(calls).toEqual([expect.objectContaining({ id: 'cat-food', envelopeId: 'pe1' })]))
})

it('dragging an income category onto an income envelope moves it into the envelope', async () => {
  usePlanHandlers()
  const calls: unknown[] = []
  server.use(http.post('*/api/v1/budget/move-element', async ({ request }) => {
    calls.push(await request.json())
    await delay('infinite')
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  renderPage()
  await screen.findByTestId('plan-sheet')
  // income mounts first: the set of the latest render is [income, expense]
  const income = () => capturedDragContexts[capturedDragContexts.length - 2]
  act(() => income().onDragStart({ active: { id: 'cat-freelance' } }))
  act(() => income().onDragEnd({ active: { id: 'cat-freelance' }, over: { id: 'benvh:ie1' } }))
  await waitFor(() => expect(calls).toEqual([expect.objectContaining({ id: 'cat-freelance', envelopeId: 'ie1' })]))
  // it leaves its old place at once instead of waiting for the refetch
  await waitFor(() => expect(document.querySelector('[data-row-id="cat-freelance:3"]')).toBeNull())
})

it('an envelope never goes into another envelope', async () => {
  usePlanHandlers()
  const calls: unknown[] = []
  server.use(http.post('*/api/v1/budget/move-element', async ({ request }) => {
    calls.push(await request.json())
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  renderPage()
  await screen.findByTestId('plan-sheet')
  const expense = () => capturedDragContexts[capturedDragContexts.length - 1]
  act(() => expense().onDragStart({ active: { id: 'pe1' } }))
  act(() => expense().onDragEnd({ active: { id: 'pe1' }, over: { id: 'benv:env-eur' } }))
  // a category of the envelope it is already in stays put too
  act(() => expense().onDragStart({ active: { id: 'cat-rent' } }))
  act(() => expense().onDragEnd({ active: { id: 'cat-rent' }, over: { id: 'benv:pe1' } }))
  await new Promise((r) => setTimeout(r, 50))
  expect(calls).toEqual([])
})

it('a row dropped on a folded folder joins it last, as in the Budget view', async () => {
  usePlanHandlers()
  const calls: unknown[] = []
  server.use(http.post('*/api/v1/budget/move-element', async ({ request }) => {
    calls.push(await request.json())
    await delay('infinite')
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Essentials' }))
  expect(screen.queryByTestId('plan-cell-pe1:0')).not.toBeInTheDocument()
  const expense = () => capturedDragContexts[capturedDragContexts.length - 1]
  act(() => expense().onDragStart({ active: { id: 'cat-food' } }))
  act(() => expense().onDragEnd({ active: { id: 'cat-food' }, over: { id: 'bfolder:bf1' } }))
  // after Living, the folded folder's last member
  await waitFor(() => expect(calls).toEqual([{ budgetId: 'b1', id: 'cat-food', folderId: 'bf1', afterId: 'pe1' }]))
})

it('the expense section never takes an income category into an expense envelope', async () => {
  usePlanHandlers()
  const calls: unknown[] = []
  server.use(http.post('*/api/v1/budget/move-element', async ({ request }) => {
    calls.push(await request.json())
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  renderPage()
  await screen.findByTestId('plan-sheet')
  const expense = () => capturedDragContexts[capturedDragContexts.length - 1]
  act(() => expense().onDragStart({ active: { id: 'cat-freelance' } }))
  act(() => expense().onDragEnd({ active: { id: 'cat-freelance' }, over: { id: 'benv:pe1' } }))
  await new Promise((r) => setTimeout(r, 50))
  expect(calls).toEqual([])
})

it('a category dragged out of its envelope lands as a row where it is dropped', async () => {
  usePlanHandlers()
  const calls: unknown[] = []
  server.use(http.post('*/api/v1/budget/move-element', async ({ request }) => {
    calls.push(await request.json())
    await delay('infinite')
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(within(document.querySelector('[data-row-id="pe1:0"]') as HTMLElement).getByRole('button', { name: 'Expand' }))
  await screen.findByTestId('plan-cell-cat-rent:0')
  const expense = () => capturedDragContexts[capturedDragContexts.length - 1]
  act(() => expense().onDragStart({ active: { id: 'cat-rent' } }))
  // dropped on Food, the first folder-less row: it lands first among them
  act(() => expense().onDragEnd({ active: { id: 'cat-rent' }, over: { id: 'cat-food' } }))
  await waitFor(() => expect(calls).toEqual([{ budgetId: 'b1', id: 'cat-rent', folderId: null, afterId: null }]))
  // hidden from the envelope until the refetch shows it in its new place
  await waitFor(() => expect(screen.queryByTestId('plan-cell-cat-rent:0')).not.toBeInTheDocument())
})

it('while a row is dragged the insertion line marks where it lands', async () => {
  usePlanHandlers()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const expense = () => capturedDragContexts[capturedDragContexts.length - 1]
  act(() => expense().onDragStart({ active: { id: 'cat-food' } }))
  // Food dragged down over Euro Stash lands right after it
  act(() => expense().onDragOver({ active: { id: 'cat-food' }, over: { id: 'env-eur' } }))
  const line = await screen.findByTestId('drop-line-row')
  expect(line.parentElement).toContainElement(document.querySelector('[data-row-id="env-eur:0"]') as HTMLElement)
  expect(line.parentElement).not.toContainElement(document.querySelector('[data-row-id="tag1:2"]') as HTMLElement)
  act(() => expense().onDragCancel())
  expect(screen.queryByTestId('drop-line-row')).not.toBeInTheDocument()
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
  renderPage()
  await screen.findByTestId('plan-sheet')
  await screen.findByRole('button', { name: 'move cat-food' })

  // the expense band's LOOSE rows (cat-food, tag1, env-eur) are the reorderable set
  const looseOrder = () =>
    [...document.querySelectorAll('[data-testid="plan-section-expense"] [data-row-id]')]
      .map((r) => r.getAttribute('data-row-id'))
      .filter((id) => id === 'cat-food:1' || id === 'tag1:2' || id === 'env-eur:0')
  expect(looseOrder()[0]).toBe('cat-food:1')

  // drop the first loose row onto the last one
  const expense = () => capturedDragContexts[capturedDragContexts.length - 1]
  act(() => expense().onDragStart({ active: { id: 'cat-food' } }))
  act(() => expense().onDragEnd({ active: { id: 'cat-food' }, over: { id: 'env-eur' } }))

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

it('names a foreign-currency row by its code next to the name; the row ⋮ menu sits in the name column', async () => {
  usePlanHandlers()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')

  // no trailing currency track: a budget-currency row shows none, a EUR row its code
  const nameCell = (testId: string) => screen.getByTestId(testId).closest('[role="row"]')!.querySelector('[role="rowheader"]') as HTMLElement
  expect(within(nameCell('plan-cell-pe1:0')).queryByTestId('currency-tag')).not.toBeInTheDocument()
  expect(within(nameCell('plan-cell-env-eur:0')).getByTestId('currency-tag')).toHaveTextContent('EUR')
  expect(within(grid).queryByText('$')).not.toBeInTheDocument()

  const menu = await screen.findByRole('button', { name: 'menu Living' })
  expect(nameCell('plan-cell-pe1:0')).toContainElement(menu)
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
  renderPage()
  await screen.findByTestId('plan-sheet')

  // pe1/Living sits inside the "Essentials" folder and is visible at rest
  expect(screen.getByTestId('plan-cell-pe1:0')).toBeInTheDocument()
  const folderHeader = screen.getByRole('button', { name: 'Essentials' })
  expect(folderHeader).toHaveAttribute('aria-expanded', 'true')

  const expense = () => capturedDragContexts[capturedDragContexts.length - 1]
  act(() => expense().onDragStart({ active: { id: 'bf1' } }))

  // rows hide so the headers reorder as compact blocks, but the folder's own fold
  // state is untouched — the chevron must not claim the user collapsed it
  await waitFor(() => expect(screen.queryByTestId('plan-cell-pe1:0')).not.toBeInTheDocument())
  expect(screen.getByRole('button', { name: 'Essentials' })).toHaveAttribute('aria-expanded', 'true')

  // the plan fixture ships a single folder, so there is nothing to reorder against —
  // this covers the collapse lifecycle, not the reorder itself (which the neutral
  // band's test covers)
  act(() => expense().onDragEnd({ active: { id: 'bf1' }, over: null }))

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
  renderPage()
  await screen.findByTestId('plan-sheet')
  await screen.findByRole('button', { name: 'move cat-food' })

  const looseOrder = () =>
    [...document.querySelectorAll('[data-testid="plan-section-expense"] [data-row-id]')]
      .map((r) => r.getAttribute('data-row-id'))
      .filter((id) => id === 'cat-food:1' || id === 'tag1:2' || id === 'env-eur:0')
  expect(looseOrder()[0]).toBe('cat-food:1')

  const expense = () => capturedDragContexts[capturedDragContexts.length - 1]
  act(() => expense().onDragStart({ active: { id: 'cat-food' } }))
  act(() => expense().onDragEnd({ active: { id: 'cat-food' }, over: { id: 'env-eur' } }))

  await waitFor(() => expect(looseOrder()[0]).not.toBe('cat-food:1'))

  // the mutation has settled and its refetch is in flight; the dropped order must hold
  await waitFor(() => expect(planRequests).toBeGreaterThan(1))
  const settled = looseOrder()
  expect(settled[0]).not.toBe('cat-food:1')
  expect(settled).toContain('cat-food:1')
})

it('while a row is dragged an empty No folder shows as the way out of a folder', async () => {
  // pe1/Living is the sole member of the "Essentials" folder in the base fixture;
  // move every loose expense element into the folder too, so the No folder group is
  // empty and would not render at rest
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
      await delay('infinite')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  renderPage()
  await screen.findByTestId('plan-sheet')
  await screen.findByRole('button', { name: 'move cat-food' })
  const expenseSection = screen.getByTestId('plan-section-expense')
  expect(within(expenseSection).queryByTestId('plan-folder-__no_folder__')).not.toBeInTheDocument()

  const expense = () => capturedDragContexts[capturedDragContexts.length - 1]
  act(() => expense().onDragStart({ active: { id: 'cat-food' } }))
  // the drop target for taking it out of its folder: an empty No folder group
  expect(within(expenseSection).getByTestId('plan-folder-__no_folder__')).toBeInTheDocument()
  act(() => expense().onDragEnd({ active: { id: 'cat-food' }, over: { id: 'bfolder:null' } }))

  await waitFor(() => expect(body).toBeDefined())
  expect(body).toMatchObject({ id: 'cat-food', folderId: null })
  // the row shows under No folder while the call is in flight
  await waitFor(() =>
    expect(within(screen.getByTestId('plan-folder-__no_folder__')).getByTestId('plan-cell-cat-food:0')).toBeInTheDocument(),
  )
})

it('row ⋮ menus: an envelope offers Edit and Delete, a category and a tag Edit and their own classification actions', async () => {
  // the budget view's wire response strips income envelopes entirely, so the plan
  // sheet is the ONLY place ie1/Salaries can be managed at all
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const items = () => screen.getAllByRole('menuitem').map((i) => i.textContent)

  // ie1/Salaries is an income envelope (type 4)
  await user.click(await screen.findByRole('button', { name: 'menu Salaries' }))
  await screen.findByRole('menuitem', { name: 'Edit' })
  expect(items()).toEqual(['Edit', 'Change currency', 'Move to folder…', 'Delete'])
  await user.keyboard('{Escape}')

  // pe1/Living is an expense envelope (type 0) — the same items
  await user.click(await screen.findByRole('button', { name: 'menu Living' }))
  await screen.findByRole('menuitem', { name: 'Edit' })
  expect(items()).toEqual(['Edit', 'Change currency', 'Move to folder…', 'Delete'])
  await user.keyboard('{Escape}')

  // cat-food/Food is a category (type 1): the envelope's Delete is not offered, its
  // own classification actions follow instead
  await user.click(await screen.findByRole('button', { name: 'menu Food' }))
  await screen.findByRole('menuitem', { name: 'Edit' })
  expect(items().slice(0, 3)).toEqual(['Edit', 'Change currency', 'Move to folder…'])
  expect(items()).toContain('Archive')
  await user.keyboard('{Escape}')

  // tag1/vacation is a tag (type 2)
  await user.click(await screen.findByRole('button', { name: 'menu vacation' }))
  await screen.findByRole('menuitem', { name: 'Edit' })
  expect(items().slice(0, 3)).toEqual(['Edit', 'Change currency', 'Move to folder…'])
  expect(items()).toContain('Archive')
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

  await user.click(await screen.findByRole('button', { name: 'menu Salaries' }))
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

  await user.click(await screen.findByRole('button', { name: 'menu Salaries' }))
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
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const createFolder = async () => {
    await user.click(within(screen.getByTestId('plan-section-line-expense')).getByRole('button', { name: 'menu Expenses' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Create folder' }))
    return screen.findByRole('dialog', { name: 'New folder' })
  }

  const dialog = await createFolder()
  await user.type(within(dialog).getByLabelText('Folder name'), 'Employment')
  await user.click(within(dialog).getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'New folder' })).not.toBeInTheDocument())

  // a surviving name would let a second submit duplicate the folder
  const reopened = await createFolder()
  expect(within(reopened).getByLabelText('Folder name')).toHaveValue('')
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

  await user.click(within(screen.getByTestId('plan-section-line-income')).getByRole('button', { name: 'menu Income' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Create folder' }))
  const dialog = await screen.findByRole('dialog', { name: 'New folder' })
  await user.type(within(dialog).getByLabelText('Folder name'), 'Ab')
  await user.click(within(dialog).getByRole('button', { name: 'Create' }))

  expect(await within(dialog).findByText('Folder name must be 3-64 characters')).toBeInTheDocument()
  expect(called).toBe(false)
})

it('Plan rows, folders and sections offer the Budget view ⋮ menus on hover', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getAllByRole('button', { name: /^menu / })[0])
  const items = screen.getAllByRole('menuitem').map((i) => i.textContent)
  expect(items.some((x) => /create folder/i.test(x ?? ''))).toBe(true)
})

it('desktop Plan view: Configure opens Budget settings at once, no Edit structure', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: /configure/i }))
  expect(screen.queryByText(/edit structure/i)).not.toBeInTheDocument()
  expect(await screen.findByRole('dialog')).toBeInTheDocument()
})

it('a category inside an envelope offers its own ⋮ menu, as in the Budget view', async () => {
  usePlanHandlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const living = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  await user.click(within(living).getByTitle('Expand'))
  const child = (await waitFor(() => document.querySelector('[data-row-id="cat-rent:1"]'))) as HTMLElement
  await user.click(within(child).getByRole('button', { name: 'menu Rent' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
  // its classification actions follow; Rent is not among the caller's own categories here
  expect(screen.getAllByRole('menuitem').map((i) => i.textContent)).toContain('Archive (no access)')
})

it('tablet: no ⋮ menus until Edit structure is on, then on every line', async () => {
  usePlanHandlers()
  mockCompactViewport()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  expect(screen.queryByRole('button', { name: /^menu / })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('button', { name: 'Edit structure' }))
  const living = await screen.findByRole('button', { name: 'menu Living' })
  expect(living).not.toHaveClass('opacity-0')
  expect(within(screen.getByTestId('plan-section-line-income')).getByRole('button', { name: 'menu Income' })).toBeInTheDocument()
})

it('the name column is resized from its edge in the month row, remembered, and reset by double-click', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', planNameWidth: null })
  const user = userEvent.setup()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')
  const edge = within(screen.getByTestId('plan-month-header')).getByRole('separator', { name: /name column/i })
  expect(edge).toHaveAttribute('aria-valuenow', '224')
  expect(grid.style.getPropertyValue('--plan-name-col')).toBe('224px')

  edge.focus()
  await user.keyboard('{ArrowRight}{ArrowRight}')
  expect(edge).toHaveAttribute('aria-valuenow', '256')
  expect(grid.style.getPropertyValue('--plan-name-col')).toBe('256px')
  expect(useBudgetPeriodStore.getState().planNameWidth).toBe(256)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_RESIZE_NAME_COLUMN)
  // the arrows resize the column; they never move the grid's selection
  expect(document.querySelector('[role="gridcell"][aria-selected="true"]')).toBeNull()

  // a pointer drag: 60px to the left
  fireEvent.pointerDown(edge, { clientX: 500, pointerId: 1 })
  fireEvent.pointerMove(edge, { clientX: 440, pointerId: 1 })
  expect(grid.style.getPropertyValue('--plan-name-col')).toBe('196px')
  fireEvent.pointerUp(edge, { clientX: 440, pointerId: 1 })
  expect(useBudgetPeriodStore.getState().planNameWidth).toBe(196)

  // never below the minimum
  fireEvent.pointerDown(edge, { clientX: 500, pointerId: 1 })
  fireEvent.pointerMove(edge, { clientX: 0, pointerId: 1 })
  fireEvent.pointerUp(edge, { clientX: 0, pointerId: 1 })
  expect(useBudgetPeriodStore.getState().planNameWidth).toBe(160)

  await user.dblClick(edge)
  expect(useBudgetPeriodStore.getState().planNameWidth).toBeNull()
  expect(edge).toHaveAttribute('aria-valuenow', '224')
})

it('selecting a cell highlights its whole row and its whole month column', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const sheet = screen.getByTestId('plan-sheet')
  const colCells = (col: number) => Array.from(sheet.querySelectorAll(`[data-col="${col}"]`))
  // nothing selected: no crosshair
  expect(sheet.querySelector('[data-crosshair]')).toBeNull()

  await user.click(screen.getByTestId('plan-cell-pe1:1'))
  // the row: Living's line
  const row = document.querySelector('[data-row-id="pe1:0"] [role="row"]') ?? document.querySelector('[data-row-id="pe1:0"]')!
  expect(row.closest('[data-crosshair="row"]')).not.toBeNull()
  // the column: every line's July cell — the month header, section and folder lines, other rows, totals
  const july = colCells(1)
  expect(july.length).toBeGreaterThan(5)
  for (const cell of july) {
    expect(cell).toHaveAttribute('data-crosshair', 'col')
  }
  expect(within(screen.getByTestId('plan-month-header')).getAllByRole('columnheader')[1]).toHaveAttribute('data-crosshair', 'col')
  // other columns and rows stay plain
  for (const cell of colCells(0)) {
    expect(cell).not.toHaveAttribute('data-crosshair')
  }
  expect(document.querySelector('[data-row-id="cat-food:1"]')!.querySelector('[data-crosshair="row"]')).toBeNull()

  // a name is no stop: clicking Food's highlights nothing new, the crosshair stays
  // on Living's July
  await user.click(within(document.querySelector('[data-row-id="cat-food:1"]') as HTMLElement).getByTitle('Food'))
  expect(document.querySelector('[data-row-id="cat-food:1"]')!.querySelector('[data-crosshair="row"]')).toBeNull()
  expect(row.closest('[data-crosshair="row"]')).not.toBeNull()
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('data-crosshair', 'col')
})

it('one cell of an open line shows its own sum from its Σ, and hides it on a second click', async () => {
  usePlanHandlers()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01' })
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const line = () => screen.getByTestId('plan-section-line-expense')
  expect(within(line()).queryAllByTestId(/^plan-sum-/)).toHaveLength(0)
  await user.click(within(line()).getByRole('button', { name: /show the july sum for expenses/i }))
  // July alone
  expect(within(line()).getAllByTestId(/^plan-sum-/).map((c) => c.getAttribute('data-testid'))).toEqual(['plan-sum-1'])
  expect(useBudgetPeriodStore.getState().planSumsShown).toEqual({ 'expense@2026-07-01': true })
  expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_TOGGLE_SUMS)
  // the shown sum is the button that hides it again
  await user.click(within(line()).getByRole('button', { name: /hide the july sum for expenses/i }))
  expect(within(line()).queryAllByTestId(/^plan-sum-/)).toHaveLength(0)
  // the line's own Σ still shows every month
  await user.click(within(line()).getByRole('button', { name: /show sums for expenses/i }))
  expect(within(line()).getAllByTestId(/^plan-sum-/)).toHaveLength(3)
})
