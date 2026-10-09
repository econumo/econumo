import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent, { PointerEventsCheckLevel } from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget, planHandler } from '@/test/fixtures'
import { BudgetPage } from './BudgetPage'
import { useBudgetPeriodStore } from './budgetStore'

// jsdom cannot drive real dnd-kit pointer drags (no layout); PlanSheet mounts a
// DndContext per band only in edit mode, which these tests never enter, but the
// harness mocks it anyway for parity with PlanSheet.test.tsx.
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

function mockTabletViewport() {
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
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

const comment = {
  id: 'cm1',
  elementId: 'pe1',
  period: '2026-08-01',
  comment: 'Trip to Lisbon',
  author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' },
  createdAt: '2026-08-17 09:00:00',
  updatedAt: '2026-08-17 09:00:00',
}

function usePlanHandlers() {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [comment], truncated: false } }),
    ),
  )
}

// The plan fixtures span May-Aug 2026 and read "today" off the system clock, so the
// clock is pinned to Aug 2026 and the selected month is August: a three-month window
// of Jul/Aug/Sep.
beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 7, 15, 12, 0, 0))
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
  useBudgetPeriodStore.setState({
    selectedDate: '2026-08-01',
    unfoldedElements: {},
    foldBudgetId: null,
    planFolds: {},
  })
})

afterEach(() => {
  vi.useRealTimers()
})

it('opens the thread from the marker as a popover, and keeps the in-cell editor comment-free', async () => {
  usePlanHandlers()
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:1')
  expect(within(cell).getByTestId('comment-marker')).toHaveAccessibleName('1 comment')

  await user.click(cell)
  await user.keyboard('{Enter}')
  expect(await screen.findByRole('textbox', { name: 'Plan for Living, August' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /Comments \(/ })).toBeNull()
  // the corner marker steps aside while the cell is being edited
  expect(within(cell).queryByTestId('comment-marker')).toBeNull()
  await user.keyboard('{Escape}')
  expect(screen.queryByRole('textbox')).toBeNull()

  await user.click(within(cell).getByTestId('comment-marker'))
  expect(await screen.findByTestId('comments-popover')).toHaveTextContent('Trip to Lisbon')
})

it('shows the truncated notice when get-comment-list reports its cap was hit', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [comment], truncated: true } }),
    ),
  )
  mockViewport()
  const user = userEvent.setup()
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:1')
  await user.click(within(cell).getByTestId('comment-marker'))
  expect(await screen.findByText('Showing the latest 2000 comments.')).toBeInTheDocument()
})

it('shows no marker on a cell without comments', async () => {
  usePlanHandlers()
  mockViewport()
  renderPage('/plan')
  const cell = await screen.findByTestId('plan-cell-pe1:2')
  expect(within(cell).queryByTestId('comment-marker')).toBeNull()
})

it('never marks the uncategorized row', async () => {
  // the fixture's uncategorized row carries no element id a comment could name
  usePlanHandlers()
  mockViewport()
  renderPage('/plan')
  const cell = await screen.findByTestId('plan-cell-uncategorized:1')
  expect(within(cell).queryByTestId('comment-marker')).toBeNull()
})

it('opens the thread with Shift+Enter and leaves Enter editing the amount', async () => {
  usePlanHandlers()
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:1')
  await user.click(cell)
  screen.getByTestId('plan-sheet').focus()

  await user.keyboard('{Shift>}{Enter}{/Shift}')
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
  await user.keyboard('{Escape}')

  await user.keyboard('{Enter}')
  expect(await screen.findByRole('textbox', { name: 'Plan for Living, August' })).toBeInTheDocument()
})

