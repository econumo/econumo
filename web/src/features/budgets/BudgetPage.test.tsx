import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { delay, http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { act } from 'react'
import { coreHandlers, fixtureBudgets, fixtureUser, fixtureWireBudget, fixtureWirePlan, planHandler } from '@/test/fixtures'
import { BudgetPage } from './BudgetPage'
import { HomePage } from '@/features/home/HomePage'
import { useBudgetPeriodStore } from './budgetStore'
import { METRICS, trackEvent } from '@/lib/metrics'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
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

function renderPage(initialPath: '/budget' | '/plan' = '/budget') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/budget', element: <BudgetPage key="budget" mode="budget" /> },
      { path: '/plan', element: <BudgetPage key="plan" mode="plan" /> },
      { path: '/', element: <BudgetPage key="budget" mode="budget" /> },
      { path: '/settings/budgets', element: <div>BUDGETS LIST</div> },
    ],
    { initialEntries: [initialPath] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return { queryClient, router }
}

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null, planHideEmpty: false, planFolds: {} })
})

it('renders the full budget page: strip, chips, table, totals', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
  )
  renderPage()
  expect(await screen.findByText('Main budget')).toBeInTheDocument()
  // the strip spans the full window (pre-start months stay browsable as
  // read-only history): 47 month tabs + the Budget/Plan mode toggle's 2 tabs
  expect(screen.getAllByRole('tab')).toHaveLength(49)
  expect(await screen.findByTestId('budget-folder-Essentials')).toBeInTheDocument()
  expect(screen.getByTestId('budget-totals')).toBeInTheDocument()
  // the header carries no currency chips and there is no spending widget
  expect(screen.queryByRole('button', { name: /^currency / })).not.toBeInTheDocument()
  expect(screen.queryByTestId('expense-widget')).not.toBeInTheDocument()
})

it('the cold-load spinner grows a logout escape after three seconds when the backend never answers', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', async () => {
      await delay('infinite')
      return HttpResponse.error()
    }),
  )
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    renderPage()
    expect(await screen.findByTestId('budget-loading')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Log out' })).not.toBeInTheDocument()
    await vi.advanceTimersByTimeAsync(3000)
    const link = await screen.findByRole('link', { name: 'Log out' })
    expect(link).toHaveAttribute('href', '/logout')
    expect(screen.getByText(/Having trouble\?/)).toBeInTheDocument()
    // anchored inside the loader area (not the viewport) so it centers under
    // the spinner next to the sidebar, without shifting layout
    expect(link.closest('div')?.className).toContain('absolute')
    expect(screen.getByTestId('budget-loading').className).toContain('relative')
  } finally {
    vi.useRealTimers()
  }
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
  expect(screen.getByRole('button', { name: /Done editing/ })).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Create folder' }))
  await user.type(await screen.findByLabelText('Folder name'), 'Fun')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.budgetId).toBe('b1')
  expect(body!.name).toBe('Fun')
  expect(String(body!.id)).toMatch(/^[0-9a-f-]{36}$/)
})

it('guest role: the Budget details menu item is disabled', async () => {
  const guestBudget = {
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
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: guestBudget } })),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Configure' }))
  expect(await screen.findByRole('menuitem', { name: 'Budget details' })).toHaveAttribute('aria-disabled', 'true')
  expect(screen.getByRole('menuitem', { name: 'Edit structure' })).toHaveAttribute('aria-disabled', 'true')
})

it('owner role: the Budget details menu item opens the edit dialog', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Configure' }))
  const item = await screen.findByRole('menuitem', { name: 'Budget details' })
  expect(item).not.toHaveAttribute('aria-disabled', 'true')
  await user.click(item)
  expect(await screen.findByRole('heading', { name: 'Edit budget' })).toBeInTheDocument()
})

