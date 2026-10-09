import type { ReactNode } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { delay, http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget, fixtureWirePlan, planHandler } from '@/test/fixtures'
import { BudgetPage } from './BudgetPage'
import { useBudgetPeriodStore } from './budgetStore'
import { METRICS, trackEvent } from '@/lib/metrics'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})
vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))
vi.mock('@dnd-kit/core', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@dnd-kit/core')>()
  return {
    ...actual,
    DndContext: ({ children }: { children: ReactNode }) => children,
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

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/plan', element: <BudgetPage key="plan" mode="plan" /> }], { initialEntries: ['/plan'] })
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

function usePlanHandlers(plan: unknown = fixtureWirePlan) {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(plan),
  )
}

function recordSetLimit() {
  const calls: { elementId: string; period: string; amount: string | null }[] = []
  server.use(
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      calls.push((await request.json()) as { elementId: string; period: string; amount: string | null })
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  return calls
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 7, 15, 12, 0, 0))
  vi.clearAllMocks()
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
  useBudgetPeriodStore.setState({
    selectedDate: '2026-07-01',
    unfoldedElements: {},
    foldBudgetId: null,
    planFolds: {},
  })
})

// Jun/Jul/Aug on screen; pe1 "Living" is planned 250 in Jul (column 1), unplanned in Aug
async function gridAtFirstExpenseCell() {
  const grid = await screen.findByTestId('plan-sheet')
  const cell = screen.getByTestId('plan-cell-pe1:1')
  fireEvent.click(cell)
  return { grid, cell }
}

it('typing a digit edits the cell in place and Enter commits and moves down', async () => {
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: '7' })
  const input = await screen.findByRole('textbox', { name: 'Plan for Living, July' })
  expect(input).toHaveValue('7')
  expect(input).toHaveFocus()
  fireEvent.change(input, { target: { value: '75' } })
  fireEvent.keyDown(input, { key: 'Enter' })
  await waitFor(() => expect(calls).toEqual([expect.objectContaining({ elementId: 'pe1', period: '2026-07-01', amount: '75' })]))
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  // the selection moved one row down, over the No folder line (no stop) to Food
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('aria-selected', 'false')
  expect(screen.getByTestId('plan-cell-cat-food:1')).toHaveAttribute('aria-selected', 'true')
  expect(grid).toHaveFocus()
})

it('F2 edits the current value; Esc cancels without a request', async () => {
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: 'F2' })
  const input = await screen.findByRole('textbox')
  expect(input).toHaveValue('250')
  expect((input as HTMLInputElement).selectionStart).toBe(3)
  fireEvent.keyDown(input, { key: 'Escape' })
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  expect(grid).toHaveFocus()
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('aria-selected', 'true')
  await new Promise((r) => setTimeout(r, 30))
  expect(calls).toEqual([])
})

it('Tab commits and moves right; an unchanged value sends nothing', async () => {
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: 'Enter' })
  const input = await screen.findByRole('textbox')
  // the same amount written differently is still unchanged
  fireEvent.change(input, { target: { value: '250.00' } })
  fireEvent.keyDown(input, { key: 'Tab' })
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  expect(screen.getByTestId('plan-cell-pe1:2')).toHaveAttribute('aria-selected', 'true')
  await new Promise((r) => setTimeout(r, 30))
  expect(calls).toEqual([])
})

it('Shift+Tab commits and moves left; ↑ commits and moves up', async () => {
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: '3' })
  fireEvent.keyDown(await screen.findByRole('textbox'), { key: 'Tab', shiftKey: true })
  await waitFor(() => expect(calls).toEqual([expect.objectContaining({ elementId: 'pe1', period: '2026-07-01', amount: '3' })]))
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')

  fireEvent.keyDown(grid, { key: '4' })
  fireEvent.keyDown(await screen.findByRole('textbox'), { key: 'ArrowUp' })
  await waitFor(() => expect(calls).toHaveLength(2))
  expect(calls[1]).toEqual(expect.objectContaining({ elementId: 'pe1', period: '2026-06-01', amount: '4' }))
  // the Expenses section and Essentials folder lines above Living are no stops: the
  // row above is the income side's Uncategorized, in the same month
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'false')
  const incomeUncat = document.querySelector('[data-row-id="uncategorized:3"]') as HTMLElement
  expect(within(incomeUncat).getAllByRole('gridcell')[0]).toHaveAttribute('aria-selected', 'true')
})

