import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { delay, http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureEur, fixtureUsd, fixtureUser, fixtureWireBudget, fixtureWirePlan, planHandler } from '@/test/fixtures'
import type { BudgetPlanDto } from '@/api/dto/budget'
import { BudgetPage } from './BudgetPage'
import { useBudgetPeriodStore } from './budgetStore'
import { toast } from 'sonner'
import { balanceRow, everydayBalanceRow, makePlanExchange, planTotals, savingsAsPlanElement, savingsBalanceRow } from './planMath'
import { moneyFormat } from '@/lib/money'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})
vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

// Same dnd-kit stand-in as PlanSheet.test.tsx: every band's onDragEnd is captured and
// fired directly. The savings band renders after the expense band, so while savings
// rows exist its handler is the LAST captured entry.
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

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/plan', element: <BudgetPage key="plan" mode="plan" /> }], { initialEntries: ['/plan'] })
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

const cells = (...pairs: [string, string][]) => pairs.map(([actual, planned]) => ({ actual, planned }))

// Wire order is deliberately NOT position order: the section must sort by position.
const savingsS2 = {
  id: 'acc-s2', type: 5, name: 'Holiday fund', icon: 'beach_access', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 1,
  cells: cells(['20', '50'], ['30', '50'], ['40', '50'], ['0', '']),
}
const savingsS1 = {
  id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0,
  cells: cells(['100', '100'], ['100', '150'], ['50', '200'], ['0', '200']),
}
const savingsDeleted = {
  id: 'acc-s3', type: 5, name: 'Closed deposit', icon: 'savings', currencyId: 'cur-eur', ownerUserId: 'u1', isArchived: 1, position: 2,
  cells: cells(['10', ''], ['0', '40'], ['0', ''], ['0', '']),
}
const archivedElement = {
  id: 'arch-1', type: 1, name: 'Old hobby', icon: 'delete', currencyId: 'cur-usd', isArchived: 1, folderId: null, position: 9, ownerUserId: 'u1',
  cells: cells(['0', ''], ['0', ''], ['0', '30'], ['0', '']),
  children: [],
}

const savingsPlan = {
  ...fixtureWirePlan,
  structure: {
    ...fixtureWirePlan.structure,
    elements: [...fixtureWirePlan.structure.elements, archivedElement],
    savings: [savingsS2, savingsS1, savingsDeleted],
  },
  savingsOpeningBalances: [{ currencyId: 'cur-usd', amount: '1000' }],
  savingsFlows: [
    { month: '2026-05-01', currencyId: 'cur-usd', amount: '125' },
    { month: '2026-06-01', currencyId: 'cur-usd', amount: '140' },
    { month: '2026-07-01', currencyId: 'cur-eur', amount: '20' },
  ],
}

const savingsComment = {
  id: 'cm-s1',
  elementId: 'acc-s1',
  period: '2026-08-01',
  comment: 'Top up after the bonus',
  author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' },
  createdAt: '2026-08-17 09:00:00',
  updatedAt: '2026-08-17 09:00:00',
}

function useHandlers(plan: unknown = savingsPlan, extra: Parameters<typeof server.use> = []) {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(plan),
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [savingsComment], truncated: false } }),
    ),
    ...extra,
  )
}

async function enterEditMode(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
}

const rowIds = (section: HTMLElement) => [...section.querySelectorAll('[data-row-id]')].map((r) => r.getAttribute('data-row-id'))

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 7, 15, 12, 0, 0))
  vi.clearAllMocks()
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
  capturedDragEnds = []
  useBudgetPeriodStore.setState({
    selectedDate: '2026-07-01',
    unfoldedElements: {},
    foldBudgetId: null,
    planFirstMonth: '2026-06-01',
    planFolds: {},
    planHideEmpty: false,
  })
})

afterEach(() => {
  vi.useRealTimers()
})

it('savingsAsPlanElement adapts a savings row to the element row shape', () => {
  const el = savingsAsPlanElement(savingsS1 as never)
  expect(el).toMatchObject({ id: 'acc-s1', type: 5, folderId: null, children: [], ownerUserId: 'u1', position: 0 })
  expect(el.cells).toBe(savingsS1.cells)
})

