import type { ReactNode } from 'react'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget, planHandler } from '@/test/fixtures'
import { BudgetPage } from './BudgetPage'
import { useBudgetPeriodStore } from './budgetStore'

// A guest cannot edit any cell in the plan view (role, not viewport, decides
// editability), so this suite pins the gap the review round found: before the
// fix, a non-editable cell rendered bare `plannedText` with no click handler
// at all, on every viewport — a guest could reopen a thread someone else had
// already started (via the marker) but could never start one themselves.
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

function useGuestPlanHandlers() {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: guestWireBudget } })),
    planHandler(),
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [], truncated: false } }),
    ),
  )
}

// Same clock pin as comments.plan.test.tsx / PlanSheet.test.tsx: the fixture
// spans May-Aug 2026 and the default 3-month window resolves to Jul/Aug/Sep.
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

it('lets a guest start a thread on a cell with no existing comments, on desktop', async () => {
  useGuestPlanHandlers()
  mockViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  // column 0 = July, inside the fetched window (idx >= 0), so the cell would
  // be editable for a real member — the guest role is what makes it read-only
  const cell = await screen.findByTestId('plan-cell-pe1:0')
  expect(within(cell).queryByTestId('comment-marker')).toBeNull()
  expect(within(cell).queryByLabelText(/^limit /)).toBeNull()

  await user.click(within(cell).getByLabelText(/^comments /))
  expect(await screen.findByRole('button', { name: 'Post' })).toBeInTheDocument()
})

it('lets a guest start a thread on a cell with no existing comments, on compact viewports', async () => {
  useGuestPlanHandlers()
  mockCompactViewport()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage('/plan')

  const cell = await screen.findByTestId('plan-cell-pe1:0')
  await user.click(within(cell).getByLabelText(/^comments /))
  expect(await screen.findByRole('button', { name: 'Post' })).toBeInTheDocument()
})
