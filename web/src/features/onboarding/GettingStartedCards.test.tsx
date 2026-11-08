import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureOwner, fixtureUser, fixtureWireBudget } from '@/test/fixtures'
import { BudgetPage } from '@/features/budgets/BudgetPage'
import { useBudgetPeriodStore } from '@/features/budgets/budgetStore'
import { METRICS, trackEvent } from '@/lib/metrics'
import { queryKeys } from '@/app/queryKeys'

const SLOW = { timeout: 5000 }

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

const correction = (id: string, accountId: string) => ({
  id, author: fixtureOwner, type: 'income', accountId, accountRecipientId: null, amount: '100', amountRecipient: '100',
  categoryId: null, description: '', payeeId: null, tagId: null, labelIds: [], recurringId: null, date: '2026-07-01 00:00:00',
})

const userWith = (value: string | null, budget: string | null = 'b1') => ({
  ...fixtureUser,
  options: [
    ...fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: budget } : o)),
    ...(value === null ? [] : [{ name: 'getting_started', value }]),
  ],
})

let posts: unknown[]

function renderBudget(user: unknown, transactions: unknown[], path: '/budget' | '/plan' = '/budget', extra: Record<string, unknown> = {}) {
  posts = []
  server.use(
    ...coreHandlers({ user, transactions, ...extra }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    http.post('*/api/v1/user/update-getting-started', async ({ request }) => {
      const body = (await request.json()) as { value: string }
      posts.push(body)
      return HttpResponse.json({ success: true, message: '', data: { user: userWith(body.value) } })
    }),
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/budget', element: <BudgetPage key="budget" mode="budget" /> },
      { path: '/plan', element: <BudgetPage key="plan" mode="plan" /> },
    ],
    { initialEntries: [path] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return { router, queryClient }
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  useBudgetPeriodStore.setState({ lastMode: 'budget', selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null, planFolds: {}, budgetFolds: {}, planUnfoldedElements: {} })
})

it('replaces the empty budget with the cards while only opening corrections exist', async () => {
  renderBudget(userWith(''), [correction('c1', 'a1')])
  expect(await screen.findByTestId('getting-started', {}, SLOW)).toBeInTheDocument()
  expect(screen.getByText('Main budget')).toBeInTheDocument()
  expect(screen.getByText('Set your plan')).toBeInTheDocument()
  expect(screen.getByText('Bring in transactions')).toBeInTheDocument()
  expect(screen.queryByTestId('budget-totals')).not.toBeInTheDocument()
})

it('shows the normal budget once a real transaction exists', async () => {
  const { queryClient } = renderBudget(userWith(''), [correction('c1', 'a1'), { ...correction('t1', 'a1'), categoryId: 'cat-salary' }])
  expect(await screen.findByTestId('budget-totals', {}, SLOW)).toBeInTheDocument()
  await waitFor(() => expect(queryClient.getQueryData(queryKeys.transactions)).toBeDefined(), SLOW)
  expect(screen.queryByTestId('getting-started')).not.toBeInTheDocument()
})

it('shows the normal budget once every card is done or skipped', async () => {
  const { queryClient } = renderBudget(userWith('plan,!import'), [correction('c1', 'a1')])
  expect(await screen.findByTestId('budget-totals', {}, SLOW)).toBeInTheDocument()
  await waitFor(() => expect(queryClient.getQueryData(queryKeys.transactions)).toBeDefined(), SLOW)
  expect(screen.queryByTestId('getting-started')).not.toBeInTheDocument()
})

it('never shows the cards to a user outside the flow', async () => {
  const { queryClient } = renderBudget(userWith(null), [correction('c1', 'a1')])
  expect(await screen.findByTestId('budget-totals', {}, SLOW)).toBeInTheDocument()
  await waitFor(() => expect(queryClient.getQueryData(queryKeys.transactions)).toBeDefined(), SLOW)
  expect(screen.queryByTestId('getting-started')).not.toBeInTheDocument()
})

it('Skip posts the dismissed key and the card goes away', async () => {
  const user = userEvent.setup()
  renderBudget(userWith(''), [correction('c1', 'a1')])
  await screen.findByText('Set your plan', {}, SLOW)
  await user.click(screen.getAllByRole('button', { name: 'Skip' })[0]!)
  await waitFor(() => expect(posts).toEqual([{ value: '!plan' }]))
  expect(screen.queryByText('Set your plan')).not.toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith(METRICS.GETTING_STARTED_DISMISS, { step: 'plan' })
})

it('Set your plan opens the Plan view, which shows no cards', async () => {
  const user = userEvent.setup()
  const { router } = renderBudget(userWith(''), [correction('c1', 'a1')])
  await user.click(await screen.findByRole('button', { name: 'Open plan' }, SLOW))
  expect(router.state.location.pathname).toBe('/plan')
  expect(trackEvent).toHaveBeenCalledWith(METRICS.GETTING_STARTED_OPEN, { step: 'plan', surface: 'budget' })
  expect(screen.queryByTestId('getting-started')).not.toBeInTheDocument()
})

it('the import card opens the CSV import', async () => {
  const user = userEvent.setup()
  renderBudget(userWith(''), [correction('c1', 'a1')])
  await user.click(await screen.findByRole('button', { name: 'Import CSV' }, SLOW))
  expect(await screen.findByLabelText('CSV File')).toBeInTheDocument()
})

it('shows the cards below the create-budget prompt when there is no budget', async () => {
  renderBudget(userWith('', null), [correction('c1', 'a1')], '/budget', { budgets: [] })
  expect(await screen.findByTestId('budget-empty')).toBeInTheDocument()
  expect(await screen.findByTestId('getting-started', {}, SLOW)).toBeInTheDocument()
})