it('← and → inside the editor move the caret, not the selection', async () => {
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: 'F2' })
  const input = await screen.findByRole('textbox')
  fireEvent.keyDown(input, { key: 'ArrowLeft' })
  fireEvent.keyDown(input, { key: 'ArrowRight' })
  expect(screen.getByRole('textbox')).toBe(input)
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('aria-selected', 'true')
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-07-01')
  expect(calls).toEqual([])
})

it('an invalid value keeps the editor open with a message', async () => {
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: '1' })
  const input = await screen.findByRole('textbox')
  fireEvent.change(input, { target: { value: '1+*' } })
  fireEvent.keyDown(input, { key: 'Enter' })
  expect(input).toHaveAttribute('aria-invalid', 'true')
  expect(screen.getByRole('textbox')).toBeInTheDocument()
  expect(screen.getByRole('alert')).toHaveTextContent('Only numbers and operators')
  // fixing it clears the message
  fireEvent.change(input, { target: { value: '1+2' } })
  expect(input).toHaveAttribute('aria-invalid', 'false')
  fireEvent.keyDown(input, { key: 'Enter' })
  await waitFor(() => expect(calls).toEqual([expect.objectContaining({ elementId: 'pe1', amount: '3' })]))
})

it('clicking another cell commits the open editor', async () => {
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: '8' })
  const input = await screen.findByRole('textbox')
  fireEvent.blur(input)
  fireEvent.click(screen.getByTestId('plan-cell-cat-food:1'))
  await waitFor(() => expect(calls).toEqual([expect.objectContaining({ elementId: 'pe1', period: '2026-07-01', amount: '8' })]))
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  expect(screen.getByTestId('plan-cell-cat-food:1')).toHaveAttribute('aria-selected', 'true')
})

it('double-clicking a cell edits it, but double-clicking its actual does not', async () => {
  usePlanHandlers()
  renderPage()
  await screen.findByTestId('plan-sheet')
  const cell = screen.getByTestId('plan-cell-pe1:1')
  fireEvent.doubleClick(within(cell).getByTestId('cell-actual'))
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  fireEvent.doubleClick(within(cell).getByTestId('cell-planned'))
  expect(await screen.findByRole('textbox')).toHaveValue('250')
})

it('Delete clears a planned cell and tracks it', async () => {
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: 'Delete' })
  await waitFor(() => expect(calls).toEqual([expect.objectContaining({ elementId: 'pe1', period: '2026-07-01', amount: null })]))
  await waitFor(() => expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_CLEAR_CELL))
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
})

it('Backspace on an unplanned cell sends nothing', async () => {
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')
  fireEvent.click(screen.getByTestId('plan-cell-pe1:2')) // Aug, unplanned
  fireEvent.keyDown(grid, { key: 'Backspace' })
  await new Promise((r) => setTimeout(r, 30))
  expect(calls).toEqual([])
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.BUDGET_PLAN_CLEAR_CELL)
})

it('a read-only cell never opens an editor and Delete sends nothing', async () => {
  // a guest: role decides editability (same setup as comments.plan.guest.test.tsx)
  const guestWireBudget = {
    ...fixtureWireBudget,
    meta: {
      ...fixtureWireBudget.meta,
      ownerUserId: 'u9',
      access: [
        { user: { id: 'u9', avatar: 'face:sky', name: 'Owner' }, role: 'owner', isAccepted: 1 },
        { user: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, role: 'guest', isAccepted: 1 },
      ],
    },
  }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: guestWireBudget } })),
    planHandler(),
  )
  const calls = recordSetLimit()
  renderPage()
  const { grid, cell } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: '5' })
  fireEvent.keyDown(grid, { key: 'F2' })
  fireEvent.keyDown(grid, { key: 'Enter' })
  fireEvent.keyDown(grid, { key: 'Delete' })
  fireEvent.keyDown(grid, { key: 'Backspace' })
  fireEvent.doubleClick(within(cell).getByTestId('cell-planned'))
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  await new Promise((r) => setTimeout(r, 50))
  expect(calls).toEqual([])
})

