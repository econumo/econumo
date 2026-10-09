import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { delay, http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { act } from 'react'
import { coreHandlers, fixtureBudgets, fixtureCategories, fixtureUser, fixtureWireBudget, fixtureWirePlan, planHandler } from '@/test/fixtures'
import { BudgetPage } from './BudgetPage'
import { HomePage } from '@/features/home/HomePage'
import { useBudgetPeriodStore } from './budgetStore'
import { queryKeys } from '@/app/queryKeys'
import { useUiStore } from '@/app/uiStore'
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
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null, planFolds: {}, budgetFolds: {}, planUnfoldedElements: {} })
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

it('Create folder from the Expenses line posts with a v7 id', async () => {
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
  await user.click(within(await screen.findByTestId('column-headers')).getByRole('button', { name: 'menu Expenses' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Create folder' }))
  await user.type(await screen.findByLabelText('Folder name'), 'Fun')
  await user.click(screen.getByRole('button', { name: 'Create' }))
  await waitFor(() => expect(body).toBeDefined())
  expect(body!.budgetId).toBe('b1')
  expect(body!.name).toBe('Fun')
  expect(body!.side).toBe('expense')
  expect(String(body!.id)).toMatch(/^[0-9a-f-]{36}$/)
})

it('guest role: no Configure button on the desktop Budget view (nothing a guest may configure)', async () => {
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
  renderPage()
  await screen.findByRole('tablist', { name: 'period' })
  expect(screen.queryByRole('button', { name: 'Configure' })).toBeNull()
})

it('owner role: Configure opens the budget details dialog', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Configure' }))
  expect(await screen.findByRole('heading', { name: 'Budget settings' })).toBeInTheDocument()
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
  await user.click(within(await screen.findByTestId('budget-folder-Fun')).getByRole('button', { name: 'menu Fun' }))
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

it('Configure opens Budget settings at once on the desktop, in the Budget and Plan views alike', async () => {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
  )
  const user = userEvent.setup()
  const { router } = renderPage()
  await screen.findByRole('tablist', { name: 'period' })

  // both views edit on hover, so there is nothing to choose: the details open
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  expect(await screen.findByRole('dialog', { name: 'Budget settings' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitem')).toBeNull()
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Budget settings' })).not.toBeInTheDocument())

  await act(() => router.navigate('/plan'))
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  expect(await screen.findByRole('dialog', { name: 'Budget settings' })).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Edit structure' })).not.toBeInTheDocument()
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

it('tablet viewport: no ⋮ menus or grips until Edit structure is on; then on every line; Done hides them', async () => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023'), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(),
  )
  const user = userEvent.setup()
  renderPage()
  const food = await screen.findByTestId('element-cat-food')
  expect(within(food).queryByRole('button', { name: 'menu Food' })).toBeNull()
  expect(screen.queryByRole('button', { name: /^move / })).toBeNull()

  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('button', { name: 'Edit structure' }))
  const menu = within(screen.getByTestId('element-cat-food')).getByRole('button', { name: 'menu Food' })
  expect(menu.className).not.toContain('opacity-0')
  expect(screen.getByRole('button', { name: 'move cat-food' }).className).not.toContain('opacity-0')

  await user.click(screen.getByRole('button', { name: /Done editing/ }))
  expect(within(screen.getByTestId('element-cat-food')).queryByRole('button', { name: 'menu Food' })).toBeNull()
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
  // only a touch screen has an edit mode to start
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023'), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  const user = userEvent.setup()
  const { router } = renderPage('/plan')
  await screen.findByTestId('plan-sheet')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('button', { name: 'Edit structure' }))
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

  // an ENVELOPE previously had no Change currency item — only Edit/Delete
  await user.click(within(await screen.findByTestId('element-env-1')).getByRole('button', { name: 'menu Living' }))
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

  // archived wins over role: no structure actions, no grips, no Create folder
  const living = await screen.findByTestId('element-env-1')
  await user.click(within(living).getByRole('button', { name: 'menu Living' }))
  const items = (await screen.findAllByRole('menuitem')).map((i) => i.textContent)
  expect(items).not.toContain('Change currency')
  expect(items).not.toContain('Move to folder…')
  expect(screen.queryByRole('button', { name: /^move / })).toBeNull()
  expect(within(screen.getByTestId('column-headers')).queryByRole('button', { name: 'menu Expenses' })).toBeNull()
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
  expect(useBudgetPeriodStore.getState().budgetFolds.income).toBe(true)
  // folded: the section's sums take the column headings' place
  expect(screen.getByTestId('month-income-header')).toHaveTextContent('2,500.00')
  expect(screen.getByTestId('month-income-header')).not.toHaveTextContent('Planned')

  await user.click(within(screen.getByTestId('column-headers')).getByRole('button', { name: 'Expenses' }))
  expect(screen.queryByTestId('budget-folder-Essentials')).toBeNull()
  expect(useBudgetPeriodStore.getState().budgetFolds.expense).toBe(true)
})


describe('the ⋮ menus on the Budget view', () => {
  const plainHandlers = (budget: unknown = fixtureWireBudget) => [
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: budget } })),
    planHandler(),
  ]

  it('an expense row offers Edit, Change currency, Move to folder, and the category\'s Archive, Merge and Delete; its transactions open from Spent, not the menu', async () => {
    server.use(...plainHandlers())
    const user = userEvent.setup()
    const { queryClient } = renderPage()
    const food = await screen.findByTestId('element-cat-food')
    // the category list says who owns Food
    await waitFor(() => expect(queryClient.getQueryData(queryKeys.categories)).toBeTruthy())
    await user.click(within(food).getByRole('button', { name: 'menu Food' }))
    const items = (await screen.findAllByRole('menuitem')).map((i) => i.textContent)
    expect(items).toEqual(['Edit', 'Change currency', 'Move to folder…', 'Archive', 'Merge into…', 'Delete'])
  })

  it('a greyed-out action says why: Edit on another member\'s category, Delete on a folder with items', async () => {
    const budget = JSON.parse(JSON.stringify(fixtureWireBudget))
    budget.structure.elements.find((el: { id: string }) => el.id === 'cat-food').ownerUserId = 'u9'
    server.use(...plainHandlers(budget))
    const user = userEvent.setup()
    renderPage()
    const food = await screen.findByTestId('element-cat-food')
    await user.click(within(food).getByRole('button', { name: 'menu Food' }))
    const item = (text: string) => async () => (await screen.findAllByRole('menuitem')).find((i) => i.textContent === text)
    expect(await item('Edit (no access)')()).toHaveAttribute('aria-disabled', 'true')
    await user.keyboard('{Escape}')

    await user.click(within(screen.getByTestId('budget-folder-Essentials')).getByRole('button', { name: 'menu Essentials' }))
    expect(await item('Delete folder (not empty)')()).toHaveAttribute('aria-disabled', 'true')
  })

  it('a guest gets no structure actions: only Edit, and Archive, Merge and Delete, on what they own', async () => {
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
    server.use(...plainHandlers(guestBudget))
    const user = userEvent.setup()
    const { queryClient } = renderPage()
    const food = await screen.findByTestId('element-cat-food')
    await waitFor(() => expect(queryClient.getQueryData(queryKeys.categories)).toBeTruthy())
    await user.click(within(food).getByRole('button', { name: 'menu Food' }))
    const items = (await screen.findAllByRole('menuitem')).map((i) => i.textContent)
    // a category is personal: its owner manages it in any budget
    expect(items).toEqual(['Edit', 'Archive', 'Merge into…', 'Delete'])
    // and the Expenses line has no Create folder for a guest
    expect(within(screen.getByTestId('column-headers')).queryByRole('button', { name: 'menu Expenses' })).toBeNull()
  })

  it('the Income menus offer New envelope, and it creates an income envelope in that folder', async () => {
    const plan = JSON.parse(JSON.stringify(fixtureWirePlan))
    plan.structure.folders = [...plan.structure.folders, { id: 'bf-inc', name: 'Side gigs', position: 1, side: 'income' }]
    plan.structure.elements = plan.structure.elements.map((el: { id: string }) => (el.id === 'cat-freelance' ? { ...el, folderId: 'bf-inc' } : el))
    let sent: { folderId: string | null; side?: string; name: string } | null = null
    server.use(
      ...coreHandlers({ user: userWithBudget }),
      http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
      planHandler(plan),
      http.post('*/api/v1/budget/create-envelope', async ({ request }) => {
        sent = (await request.json()) as typeof sent
        return HttpResponse.json({ success: true, message: '', data: { item: { id: 'env-new' } } })
      }),
    )
    const user = userEvent.setup()
    renderPage()
    const items = async () => (await screen.findAllByRole('menuitem')).map((i) => i.textContent)

    await user.click(within(await screen.findByTestId('month-income-header')).getByRole('button', { name: 'menu Income' }))
    expect(await items()).toEqual(['Create folder', 'New envelope', 'New category'])
    await user.keyboard('{Escape}')
    await user.click(within(screen.getByTestId('month-income-folder-__no_folder__')).getByRole('button', { name: 'menu No folder' }))
    expect(await items()).toEqual(['New envelope', 'New category'])
    await user.keyboard('{Escape}')

    await user.click(within(screen.getByTestId('month-income-folder-bf-inc')).getByRole('button', { name: 'menu Side gigs' }))
    await user.click(await screen.findByRole('menuitem', { name: 'New envelope' }))
    await user.type(await screen.findByRole('textbox', { name: 'Name' }), 'Jobs')
    await user.click(screen.getByRole('button', { name: 'Create' }))
    await waitFor(() => expect(sent).toMatchObject({ name: 'Jobs', folderId: 'bf-inc', side: 'income' }))
  })

  it('a new expense envelope sends no side', async () => {
    let sent: Record<string, unknown> | null = null
    server.use(
      ...coreHandlers({ user: userWithBudget }),
      http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
      planHandler(fixtureWirePlan),
      http.post('*/api/v1/budget/create-envelope', async ({ request }) => {
        sent = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({ success: true, message: '', data: { item: { id: 'env-new' } } })
      }),
    )
    const user = userEvent.setup()
    renderPage()
    await user.click(within(await screen.findByTestId('column-headers')).getByRole('button', { name: 'menu Expenses' }))
    await user.click(await screen.findByRole('menuitem', { name: 'New envelope' }))
    await user.type(await screen.findByRole('textbox', { name: 'Name' }), 'Home')
    await user.click(screen.getByRole('button', { name: 'Create' }))
    await waitFor(() => expect(sent).toMatchObject({ name: 'Home', folderId: null }))
    expect(sent).not.toHaveProperty('side')
  })

  it('Create folder from Income creates an income folder, listed under Income while still empty', async () => {
    let created: { id: string; name: string; side?: string } | null = null
    const folderWire = () => (created ? [{ id: created.id, name: created.name, position: 9, side: created.side }] : [])
    server.use(
      ...coreHandlers({ user: userWithBudget }),
      // get-budget leaves income folders out; the plan reports every folder with its side
      http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
      http.get('*/api/v1/budget/get-budget-plan', () => {
        const plan = JSON.parse(JSON.stringify(fixtureWirePlan))
        plan.structure.folders = [...plan.structure.folders, ...folderWire()]
        return HttpResponse.json({ success: true, message: '', data: { item: plan } })
      }),
      http.post('*/api/v1/budget/create-folder', async ({ request }) => {
        const body = (await request.json()) as { id: string; name: string; side?: string }
        created = { id: body.id, name: body.name, side: body.side }
        return HttpResponse.json({ success: true, message: '', data: { item: folderWire()[0] } })
      }),
    )
    const user = userEvent.setup()
    renderPage()
    const header = await screen.findByTestId('month-income-header')
    await user.click(within(header).getByRole('button', { name: 'menu Income' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Create folder' }))
    await user.type(await screen.findByRole('textbox', { name: 'Folder name' }), 'Side gigs')
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => expect(created?.side).toBe('income'))
    const folder = await screen.findByTestId(`month-income-folder-${created!.id}`)
    expect(folder).toHaveTextContent('Side gigs')
    expect(screen.queryByTestId('budget-folder-Side gigs')).toBeNull()

    // an income row may move there; the expense folders are not offered
    await user.click(within(screen.getByTestId('month-income-row-cat-freelance')).getByRole('button', { name: 'menu Freelance' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Move to folder…' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByRole('button', { name: 'Side gigs' })).toBeInTheDocument()
    expect(within(dialog).queryByRole('button', { name: 'Essentials' })).toBeNull()
  })
})

describe('managing categories, tags and accounts from the ⋮ menus', () => {
  const setup = (budget: unknown = fixtureWireBudget) => {
    const calls: { path: string; body: Record<string, unknown> }[] = []
    const record = (path: string) =>
      http.post(`*${path}`, async ({ request }) => {
        calls.push({ path, body: (await request.json()) as Record<string, unknown> })
        return HttpResponse.json({ success: true, message: '', data: path.endsWith('create-category') ? { item: { id: 'cat-new', ownerUserId: 'u1', name: 'Gym', position: 9, type: 'expense', icon: 'x', isArchived: 0, createdAt: '', updatedAt: '' } } : {} })
      })
    server.use(
      ...coreHandlers({ user: userWithBudget }),
      http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: budget } })),
      planHandler(fixtureWirePlan),
      record('/api/v1/category/create-category'),
      record('/api/v1/category/archive-category'),
      record('/api/v1/category/delete-category'),
      record('/api/v1/budget/move-element'),
      record('/api/v1/budget/add-account'),
    )
    return calls
  }

  it('New category in a folder creates an expense category there', async () => {
    const calls = setup()
    const user = userEvent.setup()
    renderPage()
    await user.click(within(await screen.findByTestId('budget-folder-Essentials')).getByRole('button', { name: 'menu Essentials' }))
    await user.click(await screen.findByRole('menuitem', { name: 'New category' }))
    // the type is the section's: no income/expense switch
    expect(screen.queryByRole('radiogroup', { name: 'type' })).toBeNull()
    await user.type(screen.getByRole('textbox', { name: 'Name' }), 'Gym')
    await user.click(screen.getByRole('button', { name: 'Create' }))
    await waitFor(() => expect(calls.map((c) => c.path)).toEqual(['/api/v1/category/create-category', '/api/v1/budget/move-element']))
    expect(calls[0].body).toMatchObject({ name: 'Gym', type: 'expense' })
    expect(calls[1].body).toMatchObject({ budgetId: 'b1', id: 'cat-new', folderId: 'bf1', afterId: null })
  })

  it('Archive and Delete act on the category behind the row', async () => {
    const calls = setup()
    const user = userEvent.setup()
    const { queryClient } = renderPage()
    const food = await screen.findByTestId('element-cat-food')
    await waitFor(() => expect(queryClient.getQueryData(queryKeys.categories)).toBeTruthy())
    await user.click(within(food).getByRole('button', { name: 'menu Food' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Archive' }))
    await waitFor(() => expect(calls.map((c) => c.path)).toContain('/api/v1/category/archive-category'))
    await user.click(within(food).getByRole('button', { name: 'menu Food' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
    await user.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Delete' }))
    await waitFor(() => expect(calls.find((c) => c.path === '/api/v1/category/delete-category')?.body).toMatchObject({ id: 'cat-food' }))
  })

  it('another member\'s category shows the actions greyed out, saying why', async () => {
    const budget = JSON.parse(JSON.stringify(fixtureWireBudget))
    budget.structure.elements.find((el: { id: string }) => el.id === 'cat-food').ownerUserId = 'u9'
    setup(budget)
    server.use(
      http.get('*/api/v1/category/get-category-list', () =>
        HttpResponse.json({ success: true, message: '', data: { items: fixtureCategories.map((c) => (c.id === 'cat-food' ? { ...c, ownerUserId: 'u9' } : c)) } }),
      ),
    )
    const user = userEvent.setup()
    const { queryClient } = renderPage()
    const food = await screen.findByTestId('element-cat-food')
    await waitFor(() => expect(queryClient.getQueryData(queryKeys.categories)).toBeTruthy())
    await user.click(within(food).getByRole('button', { name: 'menu Food' }))
    const texts = (await screen.findAllByRole('menuitem')).map((i) => i.textContent)
    expect(texts).toEqual(expect.arrayContaining(['Archive (no access)', 'Merge into… (no access)', 'Delete (no access)']))
  })

  it('a category inside an envelope has its own menu', async () => {
    setup()
    useBudgetPeriodStore.setState({ unfoldedElements: { 'env-1': true }, foldBudgetId: 'b1' })
    const user = userEvent.setup()
    renderPage()
    const rent = await screen.findByTestId('child-cat-rent')
    await user.click(within(rent).getByRole('button', { name: 'menu Rent' }))
    const items = (await screen.findAllByRole('menuitem')).map((i) => i.textContent)
    expect(items[0]).toBe('Edit')
    expect(items.some((i) => i?.startsWith('Merge into…'))).toBe(true)
  })

  it('New account on the Savings line opens the account dialog, and the new account joins as savings', async () => {
    const budget = JSON.parse(JSON.stringify(fixtureWireBudget))
    budget.structure.savings = [{ id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0, budgeted: '0', spent: '0', available: '0' }]
    const calls = setup(budget)
    const user = userEvent.setup()
    renderPage()
    await user.click(within(await screen.findByTestId('month-savings')).getByRole('button', { name: 'menu Savings' }))
    await user.click(await screen.findByRole('menuitem', { name: 'New account' }))
    const params = useUiStore.getState().accountModal
    expect(params?.account).toBeUndefined()
    act(() => params!.onCreated!({ id: 'acc-new' } as never))
    await waitFor(() => expect(calls.find((c) => c.path === '/api/v1/budget/add-account')?.body).toEqual({ id: 'b1', accountId: 'acc-new', isSavings: true }))
  })
})
