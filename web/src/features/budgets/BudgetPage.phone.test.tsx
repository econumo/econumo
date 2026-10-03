import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent, { PointerEventsCheckLevel } from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { delay, http, HttpResponse } from 'msw'
import type { HttpHandler } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureUser, fixtureWireBudget, fixtureWirePlan, planHandler } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import { queryKeys } from '@/app/queryKeys'
import { BudgetPage } from './BudgetPage'
import { useBudgetPeriodStore } from './budgetStore'
import { addMonths } from './planMath'

const userWithBudget = {
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b1' } : o)),
}
const author = { id: 'u1', avatar: 'face:emerald', name: 'Ada' }
const comment = {
  id: 'c1', elementId: 'cat-food', period: '2026-07-01', comment: 'Trip to Lisbon',
  author, createdAt: '2026-07-17 09:00:00', updatedAt: '2026-07-17 09:00:00',
}
const incomeComment = { ...comment, id: 'c2', elementId: 'cat-freelance', comment: 'Invoice sent' }
const savingsComment = { ...comment, id: 'c3', elementId: 'acc-s1', comment: 'Topped up' }
const guestBudget = {
  ...fixtureWireBudget,
  meta: {
    ...fixtureWireBudget.meta,
    ownerUserId: 'u9',
    access: [
      { user: { id: 'u9', avatar: 'face:sky', name: 'Owner' }, role: 'owner', isAccepted: 1 },
      { user: author, role: 'guest', isAccepted: 1 },
    ],
  },
}
const savingsBudget = {
  ...fixtureWireBudget,
  structure: {
    ...fixtureWireBudget.structure,
    savings: [
      {
        id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0,
        budgeted: '100', spent: '40', available: '60',
      },
    ],
  },
}

function phone() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: true, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function handlers({ budget = fixtureWireBudget, plan = planHandler(), accounts = fixtureAccounts }: { budget?: unknown; plan?: HttpHandler; accounts?: unknown[] } = {}) {
  let setLimitBody: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget, accounts }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: budget } })),
    plan,
    http.get('*/api/v1/budget/get-comment-list', () =>
      HttpResponse.json({ success: true, message: '', data: { items: [comment, incomeComment, savingsComment], truncated: false } }),
    ),
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      setLimitBody = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  return { setLimitBody: () => setLimitBody }
}

function renderPage(path: '/budget' | '/budget/plan' = '/budget') {
  const router = createMemoryRouter(
    [
      { path: '/budget', element: <BudgetPage key="budget" mode="budget" /> },
      { path: '/budget/plan', element: <BudgetPage key="plan" mode="plan" /> },
    ],
    { initialEntries: [path] },
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return { queryClient }
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  phone()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null, planHideEmpty: false })
})

it('renders the single month view on /budget and on /budget/plan alike', async () => {
  handlers()
  renderPage('/budget')
  expect(await screen.findByTestId('phone-month-view')).toBeInTheDocument()
  expect(screen.queryByTestId('budget-table')).toBeNull()
})

it('/budget/plan on a phone is the same month view, not the plan grid', async () => {
  handlers()
  renderPage('/budget/plan')
  expect(await screen.findByTestId('phone-month-view')).toBeInTheDocument()
  expect(screen.queryByTestId('plan-sheet')).toBeNull()
})

it('has no Month/Plan switch anywhere, and the title is not all caps', async () => {
  handlers()
  const user = userEvent.setup()
  renderPage()
  const title = await screen.findByRole('heading', { name: 'Main budget' })
  expect(title.className).not.toContain('uppercase')
  expect(screen.queryByRole('tablist', { name: 'budget mode' })).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit structure' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitemradio')).toBeNull()
})

it('/budget/plan on a phone has no Hide empty rows toggle', async () => {
  handlers()
  const user = userEvent.setup()
  renderPage('/budget/plan')
  await screen.findByTestId('phone-month-view')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit structure' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitemcheckbox')).toBeNull()
})

it('row tap → sheet → Set budget replaces the sheet and saves the selected month', async () => {
  const api = handlers()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByRole('button', { name: /^Food, budget 200.00/ }))
  const sheet = await screen.findByTestId('element-sheet')
  await user.click(within(sheet).getByRole('button', { name: 'Set budget' }))
  const input = await screen.findByLabelText('Budget')
  expect(screen.queryByTestId('element-sheet')).toBeNull()
  await user.clear(input)
  await user.type(input, '250')
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(api.setLimitBody()).toEqual({ budgetId: 'b1', elementId: 'cat-food', period: '2026-07-01', amount: '250' }))
  await waitFor(() => expect(screen.queryByLabelText('Budget')).toBeNull())
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