it('an archived row never opens an editor and Delete sends nothing', async () => {
  const plan = {
    ...fixtureWirePlan,
    structure: {
      ...fixtureWirePlan.structure,
      elements: [
        ...fixtureWirePlan.structure.elements,
        {
          id: 'arch-1', type: 1, name: 'Old hobby', icon: 'delete', currencyId: 'cur-usd', isArchived: 1, folderId: null, position: 9, ownerUserId: 'u1',
          cells: [{ actual: '0', planned: '' }, { actual: '0', planned: '40' }, { actual: '0', planned: '40' }, { actual: '0', planned: '' }],
          children: [],
        },
      ],
    },
  }
  usePlanHandlers(plan)
  const calls = recordSetLimit()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')
  fireEvent.click(await screen.findByTestId('plan-cell-arch-1:1'))
  fireEvent.keyDown(grid, { key: '5' })
  fireEvent.keyDown(grid, { key: 'Enter' })
  fireEvent.keyDown(grid, { key: 'Delete' })
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  await new Promise((r) => setTimeout(r, 50))
  expect(calls).toEqual([])
})

it('on a tablet typing opens no editor', async () => {
  mockCompactViewport()
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')
  act(() => {
    fireEvent.keyDown(grid, { key: 'ArrowDown' })
  })
  fireEvent.keyDown(grid, { key: '5' })
  fireEvent.keyDown(grid, { key: 'F2' })
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  await new Promise((r) => setTimeout(r, 30))
  expect(calls).toEqual([])
})

it('on a tablet Delete and Backspace never clear a plan', async () => {
  mockCompactViewport()
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')
  // a tap opens the item sheet; close it and keep the selection it made
  fireEvent.click(screen.getByTestId('plan-cell-pe1:1'))
  fireEvent.keyDown(await screen.findByTestId('element-sheet'), { key: 'Escape' })
  await waitFor(() => expect(screen.queryByTestId('element-sheet')).not.toBeInTheDocument())
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('aria-selected', 'true')
  fireEvent.keyDown(grid, { key: 'Delete' })
  fireEvent.keyDown(grid, { key: 'Backspace' })
  await new Promise((r) => setTimeout(r, 50))
  expect(calls).toEqual([])
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.BUDGET_PLAN_CLEAR_CELL)
})

it('an IME composition Enter does not commit the cell', async () => {
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: '1' })
  const input = await screen.findByRole('textbox')
  fireEvent.keyDown(input, { key: 'Enter', isComposing: true })
  expect(screen.getByRole('textbox')).toBe(input)
  expect(screen.getByTestId('plan-cell-pe1:1')).toHaveAttribute('aria-selected', 'true')
  await new Promise((r) => setTimeout(r, 30))
  expect(calls).toEqual([])
})

it('the browser window losing focus keeps the editor open and writes nothing', async () => {
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: '4' })
  const input = await screen.findByRole('textbox')
  const hasFocus = vi.spyOn(document, 'hasFocus').mockReturnValue(false)
  try {
    fireEvent.blur(input)
  } finally {
    hasFocus.mockRestore()
  }
  expect(screen.getByRole('textbox')).toBe(input)
  expect(input).toHaveValue('4')
  await new Promise((r) => setTimeout(r, 30))
  expect(calls).toEqual([])
  // back in the window, the edit goes on as usual
  fireEvent.keyDown(input, { key: 'Enter' })
  await waitFor(() => expect(calls).toEqual([expect.objectContaining({ elementId: 'pe1', period: '2026-07-01', amount: '4' })]))
})

it('editing at the right edge: Tab commits to the edited month, then the window moves', async () => {
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  await screen.findByTestId('plan-sheet')
  fireEvent.click(screen.getByTestId('plan-cell-pe1:2')) // Aug, last column
  const grid = screen.getByTestId('plan-sheet')
  fireEvent.keyDown(grid, { key: '9' })
  const input = await screen.findByRole('textbox')
  fireEvent.keyDown(input, { key: 'Tab' })
  await waitFor(() => expect(calls[0]?.period).toBe('2026-08-01'))
  expect(calls).toHaveLength(1)
  expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-08-01')
  // the selection keeps its column, now Sep
  await waitFor(() => expect(screen.getByTestId('plan-cell-pe1:2')).toHaveAttribute('data-month', '2026-09-01'))
  expect(screen.getByTestId('plan-cell-pe1:2')).toHaveAttribute('aria-selected', 'true')
})

