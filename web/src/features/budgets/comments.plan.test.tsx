import type { ReactNode } from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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

// The plan fixtures span May-Aug 2026 and read "today" off the system clock (the
// default three-month window resolves to Jul/Aug/Sep, same as PlanSheet.test.tsx),
// so the clock is pinned the same way here.
beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 7, 15, 12, 0, 0))
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
  useBudgetPeriodStore.setState({
    selectedDate: '2026-07-01',
    unfoldedElements: {},
    foldBudgetId: null,
    planFirstMonth: null,
    planFolds: {},
    planHideEmpty: false,
  })
})

afterEach(() => {
  vi.useRealTimers()
})

it('marks a cell that has comments and opens its thread from the limit popover', async () => {
  usePlanHandlers()
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:1')
  expect(within(cell).getByTestId('comment-marker')).toHaveAccessibleName('1 comment')

  await user.click(within(cell).getByLabelText(/^limit /))
  await user.click(await screen.findByRole('button', { name: /Comments \(1\)/ }))
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
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
  await user.click(within(cell).getByLabelText(/^limit /))
  await user.click(await screen.findByRole('button', { name: /Comments \(1\)/ }))
  expect(await screen.findByText('Showing the first 2000 comments.')).toBeInTheDocument()
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
  expect(await screen.findByLabelText('Budget')).toBeInTheDocument()
})

it('opens the thread in a dialog on compact viewports', async () => {
  usePlanHandlers()
  mockCompactViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:1')
  await user.click(within(cell).getByTestId('comment-marker'))
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
})

it('clicking the marker while the popover is already open expands the thread in place, without closing the popover or losing the draft amount', async () => {
  usePlanHandlers()
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:1')
  await user.click(within(cell).getByLabelText(/^limit /))
  const input = await screen.findByLabelText('Budget')
  await user.clear(input)
  await user.type(input, '999')

  await user.click(within(cell).getByTestId('comment-marker'))

  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
  expect(screen.getByLabelText('Budget')).toHaveValue('999')
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
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  await user.click(await screen.findByRole('button', { name: 'element actions Living' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit' }))
  const dialog = await screen.findByRole('dialog', { name: 'Edit envelope' })
  await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Edit envelope' })).not.toBeInTheDocument())

  expect(screen.getByTestId('plan-sheet')).not.toHaveFocus()
})