it('deleting an empty folder asks for confirmation before posting', async () => {
  let body: Record<string, unknown> | undefined
  const budgetWithEmptyFolder = {
    ...fixtureWireBudget,
    structure: {
      ...fixtureWireBudget.structure,
      folders: [...fixtureWireBudget.structure.folders, { id: 'bf-empty', name: 'Fun', position: 1 }],
    },
  }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: budgetWithEmptyFolder } })),
    http.post('*/api/v1/budget/delete-folder', async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  await user.click(await screen.findByRole('button', { name: 'budget folder actions Fun' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete folder' }))
  // nothing posted until the confirmation is accepted
  expect(body).toBeUndefined()
  const confirm = await screen.findByRole('dialog', { name: 'Delete folder?' })
  expect(within(confirm).getByText('Are you sure you want to delete the folder “Fun”?')).toBeInTheDocument()
  await user.click(within(confirm).getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(body).toEqual({ budgetId: 'b1', id: 'bf-empty' }))
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
  await waitFor(() => expect(screen.getByRole('button', { name: 'Add account' })).toBeInTheDocument())
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

const accessDenied = () =>
  HttpResponse.json({ success: false, message: 'Access denied', code: 0, errors: [] }, { status: 403 })

it('revoked budget access shows the unavailable state with a way to pick another budget', async () => {
  server.use(
    // the revoked budget b1 is gone from the list; b2 remains
    ...coreHandlers({ user: userWithBudget, budgets: fixtureBudgets.filter((b) => b.id !== 'b1') }),
    http.get('*/api/v1/budget/get-budget', accessDenied),
  )
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByTestId('budget-unavailable')).toBeInTheDocument()
  expect(screen.queryByTestId('budget-loading')).not.toBeInTheDocument()
  expect(screen.queryByTestId('budget-empty')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Choose another budget' }))
  expect(await screen.findByText('BUDGETS LIST')).toBeInTheDocument()
})

it('a budget that starts failing after a period switch stops the placeholder loader', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget, budgets: fixtureBudgets.filter((b) => b.id !== 'b1') }),
    http.get('*/api/v1/budget/get-budget', ({ request }) =>
      new URL(request.url).searchParams.get('date') === '2026-07-01'
        ? HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })
        : accessDenied(),
    ),
  )
  renderPage()
  expect(await screen.findByText('Main budget')).toBeInTheDocument()
  // the month switch keeps the July budget as placeholder data while June 403s
  act(() => {
    useBudgetPeriodStore.setState({ selectedDate: '2026-06-01' })
  })
  expect(await screen.findByTestId('budget-unavailable')).toBeInTheDocument()
  expect(screen.queryByTestId('budget-loading')).not.toBeInTheDocument()
})

it('revoked access with no remaining budgets falls back to onboarding', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget, budgets: [] }),
    http.get('*/api/v1/budget/get-budget', accessDenied),
  )
  renderPage()
  expect(await screen.findByTestId('budget-empty')).toBeInTheDocument()
  await waitFor(() => expect(screen.getByRole('button', { name: 'Create a budget' })).toBeInTheDocument())
})

it('a server error settles into a retryable error state instead of an endless loader', async () => {
  let failing = true
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () =>
      failing
        ? HttpResponse.json({ success: false, message: 'boom', code: 0, exceptionType: 'x' }, { status: 500 })
        : HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } }),
    ),
  )
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByTestId('budget-error')).toBeInTheDocument()
  failing = false
  await user.click(screen.getByRole('button', { name: 'Try again' }))
  expect(await screen.findByText('Main budget')).toBeInTheDocument()
})

it('offers hide-empty in the settings menu only on /plan', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
  )
  const user = userEvent.setup()
  const { router } = renderPage()
  await screen.findByRole('tablist', { name: 'period' })

  await user.click(screen.getByRole('button', { name: 'Configure' }))
  expect(screen.queryByRole('menuitemcheckbox', { name: 'Hide empty rows' })).not.toBeInTheDocument()
  await user.keyboard('{Escape}')

  await act(() => router.navigate('/plan'))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  const item = await screen.findByRole('menuitemcheckbox', { name: 'Hide empty rows' })
  expect(useBudgetPeriodStore.getState().planHideEmpty).toBe(false)
  await user.click(item)
  expect(useBudgetPeriodStore.getState().planHideEmpty).toBe(true)
})

it('the header tabs navigate between /budget and /plan and reflect the route', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
  )
  const user = userEvent.setup()
  const { router } = renderPage()
  const strip = await screen.findByRole('tablist', { name: 'period' })
  // the views lead the month row, not the header
  expect(strip.parentElement).toContainElement(screen.getByRole('tablist', { name: 'budget mode' }))
  expect(screen.getByRole('heading', { name: 'Main budget' }).closest('header')).not.toContainElement(screen.getByRole('tablist', { name: 'budget mode' }))
  const modeTabs = within(screen.getByRole('tablist', { name: 'budget mode' }))
  expect(modeTabs.getByRole('tab', { name: 'Budget' })).toHaveAttribute('aria-selected', 'true')

  await user.click(modeTabs.getByRole('tab', { name: 'Plan' }))
  await screen.findByTestId('plan-sheet')
  expect(router.state.location.pathname).toBe('/plan')
  expect(within(screen.getByRole('tablist', { name: 'budget mode' })).getByRole('tab', { name: 'Plan' })).toHaveAttribute('aria-selected', 'true')

  await user.click(within(screen.getByRole('tablist', { name: 'budget mode' })).getByRole('tab', { name: 'Budget' }))
  await screen.findByRole('tablist', { name: 'period' })
  expect(router.state.location.pathname).toBe('/budget')
})