it('renders the Savings section after Expenses and before Archived, rows in position order', async () => {
  useHandlers()
  renderPage()
  const section = await screen.findByTestId('plan-section-savings')
  const expense = screen.getByTestId('plan-section-expense')
  const archived = screen.getByTestId('plan-section-archived')
  expect(expense.compareDocumentPosition(section) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(section.compareDocumentPosition(archived) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(within(section).getByRole('button', { name: 'Savings' })).toBeInTheDocument()
  // live rows by position, then the deleted account's row — which stays in this
  // section rather than moving to the Archived band
  expect(rowIds(section)).toEqual(['acc-s1:5', 'acc-s2:5', 'acc-s3:5'])
  expect(within(archived).queryByTitle('Closed deposit')).not.toBeInTheDocument()
})

it('folding the Savings header hides its rows and persists the fold', async () => {
  useHandlers()
  const user = userEvent.setup()
  renderPage()
  const section = await screen.findByTestId('plan-section-savings')
  await user.click(within(section).getByRole('button', { name: 'Savings' }))
  expect(within(screen.getByTestId('plan-section-savings')).queryByTitle('Rainy day')).not.toBeInTheDocument()
  expect(useBudgetPeriodStore.getState().planFolds.savings).toBe(true)
})

it('ArrowDown walks from the last expense row into the savings rows, then on into Archived', async () => {
  useHandlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-section-savings')
  const grid = screen.getByTestId('plan-sheet')
  const cellOf = (rowId: string, col: number) =>
    (document.querySelector(`[data-row-id="${rowId}"]`) as HTMLElement).querySelector(`[data-col="${col}"][role="gridcell"]`) as HTMLElement

  // the expense band's last row is its uncategorized line (July carries spend)
  await user.click(cellOf('uncategorized:1', 0))
  grid.focus()
  await user.keyboard('{ArrowDown}')
  expect(cellOf('acc-s1:5', 0)).toHaveAttribute('aria-selected', 'true')
  await user.keyboard('{ArrowDown}{ArrowDown}')
  expect(cellOf('acc-s3:5', 0)).toHaveAttribute('aria-selected', 'true')
  await user.keyboard('{ArrowDown}')
  expect(cellOf('arch-1:1', 0)).toHaveAttribute('aria-selected', 'true')

  // folded, the savings rows drop out of the keyboard order too
  useBudgetPeriodStore.setState({ planFolds: { savings: true } })
  await user.click(cellOf('uncategorized:1', 0))
  grid.focus()
  await user.keyboard('{ArrowDown}')
  expect(cellOf('arch-1:1', 0)).toHaveAttribute('aria-selected', 'true')
})

it('editing a savings planned cell sends set-limit with the account id and patches structure.savings optimistically', async () => {
  let body: unknown
  useHandlers(savingsPlan, [
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      body = await request.json()
      await delay('infinite')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  ])
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-section-savings')

  // Jun/Jul/Aug window: column 2 is August
  const cell = screen.getByTestId('plan-cell-acc-s1:2')
  await user.click(within(cell).getByRole('button', { name: 'limit Rainy day' }))
  const input = await screen.findByLabelText('Budget')
  await user.clear(input)
  await user.type(input, '350')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(body).toEqual({ budgetId: 'b1', elementId: 'acc-s1', period: '2026-08-01', amount: '350' }))
  expect(within(cell).getByTestId('cell-planned')).toHaveTextContent('350')
})

it('a keyboard fill on a savings row writes each month and patches structure.savings optimistically', async () => {
  const bodies: unknown[] = []
  useHandlers(savingsPlan, [
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      bodies.push(await request.json())
      await delay('infinite')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  ])
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-section-savings')
  const grid = screen.getByTestId('plan-sheet')

  // Holiday fund: June planned 50 -> fill into Jul and Aug
  await user.click(screen.getByTestId('plan-cell-acc-s2:0'))
  fireEvent.keyDown(grid, { key: 'ArrowRight', shiftKey: true })
  fireEvent.keyDown(grid, { key: 'ArrowRight', shiftKey: true })
  fireEvent.keyUp(grid, { key: 'Shift' })

  await waitFor(() => expect(bodies).toHaveLength(2))
  expect(bodies).toEqual(
    expect.arrayContaining([
      { budgetId: 'b1', elementId: 'acc-s2', period: '2026-07-01', amount: '50' },
      { budgetId: 'b1', elementId: 'acc-s2', period: '2026-08-01', amount: '50' },
    ]),
  )
  // August was unset ('—' is never shown for an editable cell, it reads 0) and is now 50
  expect(within(screen.getByTestId('plan-cell-acc-s2:2')).getByTestId('cell-planned')).toHaveTextContent('50')
})

it('the savings row menu offers no "Move to folder…"', async () => {
  useHandlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-section-savings')
  await enterEditMode(user)
  await user.click(await screen.findByRole('button', { name: 'element actions Rainy day' }))
  expect(await screen.findByRole('menuitem', { name: 'Change currency' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitem', { name: 'Move to folder…' })).not.toBeInTheDocument()
})

it('Enter on a savings name cell opens no category or tag dialog', async () => {
  useHandlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-section-savings')
  const grid = screen.getByTestId('plan-sheet')
  await user.click(screen.getByTestId('plan-cell-acc-s1:0'))
  grid.focus()
  await user.keyboard('{ArrowLeft}{Enter}')
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(toast.error).not.toHaveBeenCalled()
})

it('edit mode: savings rows reorder within their own band only, with folderId null', async () => {
  const bodies: unknown[] = []
  useHandlers(savingsPlan, [
    http.post('*/api/v1/budget/move-element', async ({ request }) => {
      bodies.push(await request.json())
      await delay('infinite')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  ])
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-section-savings')
  await enterEditMode(user)
  const section = await screen.findByTestId('plan-section-savings')
  expect(await within(section).findByRole('button', { name: 'move Rainy day' })).toBeInTheDocument()
  expect(within(section).getByRole('button', { name: 'move Holiday fund' })).toBeInTheDocument()
  // the deleted account's row is read-only history: no grip
  expect(within(section).queryByRole('button', { name: 'move Closed deposit' })).not.toBeInTheDocument()
  // no folder or loose-area droppable inside the savings band
  expect(within(section).queryByTestId('plan-loose-drop')).not.toBeInTheDocument()
  expect(section.querySelector('[data-testid^="plan-folder-"]')).toBeNull()

  const savingsDragEnd = capturedDragEnds[capturedDragEnds.length - 1]
  // a folder target is not expressible from this band: nothing is sent
  savingsDragEnd({ active: { id: 'acc-s2' }, over: { id: 'pfolder:bf1' } })
  savingsDragEnd({ active: { id: 'acc-s2' }, over: { id: 'bfolder:null' } })
  savingsDragEnd({ active: { id: 'acc-s2' }, over: { id: 'acc-s1' } })

  await waitFor(() => expect(bodies).toHaveLength(1))
  expect(bodies[0]).toEqual({ budgetId: 'b1', id: 'acc-s2', folderId: null, afterId: null })
  // the dropped order holds locally while the call is in flight
  await waitFor(() => expect(rowIds(screen.getByTestId('plan-section-savings'))).toEqual(['acc-s2:5', 'acc-s1:5', 'acc-s3:5']))
})

it('totals gain a Savings line below Transfers; the balance splits into Balance and Savings balance', async () => {
  useHandlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-section-savings')

  const plan = savingsPlan as unknown as BudgetPlanDto
  const ex = makePlanExchange(plan, [fixtureUsd, fixtureEur])
  const totals = planTotals(plan, ex)
  const combined = balanceRow(plan, totals, ex)
  const savings = savingsBalanceRow(plan, totals, ex)
  const everyday = everydayBalanceRow(combined, savings)
  const fmt = (v: string) => moneyFormat(v, fixtureUsd, { showCurrency: false, useNativePrecision: false })

  const totalsBlock = screen.getByTestId('plan-totals')
  const labels = within(totalsBlock).getAllByRole('row').map((r) => r.firstElementChild?.textContent)
  expect(labels).toEqual(['Income', 'Expenses', 'Transfers', 'Savings'])
  // window Jun/Jul/Aug = plan months 1..3
  for (let col = 0; col < 3; col++) {
    expect(screen.getByTestId(`plan-totals-savings-${col}`)).toHaveTextContent(fmt(totals[col + 1].effectiveSavings))
    expect(screen.getByTestId(`plan-balance-${col}`)).toHaveTextContent(fmt(everyday[col + 1]))
    expect(screen.getByTestId(`plan-savings-balance-${col}`)).toHaveTextContent(fmt(savings[col + 1]))
  }

  const balanceArea = screen.getByTestId('plan-balance-row')
  expect(within(balanceArea).getByText('Balance')).toBeInTheDocument()
  const tooltip = 'Includes interest and other activity on savings accounts, which is not counted as saved'
  expect(within(balanceArea).getByText('Savings balance').closest('[title]')).toHaveAttribute('title', tooltip)
  // the same note is reachable by tap (no hover on touch screens)
  await user.click(within(balanceArea).getByRole('button', { name: 'About' }))
  expect(await screen.findByTestId('plan-savings-balance-info')).toHaveTextContent(tooltip)
})

it('a savings cell with comments shows the marker, and Shift+Enter opens its thread', async () => {
  useHandlers()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage()
  await screen.findByTestId('plan-section-savings')

  const cell = screen.getByTestId('plan-cell-acc-s1:2')
  expect(await within(cell).findByTestId('comment-marker')).toHaveAccessibleName('1 comment')
  expect(within(screen.getByTestId('plan-cell-acc-s1:1')).queryByTestId('comment-marker')).toBeNull()

  await user.click(cell)
  screen.getByTestId('plan-sheet').focus()
  await user.keyboard('{Shift>}{Enter}{/Shift}')
  expect(await screen.findByText('Top up after the bonus')).toBeInTheDocument()
})

it('a deleted-account savings row is read-only: no cell editor, no grip', async () => {
  useHandlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('plan-section-savings')
  const deletedRow = document.querySelector('[data-row-id="acc-s3:5"]') as HTMLElement
  expect(within(deletedRow).queryByRole('button', { name: /^limit / })).not.toBeInTheDocument()
  expect(within(screen.getByTestId('plan-cell-acc-s3:0')).getByTestId('cell-planned')).toHaveTextContent('40')
  expect(within(screen.getByTestId('plan-cell-acc-s1:0')).getByRole('button', { name: 'limit Rainy day' })).toBeInTheDocument()
  await enterEditMode(user)
  await screen.findByRole('button', { name: 'move Rainy day' })
  expect(screen.queryByRole('button', { name: 'move Closed deposit' })).not.toBeInTheDocument()
})

it('without savings rows: no Savings section, totals line or balance row, and Balance is the combined balance', async () => {
  useHandlers(fixtureWirePlan)
  renderPage()
  await screen.findByTestId('plan-sheet')
  expect(screen.queryByTestId('plan-section-savings')).not.toBeInTheDocument()
  const labels = within(screen.getByTestId('plan-totals')).getAllByRole('row').map((r) => r.firstElementChild?.textContent)
  expect(labels).toEqual(['Income', 'Expenses', 'Transfers'])
  expect(screen.queryByTestId('plan-savings-balance-0')).not.toBeInTheDocument()
  expect(within(screen.getByTestId('plan-balance-row')).queryByText('Savings balance')).not.toBeInTheDocument()

  const plan = fixtureWirePlan as unknown as BudgetPlanDto
  const ex = makePlanExchange(plan, [fixtureUsd, fixtureEur])
  const combined = balanceRow(plan, planTotals(plan, ex), ex)
  for (let col = 0; col < 3; col++) {
    expect(screen.getByTestId(`plan-balance-${col}`)).toHaveTextContent(
      moneyFormat(combined[col + 1], fixtureUsd, { showCurrency: false, useNativePrecision: false }),
    )
  }
})
