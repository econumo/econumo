import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget } from '@/test/fixtures'
import { BudgetPage } from './BudgetPage'
import { useBudgetPeriodStore } from './budgetStore'

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

const comment = {
  id: 'cm1',
  elementId: 'cat-food',
  period: '2026-07-01',
  comment: 'Trip to Lisbon',
  author: { id: 'u1', avatar: 'face:emerald', name: 'Ada' },
  createdAt: '2026-07-17 09:00:00',
  updatedAt: '2026-07-17 09:00:00',
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

function renderPage(initialPath: '/budget' | '/plan' = '/budget') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/budget', element: <BudgetPage key="budget" mode="budget" /> },
      { path: '/plan', element: <BudgetPage key="plan" mode="plan" /> },
    ],
    { initialEntries: [initialPath] },
  )
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

function registerMonthlyHandlers(wireBudget: unknown = fixtureWireBudget) {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: wireBudget } })),
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [comment], truncated: false } }),
    ),
  )
}

function renderGuestPage(initialPath: '/budget' | '/plan' = '/budget') {
  registerMonthlyHandlers(guestWireBudget)
  return renderPage(initialPath)
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null, planHideEmpty: false })
})

it('marks the budgeted cell and opens the thread from the limit popover', async () => {
  registerMonthlyHandlers()
  mockViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  const row = await screen.findByTestId('element-cat-food')
  expect(within(row).getByTestId('comment-marker')).toHaveAccessibleName('1 comment')

  await user.click(within(row).getByLabelText(/^limit /))
  await user.click(await screen.findByRole('button', { name: /Comments \(1\)/ }))
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
})

it('offers the thread inside SetLimitDialog on compact viewports', async () => {
  registerMonthlyHandlers()
  mockCompactViewport()
  const user = userEvent.setup()
  renderPage('/budget')

  const row = await screen.findByTestId('element-cat-food')
  await user.click(within(row).getByTestId('cell-available'))
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
})

it('lets a guest open a read-only thread on a cell they cannot edit', async () => {
  mockViewport()
  const user = userEvent.setup()
  renderGuestPage('/budget')

  const row = await screen.findByTestId('element-cat-food')
  await user.click(within(row).getByTestId('comment-marker'))
  expect(await screen.findByText('Trip to Lisbon')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Post' })).toBeInTheDocument()
})

it('shows no marker on a cell without comments', async () => {
  registerMonthlyHandlers()
  mockViewport()
  renderPage('/budget')
  const row = await screen.findByTestId('element-env-1')
  expect(within(row).queryByTestId('comment-marker')).toBeNull()
})