it('tablet viewport: the views lead the month row, as on desktop, and nothing is left in the settings menu', async () => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023'), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
  )
  const user = userEvent.setup()
  const { router } = renderPage()
  expect(await screen.findByText('Main budget')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  expect(screen.queryByRole('menuitemradio')).not.toBeInTheDocument()
  await user.keyboard('{Escape}')

  await user.click(within(screen.getByRole('tablist', { name: 'budget mode' })).getByRole('tab', { name: 'Plan' }))
  await screen.findByTestId('plan-sheet')
  expect(router.state.location.pathname).toBe('/plan')
  await user.click(within(screen.getByRole('tablist', { name: 'budget mode' })).getByRole('tab', { name: 'Budget' }))
  await screen.findByRole('tablist', { name: 'period' })
  expect(router.state.location.pathname).toBe('/budget')
})

it('the route hop remounts the page: edit structure started on /plan is off again on /budget', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
  )
  const user = userEvent.setup()
  const { router } = renderPage('/plan')
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  expect(screen.getByRole('button', { name: 'Done editing' })).toBeInTheDocument()

  await act(() => router.navigate('/budget'))
  await screen.findByRole('tablist', { name: 'period' })
  expect(screen.queryByRole('button', { name: 'Done editing' })).not.toBeInTheDocument()
})

it('opening a view remembers it for the main menu link', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
  )
  const { router } = renderPage('/plan')
  await screen.findByTestId('plan-sheet')
  expect(useBudgetPeriodStore.getState().lastMode).toBe('plan')
  await act(() => router.navigate('/budget'))
  await screen.findByRole('tablist', { name: 'period' })
  expect(useBudgetPeriodStore.getState().lastMode).toBe('budget')
})

it('rendering /plan fires BUDGET_PLAN_OPEN once; /budget does not', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
  )
  const { router } = renderPage()
  await screen.findByRole('tablist', { name: 'period' })
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.BUDGET_PLAN_OPEN)

  await act(() => router.navigate('/plan'))
  await screen.findByTestId('plan-sheet')
  expect(vi.mocked(trackEvent).mock.calls.filter(([k]) => k === METRICS.BUDGET_PLAN_OPEN)).toHaveLength(1)
})

it('offers change currency on every element, and move to folder', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
  )
  const user = userEvent.setup()
  renderPage()
  await screen.findByRole('tablist', { name: 'period' })
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))

  // an ENVELOPE previously had no Change currency item — only Edit/Delete
  await user.click(await screen.findByRole('button', { name: 'element actions Living' }))
  expect(await screen.findByRole('menuitem', { name: 'Change currency' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Move to folder…' })).toBeInTheDocument()
  expect(screen.getByRole('menuitem', { name: 'Edit' })).toBeInTheDocument()
})

it('an archived budget shows the banner and blocks structure editing', async () => {
  const archivedWire = { ...fixtureWireBudget, meta: { ...fixtureWireBudget.meta, isArchived: 1 } }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: archivedWire } })),
  )
  const user = userEvent.setup()
  renderPage()

  expect(await screen.findByText('This budget is archived and read-only')).toBeInTheDocument()

  // the configure menu's editing entries are disabled — archived wins over role
  await user.click(await screen.findByRole('button', { name: 'Configure' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit structure' })).toHaveAttribute('aria-disabled', 'true')
})

it('a live budget shows no archived banner', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
  )
  renderPage()
  expect(await screen.findByRole('button', { name: 'Configure' })).toBeInTheDocument()
  expect(screen.queryByText('This budget is archived and read-only')).not.toBeInTheDocument()
})

const budgetWithSavings = () => {
  const budget = JSON.parse(JSON.stringify(fixtureWireBudget))
  budget.structure.savings = [
    { id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0,
      budgeted: '100', spent: '120', available: '-20', closingBalance: '2500' },
  ]
  return budget
}