it('sheet → Comments opens the thread as a sheet', async () => {
  handlers()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByRole('button', { name: /^Food, budget/ }))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Comments (1)' }))
  expect(await screen.findByTestId('comments-sheet')).toHaveTextContent('Trip to Lisbon')
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

it('sheet → Transactions opens the month’s transaction list', async () => {
  handlers()
  server.use(http.get('*/api/v1/budget/get-transaction-list', () => HttpResponse.json({ success: true, message: '', data: { items: [] } })))
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByRole('button', { name: /^Food, budget/ }))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Transactions' }))
  await waitFor(() => expect(screen.queryByTestId('element-sheet')).toBeNull())
  expect(await screen.findByRole('dialog', { name: /Food/ })).toBeInTheDocument()
})

function captureTxListParams() {
  let params: URLSearchParams | undefined
  server.use(
    http.get('*/api/v1/budget/get-transaction-list', ({ request }) => {
      params = new URL(request.url).searchParams
      return HttpResponse.json({ success: true, message: '', data: { items: [] } })
    }),
  )
  return () => params
}

it('a savings row’s sheet → Transactions lists every transaction on the account that month', async () => {
  handlers({ budget: savingsBudget })
  const params = captureTxListParams()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByTestId('phone-savings-summary'))
  await user.click(await screen.findByRole('button', { name: /^Rainy day, planned/ }))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Transactions' }))
  expect(await screen.findByRole('dialog', { name: /Rainy day/ })).toBeInTheDocument()
  await waitFor(() => expect(params()?.get('accountId')).toBe('acc-s1'))
  expect(params()?.get('periodStart')).toBe('2026-07-01')
})

it('an income row’s sheet → Transactions lists the category’s income that month', async () => {
  handlers()
  const params = captureTxListParams()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByTestId('phone-income-summary'))
  await user.click(await screen.findByRole('button', { name: /^Freelance, planned/ }))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Transactions' }))
  expect(await screen.findByRole('dialog', { name: /Freelance/ })).toBeInTheDocument()
  await waitFor(() => expect(params()?.get('income')).toBe('1'))
  expect(params()?.get('categoryId')).toBe('cat-freelance')
})

it('a guest’s sheet has no Set budget but still reaches comments', async () => {
  handlers({ budget: guestBudget })
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByRole('button', { name: /^Food, budget/ }))
  const sheet = await screen.findByTestId('element-sheet')
  expect(within(sheet).queryByRole('button', { name: 'Set budget' })).toBeNull()
  expect(within(sheet).getByRole('button', { name: 'Comments (1)' })).toBeInTheDocument()
})

it('an income row’s Set plan writes that month’s plan', async () => {
  const api = handlers()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByTestId('phone-income-summary'))
  await user.click(await screen.findByRole('button', { name: /^Freelance, planned 500.00/ }))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Set plan' }))
  expect(await screen.findByRole('dialog', { name: /Set plan/ })).toBeInTheDocument()
  expect(screen.queryByTestId('element-sheet')).toBeNull()
  const input = screen.getByLabelText('Plan')
  await user.clear(input)
  await user.type(input, '650')
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(api.setLimitBody()).toEqual({ budgetId: 'b1', elementId: 'cat-freelance', period: '2026-07-01', amount: '650' }))
  await waitFor(() => expect(screen.queryByRole('dialog', { name: /Set plan/ })).toBeNull())
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