it('does not steal focus from a later mouse-opened dialog after a keyboard-opened thread closes', async () => {
  usePlanHandlers()
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:1')
  await user.click(cell)
  screen.getByTestId('plan-sheet').focus()
  await user.keyboard('{Shift>}{Enter}{/Shift}')
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
  await user.keyboard('{Escape}')

  // a later, unrelated mouse-opened dialog (the row menu's own Edit) must close
  // without the grid stealing focus back — the bug this guards against left
  // editorFromGrid stuck true from the Shift+Enter above
  await user.click(await screen.findByRole('button', { name: 'menu Living' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  const dialog = await screen.findByRole('dialog', { name: 'Edit envelope' })
  await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Edit envelope' })).not.toBeInTheDocument())

  expect(screen.getByTestId('plan-sheet')).not.toHaveFocus()
})

it('opens the thread with Shift+F2', async () => {
  usePlanHandlers()
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:1')
  await user.click(cell)
  screen.getByTestId('plan-sheet').focus()
  await user.keyboard('{Shift>}{F2}{/Shift}')
  expect(await screen.findByTestId('comments-popover')).toHaveTextContent('Trip to Lisbon')
})

it('leaves a right-click on a plan cell to the browser: no app menu', async () => {
  usePlanHandlers()
  mockViewport()
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:1')
  // fireEvent returns false when a handler called preventDefault
  expect(fireEvent.contextMenu(cell)).toBe(true)
  await new Promise((r) => setTimeout(r, 100))
  expect(screen.queryByRole('menu')).toBeNull()
})

it('offers a hover-only "Add comment" corner on a plan cell without comments, opening an empty thread beside it', async () => {
  usePlanHandlers()
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  expect(within(await screen.findByTestId('plan-cell-pe1:1')).queryByTestId('comment-marker-add')).toBeNull()
  const cell = screen.getByTestId('plan-cell-pe1:0')
  expect(cell).toHaveClass('group/cell')
  const add = within(cell).getByTestId('comment-marker-add')
  expect(add).toHaveAccessibleName('Add comment')
  expect(add).toHaveClass('invisible', 'group-hover/cell:visible')
  await user.click(add)
  const popover = await screen.findByTestId('comments-popover')
  expect(within(popover).getByText('No comments yet.')).toBeInTheDocument()
  expect(within(popover).getByRole('button', { name: 'Post' })).toBeInTheDocument()
  expect(screen.queryByLabelText('Budget')).toBeNull()
})

it('offers no add-comment corner on the uncategorized row', async () => {
  usePlanHandlers()
  mockViewport()
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-uncategorized:1')
  expect(within(cell).queryByTestId('comment-marker-add')).toBeNull()
})

it('offers no add-comment corner in edit-structure mode', async () => {
  usePlanHandlers()
  // only a touch screen has an edit mode
  mockTabletViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  expect(within(await screen.findByTestId('plan-cell-pe1:0')).getByTestId('comment-marker-add')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('button', { name: 'Edit structure' }))
  await waitFor(() => expect(within(screen.getByTestId('plan-cell-pe1:0')).queryByTestId('comment-marker-add')).toBeNull())
})

it('offers no add-comment corner on a month after the budget ends (its thread is read-only)', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () =>
      HttpResponse.json({
        success: true,
        message: '',
        // a one-month budget: the window never runs past the end month unless the
        // budget is shorter than the window, so Aug and Sep trail after July's end
        data: { item: { ...fixtureWireBudget, meta: { ...fixtureWireBudget.meta, startedAt: '2026-07-01 00:00:00', endedAt: '2026-07-01 00:00:00' } } },
      }),
    ),
    planHandler(),
    http.get('*/api/v1/budget/get-comment-list', () => HttpResponse.json({ success: true, message: '', data: { items: [], truncated: false } })),
  )
  mockViewport()
  renderPage('/plan')

  // column 0 = July, inside the range; column 2 = September, after the end month
  expect(within(await screen.findByTestId('plan-cell-pe1:0')).getByTestId('comment-marker-add')).toBeInTheDocument()
  expect(within(screen.getByTestId('plan-cell-pe1:2')).queryByTestId('comment-marker-add')).toBeNull()
})

it('a tablet tap on a plan cell opens the item sheet for that month', async () => {
  usePlanHandlers()
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage('/plan')

  await user.click(await screen.findByTestId('plan-cell-pe1:1'))
  const sheet = await screen.findByTestId('element-sheet')
  expect(within(sheet).getByTestId('sheet-figure-budget')).toBeInTheDocument()
  expect(within(sheet).getByTestId('sheet-figure-spent')).toBeInTheDocument()
  expect(within(sheet).getByRole('button', { name: 'Comments (1)' })).toBeInTheDocument()
})

it('a tablet Enter on a selected plan cell opens the item sheet, not an amount editor', async () => {
  usePlanHandlers()
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage('/plan')

  // the tap selects the cell and opens the sheet; close it and reuse that
  // selection to drive the grid's own Enter handling
  await user.click(await screen.findByTestId('plan-cell-pe1:1'))
  await screen.findByTestId('element-sheet')
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByTestId('element-sheet')).toBeNull())

  screen.getByTestId('plan-sheet').focus()
  await user.keyboard('{Enter}')
  expect(await screen.findByTestId('element-sheet')).toBeInTheDocument()
  expect(screen.queryByLabelText('Budget')).toBeNull()
  expect(screen.queryByRole('textbox', { name: /^Plan for/ })).toBeNull()
})

it('a tablet sheet’s Set budget opens the amount dialog with no comments in it', async () => {
  usePlanHandlers()
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage('/plan')

  await user.click(await screen.findByTestId('plan-cell-pe1:1'))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Set budget' }))
  expect(await screen.findByLabelText('Budget')).toBeInTheDocument()
  expect(screen.queryByTestId('element-sheet')).toBeNull()
  expect(screen.queryByRole('button', { name: /Comments \(/ })).toBeNull()
})

it('a tablet sheet’s Edit opens the envelope dialog in place of the sheet', async () => {
  usePlanHandlers()
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage('/plan')

  await user.click(await screen.findByTestId('plan-cell-pe1:1'))
  await user.click(within(await screen.findByRole('dialog', { name: /^Living · / })).getByRole('button', { name: 'Edit' }))
  expect(await screen.findByRole('dialog', { name: 'Edit envelope' })).toBeInTheDocument()
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

it('a tablet sheet’s Comments opens the thread', async () => {
  usePlanHandlers()
  mockTabletViewport()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage('/plan')

  await user.click(await screen.findByTestId('plan-cell-pe1:1'))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Comments (1)' }))
  expect(await screen.findByRole('button', { name: 'Post' })).toBeInTheDocument()
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

it('a tablet marker tap opens only the thread, not the sheet', async () => {
  usePlanHandlers()
  mockTabletViewport()
  const user = userEvent.setup()
  renderPage('/plan')

  await user.click(within(await screen.findByTestId('plan-cell-pe1:1')).getByTestId('comment-marker'))
  expect(await screen.findByTestId('comments-popover')).toHaveTextContent('Trip to Lisbon')
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

