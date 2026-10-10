import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
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
    planUnfoldedElements: {},
    foldBudgetId: null,
    planFolds: {},
  })
})

// Jun/Jul/Aug on screen, August is the current month. Living (pe1): Jun 60·200,
// Jul 45·250, Aug 0·—. Food (cat-food), the next row down: Jun 130·150, Jul 125·—.
const cell = (key: string) => screen.getByTestId(`plan-cell-${key}`)
const hint = () => screen.queryByTestId('plan-selection-hint')

it('Shift+click selects the rectangle from the active cell and shows its sum', async () => {
  usePlanHandlers()
  renderPage()
  await screen.findByTestId('plan-sheet')
  fireEvent.click(cell('pe1:0'))
  expect(hint()).not.toBeInTheDocument()
  fireEvent.click(cell('cat-food:1'), { shiftKey: true })
  const h = await screen.findByTestId('plan-selection-hint')
  expect(within(h).getByTestId('plan-selection-count')).toHaveTextContent('4 cells')
  expect(within(h).getByTestId('plan-selection-planned')).toHaveTextContent('600')
  expect(within(h).getByTestId('plan-selection-actual')).toHaveTextContent('360')
  for (const k of ['pe1:0', 'pe1:1', 'cat-food:0', 'cat-food:1']) {
    expect(cell(k)).toHaveAttribute('aria-selected', 'true')
  }
  expect(cell('pe1:2')).toHaveAttribute('aria-selected', 'false')
  // the cell the range grew from stays the active one
  expect(screen.getByTestId('plan-sheet')).toHaveAttribute('aria-activedescendant', cell('pe1:0').id)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.BUDGET_PLAN_SELECT_CELLS)
})

it('Ctrl/Cmd+click adds and removes single cells', async () => {
  usePlanHandlers()
  renderPage()
  await screen.findByTestId('plan-sheet')
  fireEvent.click(cell('pe1:1'))
  fireEvent.click(cell('tag1:1'), { ctrlKey: true })
  expect(await screen.findByTestId('plan-selection-count')).toHaveTextContent('2 cells')
  // Living Jul 250 + vacation Jul 50; actuals 45 + 15
  expect(screen.getByTestId('plan-selection-planned')).toHaveTextContent('300')
  expect(screen.getByTestId('plan-selection-actual')).toHaveTextContent('60')
  fireEvent.click(cell('cat-food:0'), { metaKey: true })
  expect(screen.getByTestId('plan-selection-count')).toHaveTextContent('3 cells')
  expect(cell('pe1:2')).toHaveAttribute('aria-selected', 'false')
  // the analytics event marks the selection growing past one cell, once
  expect(vi.mocked(trackEvent).mock.calls.filter(([m]) => m === METRICS.BUDGET_PLAN_SELECT_CELLS)).toHaveLength(1)
  fireEvent.click(cell('cat-food:0'), { ctrlKey: true })
  fireEvent.click(cell('tag1:1'), { ctrlKey: true })
  expect(hint()).not.toBeInTheDocument()
  expect(cell('pe1:1')).toHaveAttribute('aria-selected', 'true')
})

it('dragging across cells selects the rectangle; a plain click goes back to one cell', async () => {
  usePlanHandlers()
  renderPage()
  await screen.findByTestId('plan-sheet')
  fireEvent.pointerDown(cell('pe1:0'), { button: 0, buttons: 1, pointerType: 'mouse' })
  fireEvent.pointerEnter(cell('pe1:1'), { buttons: 1, pointerType: 'mouse' })
  fireEvent.pointerEnter(cell('cat-food:2'), { buttons: 1, pointerType: 'mouse' })
  fireEvent.pointerUp(window)
  expect(await screen.findByTestId('plan-selection-count')).toHaveTextContent('6 cells')
  // once released, moving over cells changes nothing
  fireEvent.pointerEnter(cell('pe1:1'), { buttons: 0, pointerType: 'mouse' })
  expect(screen.getByTestId('plan-selection-count')).toHaveTextContent('6 cells')
  fireEvent.click(cell('tag1:0'))
  expect(hint()).not.toBeInTheDocument()
  expect(cell('pe1:0')).toHaveAttribute('aria-selected', 'false')
  expect(cell('tag1:0')).toHaveAttribute('aria-selected', 'true')
})

it('Esc and the arrow keys drop back to the active cell', async () => {
  usePlanHandlers()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')
  fireEvent.click(cell('pe1:0'))
  fireEvent.click(cell('cat-food:1'), { shiftKey: true })
  expect(await screen.findByTestId('plan-selection-hint')).toBeInTheDocument()
  fireEvent.keyDown(grid, { key: 'Escape' })
  expect(hint()).not.toBeInTheDocument()
  expect(cell('pe1:0')).toHaveAttribute('aria-selected', 'true')
  expect(cell('cat-food:1')).toHaveAttribute('aria-selected', 'false')
  fireEvent.click(cell('cat-food:1'), { shiftKey: true })
  expect(await screen.findByTestId('plan-selection-hint')).toBeInTheDocument()
  fireEvent.keyDown(grid, { key: 'ArrowDown' })
  expect(hint()).not.toBeInTheDocument()
  expect(cell('cat-food:0')).toHaveAttribute('aria-selected', 'true')
})

it('Delete with several cells selected clears the active cell only', async () => {
  usePlanHandlers()
  const calls = recordSetLimit()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')
  fireEvent.click(cell('pe1:0'))
  fireEvent.click(cell('cat-food:1'), { shiftKey: true })
  fireEvent.keyDown(grid, { key: 'Delete' })
  await waitFor(() => expect(calls).toEqual([expect.objectContaining({ elementId: 'pe1', period: '2026-06-01', amount: null })]))
  await new Promise((r) => setTimeout(r, 30))
  expect(calls).toHaveLength(1)
})

it('typing into a multi-selection edits the active cell and drops the rest', async () => {
  usePlanHandlers()
  renderPage()
  const grid = await screen.findByTestId('plan-sheet')
  fireEvent.click(cell('pe1:0'))
  fireEvent.click(cell('cat-food:1'), { shiftKey: true })
  fireEvent.keyDown(grid, { key: '5' })
  expect(await screen.findByRole('textbox', { name: 'Plan for Living, June' })).toBeInTheDocument()
  expect(hint()).not.toBeInTheDocument()
})

it('paging the months drops the multi-selection', async () => {
  usePlanHandlers()
  renderPage()
  await screen.findByTestId('plan-sheet')
  fireEvent.click(cell('pe1:0'))
  fireEvent.click(cell('cat-food:1'), { shiftKey: true })
  expect(await screen.findByTestId('plan-selection-hint')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Scroll to later months' }))
  await waitFor(() => expect(hint()).not.toBeInTheDocument())
})

it('a touch viewport keeps one cell: modifier clicks and drags select nothing more', async () => {
  mockCompactViewport()
  usePlanHandlers()
  renderPage()
  await screen.findByTestId('plan-sheet')
  fireEvent.click(cell('pe1:0'))
  fireEvent.click(cell('cat-food:1'), { shiftKey: true })
  fireEvent.click(cell('tag1:1'), { ctrlKey: true })
  fireEvent.pointerDown(cell('pe1:0'), { button: 0, buttons: 1, pointerType: 'mouse' })
  fireEvent.pointerEnter(cell('cat-food:2'), { buttons: 1, pointerType: 'mouse' })
  fireEvent.pointerUp(window)
  expect(hint()).not.toBeInTheDocument()
})