it('an income row’s sheet reaches its comment thread', async () => {
  handlers()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByTestId('phone-income-summary'))
  await user.click(await screen.findByRole('button', { name: /^Freelance, planned/ }))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Comments (1)' }))
  expect(await screen.findByTestId('comments-sheet')).toHaveTextContent('Invoice sent')
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

it('a savings row’s sheet sets its plan and reaches its comment thread', async () => {
  const api = handlers({ budget: savingsBudget })
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  // the savings section starts folded
  await user.click(await screen.findByTestId('phone-savings-summary'))
  await user.click(await screen.findByRole('button', { name: /^Rainy day, planned 100.00/ }))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Set plan' }))
  expect(await screen.findByRole('dialog', { name: 'Set plan' })).toBeInTheDocument()
  const input = await screen.findByLabelText('Plan')
  await user.clear(input)
  await user.type(input, '150')
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(api.setLimitBody()).toEqual({ budgetId: 'b1', elementId: 'acc-s1', period: '2026-07-01', amount: '150' }))
  await waitFor(() => expect(screen.queryByLabelText('Plan')).toBeNull())

  await user.click(screen.getByRole('button', { name: /^Rainy day, planned/ }))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Comments (1)' }))
  expect(await screen.findByTestId('comments-sheet')).toHaveTextContent('Topped up')
})

it('keeps the expense list while get-budget-plan is still loading', async () => {
  handlers({
    plan: http.get('*/api/v1/budget/get-budget-plan', async () => {
      await delay('infinite')
      return HttpResponse.json({})
    }),
  })
  renderPage()
  expect(await screen.findByTestId('phone-row-cat-food')).toBeInTheDocument()
  expect(screen.queryByTestId('phone-income-summary')).toBeNull()
  expect(screen.queryByTestId('phone-total-balance')).toBeNull()
})

it('leaves the plan lines out when get-budget-plan fails', async () => {
  let planRequests = 0
  handlers({
    plan: http.get('*/api/v1/budget/get-budget-plan', () => {
      planRequests += 1
      return HttpResponse.json({ success: false, message: 'x', code: 0, errors: {} }, { status: 500 })
    }),
  })
  const { queryClient } = renderPage()
  expect(await screen.findByTestId('phone-row-cat-food')).toBeInTheDocument()
  // only once the 500 has landed in the plan query does its absence prove anything
  await waitFor(() => {
    const [plan] = queryClient.getQueryCache().findAll({ queryKey: [...queryKeys.budgetPlan, 'b1'] })
    expect(plan?.state.status).toBe('error')
  })
  expect(planRequests).toBeGreaterThan(0)
  expect(screen.queryByTestId('phone-income-summary')).toBeNull()
  expect(screen.queryByTestId('phone-total-balance')).toBeNull()
  expect(screen.getByTestId('phone-row-cat-food')).toBeInTheDocument()
})

it('edit structure on a phone still shows the table editor', async () => {
  handlers()
  const user = userEvent.setup()
  renderPage()
  await screen.findByTestId('phone-month-view')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Edit structure' }))
  expect(await screen.findByTestId('budget-table')).toBeInTheDocument()
  expect(screen.queryByTestId('phone-month-view')).toBeNull()
})

it('a month three past the current one still carries the unmet plans of the months between', async () => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 6, 15, 12, 0, 0))
  useBudgetPeriodStore.setState({ selectedDate: '2026-10-01' })
  const froms: string[] = []
  handlers({
    plan: http.get('*/api/v1/budget/get-budget-plan', ({ request }) => {
      const params = new URL(request.url).searchParams
      const from = params.get('from') ?? ''
      froms.push(from)
      const months = Array.from({ length: Number(params.get('months')) }, (_, i) => addMonths(from, i))
      const cells = (planned: string) => months.map(() => ({ actual: '0', planned }))
      const plan = {
        meta: fixtureWirePlan.meta,
        months,
        openingBalances: [{ currencyId: 'cur-usd', amount: '500' }],
        currencyRates: months.map((m) => ({
          period: m,
          rates: [{ currencyId: 'cur-usd', baseCurrencyId: 'cur-usd', rate: '1', periodStart: m, periodEnd: addMonths(m, 1) }],
        })),
        transfers: months.map((m) => ({ period: m, items: [] })),
        structure: {
          folders: [],
          elements: [
            { id: 'ie1', type: 4, name: 'Salaries', icon: 'payments', currencyId: 'cur-usd', isArchived: 0, folderId: null, position: 0, ownerUserId: null, cells: cells('1000'), children: [] },
            { id: 'cat-food', type: 1, name: 'Food', icon: 'restaurant', currencyId: 'cur-usd', isArchived: 0, folderId: null, position: 1, ownerUserId: 'u1', cells: cells('300'), children: [] },
          ],
        },
      }
      return HttpResponse.json({ success: true, message: '', data: { item: plan } })
    }),
  })
  try {
    renderPage()
    // July (current) through October, each planned +1000 −300 and nothing booked yet
    expect(await screen.findByTestId('phone-total-balance')).toHaveTextContent('3,300.00')
    expect(froms.every((f) => f <= '2026-07-01')).toBe(true)
  } finally {
    vi.useRealTimers()
  }
})

