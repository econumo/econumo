import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent, { PointerEventsCheckLevel } from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { delay, http, HttpResponse } from 'msw'
import type { HttpHandler } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget, planHandler } from '@/test/fixtures'
import { BudgetPage } from './BudgetPage'
import { useBudgetPeriodStore } from './budgetStore'

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

function handlers({ budget = fixtureWireBudget, plan = planHandler() }: { budget?: unknown; plan?: HttpHandler } = {}) {
  let setLimitBody: unknown
  server.use(
    ...coreHandlers({ user: userWithBudget }),
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

function renderPage(path: '/budget' | '/plan' = '/budget') {
  const router = createMemoryRouter(
    [
      { path: '/budget', element: <BudgetPage key="budget" mode="budget" /> },
      { path: '/plan', element: <BudgetPage key="plan" mode="plan" /> },
    ],
    { initialEntries: [path] },
  )
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  phone()
  useBudgetPeriodStore.setState({ selectedDate: '2026-07-01', unfoldedElements: {}, foldBudgetId: null, planHideEmpty: false })
})

it('renders the single month view on /budget and on /plan alike', async () => {
  handlers()
  renderPage('/budget')
  expect(await screen.findByTestId('phone-month-view')).toBeInTheDocument()
  expect(screen.queryByTestId('budget-table')).toBeNull()
})

it('/plan on a phone is the same month view, not the plan grid', async () => {
  handlers()
  renderPage('/plan')
  expect(await screen.findByTestId('phone-month-view')).toBeInTheDocument()
  expect(screen.queryByTestId('plan-sheet')).toBeNull()
})

it('has no Budget/Plan switch in the settings menu, and the title is not all caps', async () => {
  handlers()
  const user = userEvent.setup()
  renderPage()
  const title = await screen.findByRole('heading', { name: 'Main budget' })
  expect(title.className).not.toContain('uppercase')
  await user.click(screen.getByRole('button', { name: 'Configure' }))
  expect(await screen.findByRole('menuitem', { name: 'Edit structure' })).toBeInTheDocument()
  expect(screen.queryByRole('menuitemradio')).toBeNull()
})

it('/plan on a phone has no Hide empty rows toggle', async () => {
  handlers()
  const user = userEvent.setup()
  renderPage('/plan')
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
  const input = screen.getByLabelText('Budget')
  await user.clear(input)
  await user.type(input, '650')
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(api.setLimitBody()).toEqual({ budgetId: 'b1', elementId: 'cat-freelance', period: '2026-07-01', amount: '650' }))
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
  await user.click(await screen.findByRole('button', { name: /^Rainy day, planned 100.00/ }))
  await user.click(within(await screen.findByTestId('element-sheet')).getByRole('button', { name: 'Set budget' }))
  const input = await screen.findByLabelText('Budget')
  await user.clear(input)
  await user.type(input, '150')
  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(api.setLimitBody()).toEqual({ budgetId: 'b1', elementId: 'acc-s1', period: '2026-07-01', amount: '150' }))
  await waitFor(() => expect(screen.queryByLabelText('Budget')).toBeNull())

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
  expect(screen.queryByTestId('phone-income')).toBeNull()
  expect(screen.queryByTestId('phone-total-balance')).toBeNull()
})

it('leaves the plan lines out when get-budget-plan fails', async () => {
  handlers({
    plan: http.get('*/api/v1/budget/get-budget-plan', () =>
      HttpResponse.json({ success: false, message: 'x', code: 0, errors: {} }, { status: 500 }),
    ),
  })
  renderPage()
  expect(await screen.findByTestId('phone-row-cat-food')).toBeInTheDocument()
  await waitFor(() => expect(screen.queryByTestId('phone-income')).toBeNull())
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