it('editing at the left edge: Shift+Tab commits to the edited month, then the window moves back', async () => {
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')
  fireEvent.click(screen.getByTestId('plan-cell-pe1:0')) // Jun, first column
  fireEvent.keyDown(grid, { key: '6' })
  fireEvent.keyDown(await screen.findByRole('textbox'), { key: 'Tab', shiftKey: true })
  await waitFor(() => expect(calls).toEqual([expect.objectContaining({ elementId: 'pe1', period: '2026-06-01', amount: '6' })]))
  expect(calls).toHaveLength(1)
  // like ← on the first month: the window pages back and the selection keeps its
  // column, now May
  await waitFor(() => expect(useBudgetPeriodStore.getState().selectedDate).toBe('2026-06-01'))
  await waitFor(() => expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('data-month', '2026-05-01'))
  expect(screen.getByTestId('plan-cell-pe1:0')).toHaveAttribute('aria-selected', 'true')
  expect(grid.getAttribute('aria-activedescendant')).toBe(screen.getByTestId('plan-cell-pe1:0').id)
})

// The editor takes the plan's place only: the month's actual stays in view beside it.
it('while a month is edited its actual stays in view beside the editor', async () => {
  usePlanHandlers()
  renderPage()
  const { grid, cell } = await gridAtFirstExpenseCell()
  // Living, July: 45 spent of 250 planned
  expect(within(cell).getByTestId('cell-actual')).toHaveTextContent('45')
  fireEvent.keyDown(grid, { key: 'F2' })
  const input = await screen.findByRole('textbox', { name: 'Plan for Living, July' })
  expect(cell).toContainElement(input)
  expect(input).toHaveValue('250')
  const actual = within(cell).getByTestId('cell-actual')
  expect(actual).toHaveTextContent('45')
  expect(actual).toBeVisible()
  // the plan figure itself is what the editor replaces
  expect(within(cell).queryByTestId('cell-planned')).not.toBeInTheDocument()
  fireEvent.keyDown(input, { key: 'Escape' })
  expect(within(cell).getByTestId('cell-planned')).toHaveTextContent('250')

  // typing over the value and double-clicking keep it in view the same way
  fireEvent.keyDown(grid, { key: '9' })
  expect(within(cell).getByTestId('cell-actual')).toHaveTextContent('45')
  fireEvent.keyDown(await screen.findByRole('textbox'), { key: 'Escape' })
  fireEvent.doubleClick(within(cell).getByTestId('cell-planned'))
  expect(await screen.findByRole('textbox')).toBeInTheDocument()
  expect(within(cell).getByTestId('cell-actual')).toHaveTextContent('45')
})

it('two clears in quick succession are tracked twice', async () => {
  usePlanHandlers()
  const calls: unknown[] = []
  server.use(
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      calls.push(await request.json())
      await delay(50)
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  renderPage()
  const { grid } = await gridAtFirstExpenseCell()
  fireEvent.keyDown(grid, { key: 'Delete' })
  // vacation is planned 50 in July: cleared before the first request has answered
  fireEvent.click(screen.getByTestId('plan-cell-tag1:1'))
  fireEvent.keyDown(grid, { key: 'Delete' })
  await waitFor(() => expect(calls).toHaveLength(2))
  await waitFor(() => expect(vi.mocked(trackEvent).mock.calls.filter(([m]) => m === METRICS.BUDGET_PLAN_CLEAR_CELL)).toHaveLength(2))
})

it('keys pressed on a focused control in the grid are the control\'s, not the grid\'s', async () => {
  usePlanHandlers()
  renderPage()
  await gridAtFirstExpenseCell()
  // a month cell is selected; Enter or a digit on Living's fold chevron opens no editor
  const living = document.querySelector('[data-row-id="pe1:0"]') as HTMLElement
  const chevron = within(living).getByRole('button', { name: 'Expand' })
  chevron.focus()
  fireEvent.keyDown(chevron, { key: 'Enter' })
  fireEvent.keyDown(chevron, { key: '5' })
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()

  // Enter on the ⋮ trigger opens its menu and nothing else: no in-cell editor ...
  const trigger = within(living).getByRole('button', { name: 'menu Living' })
  trigger.focus()
  fireEvent.keyDown(trigger, { key: 'Enter' })
  expect(await screen.findByRole('menu')).toBeInTheDocument()
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
  fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' })
  await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())

  // ... and a click on the name (which selects nothing) leaves Enter on the ⋮
  // trigger opening only its menu: no edit dialog
  fireEvent.click(within(living).getByTitle('Living'))
  trigger.focus()
  fireEvent.keyDown(trigger, { key: 'Enter' })
  expect(await screen.findByRole('menu')).toBeInTheDocument()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})
