import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget } from '@/test/fixtures'
import { BudgetPage } from './BudgetPage'
import { HomePage } from '@/features/home/HomePage'
import { useBudgetPeriodStore } from './budgetStore'

const userWithBudget = {
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b1' } : o)),
}

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage(element = <BudgetPage />) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/budget', element },
      { path: '/', element },
      { path: '/settings/budgets', element: <div>BUDGETS LIST</div> },
    ],
    { initialEntries: ['/budget'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null })
})

it('renders the full budget page: strip, chips, table, totals', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
  )
  renderPage()
  expect(await screen.findByText('Main budget')).toBeInTheDocument()
  expect(screen.getAllByRole('tab')).toHaveLength(47)
  expect(await screen.findByTestId('budget-folder-Essentials')).toBeInTheDocument()
  expect(screen.getByTestId('budget-totals')).toBeInTheDocument()
  // currency chips from balances
  expect(screen.getByRole('button', { name: 'currency USD' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'currency EUR' })).toBeInTheDocument()
  // widget hidden until a chip is selected
  expect(screen.queryByTestId('expense-widget')).not.toBeInTheDocument()
})

it('toggling a currency chip mounts the expense widget', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'currency USD' }))
  expect(await screen.findByTestId('expense-widget')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'currency USD' }))
  expect(screen.queryByTestId('expense-widget')).not.toBeInTheDocument()
})

it('configure menu enters edit mode; folder create posts with a v7 id', async () => {
  let body: Record<string, unknown> | undefined
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    http.post('*/api/v1/budget/create-folder', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: { item: { id: 'bf-new', name: 'Fun', position: 0 } } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  expect(screen.getByRole('button', { name: /Done Editing/ })).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Create a folder' }))
  await user.type(await screen.findByLabelText('Folder name'), 'Fun')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.budgetId).toBe('b1')
  expect(body!.name).toBe('Fun')
  expect(String(body!.id)).toMatch(/^[0-9a-f-]{36}$/)
})

it('inline limit editor commits a formula as a normalized string', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  const food = await screen.findByTestId('element-cat-food')
  await user.click(within(food).getByRole('button', { name: 'limit Food' }))
  const input = await screen.findByLabelText('Budget')
  await user.clear(input)
  await user.type(input, '100+50')
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(body).toEqual({ budgetId: 'b1', elementId: 'cat-food', period: '2026-07-01', amount: '150' }))
})

it('empty state: no default budget shows create-budget when accounts+categories exist', async () => {
  server.use(...coreHandlers())
  renderPage()
  expect(await screen.findByTestId('budget-empty')).toBeInTheDocument()
  expect(screen.getByText('You haven’t created a budget yet.')).toBeInTheDocument()
  await waitFor(() => expect(screen.getByRole('button', { name: 'Create a budget' })).toBeInTheDocument())
})

it('empty state: no accounts shows the initial-setup prompt', async () => {
  server.use(...coreHandlers({ accounts: [] }))
  renderPage()
  expect(await screen.findByTestId('budget-empty')).toBeInTheDocument()
  await waitFor(() => expect(screen.getByRole('button', { name: 'Create an account' })).toBeInTheDocument())
})

it('/ renders the budget for an onboarded user with a default budget', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter([{ path: '/', element: <HomePage /> }], { initialEntries: ['/'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  expect(await screen.findByText('Main budget')).toBeInTheDocument()
})