it('the sheet’s Edit opens the category’s own dialog in place of the sheet', async () => {
  handlers()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByRole('button', { name: /^Food, budget 200.00/ }))
  await user.click(within(await screen.findByRole('dialog', { name: /^Food · / })).getByRole('button', { name: 'Edit' }))
  const dialog = await screen.findByRole('dialog', { name: 'Edit category' })
  expect(within(dialog).getByDisplayValue('Food')).toBeInTheDocument()
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

it('a guest’s envelope sheet shows Edit inactive', async () => {
  handlers({ budget: guestBudget })
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByRole('button', { name: /^Living, budget/ }))
  expect(within(await screen.findByRole('dialog', { name: /^Living · / })).getByRole('button', { name: 'Edit' })).toBeDisabled()
})

it('a savings row’s Edit opens the account dialog, and is inactive once the account is gone', async () => {
  useUiStore.setState({ accountModal: null })
  const account = { ...fixtureAccounts[2], id: 'acc-s1', name: 'Rainy day' }
  handlers({ budget: savingsBudget, accounts: [...fixtureAccounts, account] })
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  const { queryClient } = renderPage()
  await user.click(await screen.findByTestId('phone-savings-summary'))
  await user.click(await screen.findByRole('button', { name: /^Rainy day, planned/ }))
  const edit = within(await screen.findByRole('dialog', { name: /^Rainy day · / })).getByRole('button', { name: 'Edit' })
  await waitFor(() => expect(edit).toBeEnabled())
  await user.click(edit)
  expect(useUiStore.getState().accountModal?.account?.id).toBe('acc-s1')
  expect(screen.queryByTestId('element-sheet')).toBeNull()

  queryClient.setQueryData(queryKeys.accounts, fixtureAccounts)
  await user.click(screen.getByRole('button', { name: /^Rainy day, planned/ }))
  expect(within(await screen.findByRole('dialog', { name: /^Rainy day · / })).getByRole('button', { name: 'Edit' })).toBeDisabled()
})

const labelBudget = {
  ...fixtureWireBudget,
  structure: {
    ...fixtureWireBudget.structure,
    labels: [{ id: 'label-kid-a', name: 'kid-A', icon: 'label', isArchived: 0, spent: '50', ownerUserId: 'u1', children: [] }],
  },
}

it('a reporting tag’s sheet → Transactions lists that tag’s spending that month', async () => {
  handlers({ budget: labelBudget })
  const params = captureTxListParams()
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Reporting tags' }))
  await user.click(await screen.findByRole('button', { name: /^kid-A, spent/ }))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Transactions' }))
  expect(await screen.findByRole('dialog', { name: /kid-A/ })).toBeInTheDocument()
  await waitFor(() => expect(params()?.get('labelId')).toBe('label-kid-a'))
  expect(params()?.get('periodStart')).toBe('2026-07-01')
})

it('a reporting tag’s Edit opens its tag dialog in place of the sheet', async () => {
  handlers({ budget: labelBudget })
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Reporting tags' }))
  await user.click(await screen.findByRole('button', { name: /^kid-A, spent/ }))
  await user.click(within(await screen.findByRole('dialog', { name: /^kid-A · / })).getByRole('button', { name: 'Edit' }))
  const dialog = await screen.findByRole('dialog', { name: 'Edit tag' })
  expect(within(dialog).getByDisplayValue('kid-A')).toBeInTheDocument()
  expect(within(dialog).getByTestId('kind-locked-note')).toBeInTheDocument()
  expect(screen.queryByTestId('element-sheet')).toBeNull()
})

it('someone else’s reporting tag shows Edit inactive', async () => {
  handlers({ budget: { ...labelBudget, structure: { ...labelBudget.structure, labels: [{ ...labelBudget.structure.labels[0], ownerUserId: 'u9' }] } } })
  const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never })
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Reporting tags' }))
  await user.click(await screen.findByRole('button', { name: /^kid-A, spent/ }))
  expect(within(await screen.findByRole('dialog', { name: /^kid-A · / })).getByRole('button', { name: 'Edit' })).toBeDisabled()
})