it('the Budget view follows the phone order: Income, Savings, Expenses, then the totals lines', async () => {
  const budget = budgetWithSavings()
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: budget } })),
    planHandler(),
  )
  renderPage()
  const income = await screen.findByTestId('month-income')
  const savings = screen.getByTestId('month-savings')
  const expenses = screen.getByTestId('budget-table')
  expect(income.compareDocumentPosition(savings) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(savings.compareDocumentPosition(expenses) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

  // July of the plan fixture: Salaries planned 2000, nothing in; Freelance 500 planned, 400 in
  await waitFor(() => expect(within(income).getByTestId('month-income-row-cat-freelance')).toHaveTextContent('400.00'))
  // the third column: what each source is still expected to bring
  expect(within(within(income).getByTestId('month-income-row-cat-freelance')).getByTestId('flow-third')).toHaveTextContent(/^100\.00$/)
  expect(within(within(income).getByTestId('month-income-row-ie1')).getByTestId('flow-third')).toHaveTextContent(/^2,000\.00$/)
  expect(screen.getByTestId('month-income-header')).toHaveTextContent('To receive')
  expect(within(income).getByTestId('month-income-row-ie1')).toHaveTextContent('2,000.00')
  const savingsRow = within(savings).getByTestId('month-savings-row-acc-s1')
  expect(savingsRow).toHaveTextContent('100.00')
  expect(savingsRow).toHaveTextContent('120.00')
  expect(savingsRow).toHaveTextContent('2,500.00')

  // the Total row stays expenses only (BudgetTable.test: 300.00 / 45.50 / 554.50)
  const totals = screen.getByTestId('budget-totals')
  await waitFor(() => expect(totals).toHaveTextContent('554.50'))
  expect(totals).not.toHaveTextContent('165.50')
  expect(screen.getByTestId('month-total-income')).toHaveTextContent('400.00')
  expect(screen.getByTestId('month-total-expenses')).toHaveTextContent('45.50')
  expect(screen.getByTestId('month-total-savings')).toHaveTextContent('120.00')
  expect(screen.queryByTestId('month-total-transfers')).toBeNull()
  expect(screen.getByTestId('month-total-balance')).toBeInTheDocument()
})

it('the Budget view groups income like the Plan grid: folders with sums, No folder, envelopes unfold to categories', async () => {
  const plan = JSON.parse(JSON.stringify(fixtureWirePlan))
  plan.structure.folders = [...plan.structure.folders, { id: 'bf-inc', name: 'Side gigs', position: 1 }]
  plan.structure.elements = plan.structure.elements.map((el: { id: string }) => (el.id === 'cat-freelance' ? { ...el, folderId: 'bf-inc' } : el))
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(plan),
  )
  const user = userEvent.setup()
  renderPage()
  const folder = await screen.findByTestId('month-income-folder-bf-inc')
  expect(within(folder).getByText('Side gigs')).toBeInTheDocument()
  // July: Freelance planned 500, received 400 — the folder line sums its rows
  expect(within(folder).getByRole('banner')).toHaveTextContent('500.00')
  expect(within(folder).getByRole('banner')).toHaveTextContent('400.00')
  expect(within(folder).getByTestId('month-income-row-cat-freelance')).toBeInTheDocument()
  const loose = screen.getByTestId('month-income-folder-__no_folder__')
  expect(within(loose).getByText('No folder')).toBeInTheDocument()
  expect(within(loose).getByTestId('month-income-row-ie1')).toBeInTheDocument()
  expect(folder.compareDocumentPosition(loose) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

  // the Salaries envelope unfolds to its Salary category
  expect(screen.queryByTestId('month-income-child-cat-salary')).toBeNull()
  await user.click(within(loose).getByRole('button', { name: 'Salaries' }))
  expect(await screen.findByTestId('month-income-child-cat-salary')).toBeInTheDocument()
})

it('an income row with no plan reads "—" under To receive, even when money came in', async () => {
  const plan = JSON.parse(JSON.stringify(fixtureWirePlan))
  // Freelance in July: no plan, 400 in
  plan.structure.elements.find((el: { id: string }) => el.id === 'cat-freelance').cells[2] = { actual: '400', planned: '' }
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(plan),
  )
  renderPage()
  const row = await screen.findByTestId('month-income-row-cat-freelance')
  expect(within(row).getByTestId('flow-third')).toHaveTextContent(/^—$/)
  expect(within(within(row).getByTestId('flow-third')).getByText('—')).toHaveClass('text-muted-foreground/50')
  // Salaries still expects its 2,000.00
  expect(within(screen.getByTestId('month-income-row-ie1')).getByTestId('flow-third')).toHaveTextContent(/^2,000\.00$/)
})

it('a Budget view section header folds its section, shows its sums, and shares the fold with the Plan view', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: budgetWithSavings() } })),
    planHandler(),
  )
  const user = userEvent.setup()
  renderPage()
  const header = await screen.findByTestId('month-income-header')
  await screen.findByTestId('month-income-row-cat-freelance')
  expect(header).toHaveTextContent('Planned')

  await user.click(within(header).getByRole('button', { name: 'Income' }))
  expect(screen.queryByTestId('month-income-row-cat-freelance')).toBeNull()
  expect(useBudgetPeriodStore.getState().planFolds.income).toBe(true)
  // folded: the section's sums take the column headings' place
  expect(screen.getByTestId('month-income-header')).toHaveTextContent('2,500.00')
  expect(screen.getByTestId('month-income-header')).not.toHaveTextContent('Planned')

  await user.click(within(screen.getByTestId('column-headers')).getByRole('button', { name: 'Expenses' }))
  expect(screen.queryByTestId('budget-folder-Essentials')).toBeNull()
  expect(useBudgetPeriodStore.getState().planFolds.expense).toBe(true)
})

