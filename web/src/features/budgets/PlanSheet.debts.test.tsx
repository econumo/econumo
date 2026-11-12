import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { delay, http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureEur, fixtureUsd, fixtureUser, fixtureWireBudget, fixtureWirePlan, planHandler } from '@/test/fixtures'
import type { BudgetPlanDto } from '@/api/dto/budget'
import { moneyFormat } from '@/lib/money'
import { BudgetPage } from './BudgetPage'
import { useBudgetPeriodStore } from './budgetStore'
import { useUiStore } from '@/app/uiStore'
import { balanceRow, makePlanExchange, planTotals } from './planMath'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})
vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

const userWithBudget = {
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'budget' ? { ...o, value: 'b1' } : o)),
}

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/plan', element: <BudgetPage key="plan" mode="plan" /> }], { initialEntries: ['/plan'] })
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

const debtCell = (actual: string, planned: string, closingBalance: string, paymentsLeft: number, isPlannable: 0 | 1, scheduledTotal = '1000') => ({
  actual, planned, closingBalance, paymentsLeft, scheduledTotal, upcoming: '0', isPlannable,
})

// plan months May..Aug; the window shows Jun/Jul/Aug
const mortgage = {
  id: 'debt-1', type: 6, name: 'Mortgage', icon: 'home', currencyId: 'cur-usd', position: 0,
  payoffDate: '2028-03-05', basePayoffDate: '2028-10-05', overpaymentLeft: '1500', baseOverpaymentLeft: '2000', isPaidOff: 0, isSetUp: 1,
  cells: [
    debtCell('0', '', '0', 0, 0, '0'),
    debtCell('1000', '1000', '24000', 24, 1),
    debtCell('1000', '1500', '23000', 23, 1),
    debtCell('0', '1500', '21500', 21, 1),
  ],
}
// paid off in June: July and August are past its life
const car = {
  id: 'debt-2', type: 6, name: 'Car', icon: 'directions_car', currencyId: 'cur-usd', position: 1,
  payoffDate: '2026-06-20', basePayoffDate: '2026-06-20', overpaymentLeft: '0', baseOverpaymentLeft: '0', isPaidOff: 1,
  cells: [
    debtCell('300', '300', '300', 1, 1, '300'),
    debtCell('300', '300', '0', 0, 1, '300'),
    debtCell('0', '', '0', 0, 0, '0'),
    debtCell('0', '', '0', 0, 0, '0'),
  ],
}
const savingsRow = {
  id: 'acc-s1', type: 5, name: 'Rainy day', icon: 'savings', currencyId: 'cur-usd', ownerUserId: 'u1', isArchived: 0, position: 0,
  cells: [{ actual: '0', planned: '' }, { actual: '0', planned: '' }, { actual: '0', planned: '' }, { actual: '0', planned: '' }],
}

const debtPlan = {
  ...fixtureWirePlan,
  structure: { ...fixtureWirePlan.structure, savings: [savingsRow], debts: [car, mortgage] },
}

function useHandlers(plan: unknown = debtPlan, extra: Parameters<typeof server.use> = []) {
  server.use(
    ...coreHandlers({ user: userWithBudget }),
    http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: fixtureWireBudget } })),
    planHandler(plan),
    http.get('*/api/v1/budget/get-comment-list', () => HttpResponse.json({ success: true, message: '', data: { items: [], truncated: false } })),
    ...extra,
  )
}

const rowIds = (section: HTMLElement) => [...section.querySelectorAll('[data-row-id]')].map((r) => r.getAttribute('data-row-id'))

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(new Date(2026, 7, 15, 12, 0, 0))
  vi.clearAllMocks()
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
  useBudgetPeriodStore.setState({
    selectedDate: '2026-07-01',
    unfoldedElements: {},
    planUnfoldedElements: {},
    foldBudgetId: null,
    planFolds: {},
    planSumsShown: {},
  })
})

afterEach(() => {
  vi.useRealTimers()
})

it('renders the Debts section after Savings', async () => {
  useHandlers()
  renderPage()
  const section = await screen.findByTestId('plan-section-debts')
  const savings = screen.getByTestId('plan-section-savings')
  const expense = screen.getByTestId('plan-section-expense')
  expect(savings.compareDocumentPosition(section) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(section.compareDocumentPosition(expense) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(section).toHaveClass('plan-band')
  expect(within(section).getByRole('button', { name: 'Debts' })).toBeInTheDocument()
  // rows by position
  expect(rowIds(section)).toEqual(['debt-1:6', 'debt-2:6'])
  // a debt row never shows among the expenses
  expect(within(expense).queryByTitle('Mortgage')).toBeNull()

  // each cell: the month-end balance where the actual would be, the plan beside it,
  // and the payments still left under them
  const jul = screen.getByTestId('plan-cell-debt-1:1')
  expect(within(jul).getByTestId('cell-actual')).toHaveAttribute('data-figure', 'balance')
  expect(within(jul).getByTestId('cell-actual')).toHaveTextContent('23,000.00')
  expect(within(jul).getByTestId('cell-planned')).toHaveTextContent('1,500.00')
  expect(within(jul).getByTestId('cell-payments-left')).toHaveTextContent('23 payments left')
  expect(within(screen.getByTestId('plan-cell-debt-2:0')).getByTestId('cell-payments-left')).toHaveTextContent('Paid off')

  // folding hides the rows and keeps the fold
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  await user.click(within(section).getByRole('button', { name: 'Debts' }))
  expect(within(screen.getByTestId('plan-section-debts')).queryByTitle('Mortgage')).toBeNull()
  expect(useBudgetPeriodStore.getState().planFolds.debts).toBe(true)
})

it('editing a debt cell sends set-limit with the debt id', async () => {
  let body: unknown
  useHandlers(debtPlan, [
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      body = await request.json()
      await delay('infinite')
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  ])
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage()
  await screen.findByTestId('plan-section-debts')

  // column 2 is August
  const cell = screen.getByTestId('plan-cell-debt-1:2')
  await user.click(cell)
  await user.keyboard('{Enter}')
  const input = await screen.findByRole('textbox', { name: 'Plan for Mortgage, August' })
  await user.clear(input)
  await user.type(input, '2500')
  await user.keyboard('{Enter}')

  await waitFor(() => expect(body).toEqual({ budgetId: 'b1', elementId: 'debt-1', period: '2026-08-01', amount: '2500' }))
  // the optimistic patch reaches structure.debts
  expect(within(cell).getByTestId('cell-planned')).toHaveTextContent('2,500.00')
})

it('cells outside the debt\'s months are read-only', async () => {
  useHandlers()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage()
  await screen.findByTestId('plan-section-debts')

  // the Car was paid off in June: its June cell edits, July does not
  await user.click(screen.getByTestId('plan-cell-debt-2:0'))
  expect(within(screen.getByTestId('plan-cell-debt-2:0')).getByTestId('fill-handle')).toBeInTheDocument()

  const jul = screen.getByTestId('plan-cell-debt-2:1')
  await user.click(jul)
  expect(within(jul).queryByTestId('fill-handle')).toBeNull()
  await user.keyboard('{Enter}')
  await user.keyboard('5')
  expect(screen.queryByRole('textbox')).toBeNull()
  // the server refuses a comment there too: no marker offers a new one
  expect(within(screen.getByTestId('plan-cell-debt-2:0')).getByTestId('comment-marker-add')).toBeInTheDocument()
  expect(within(jul).queryByTestId('comment-marker-add')).toBeNull()
  // no balance or payments line where the debt no longer runs
  expect(within(jul).queryByTestId('cell-actual')).toBeNull()
  expect(within(jul).queryByTestId('cell-payments-left')).toBeNull()
})

it('the row header shows the payoff date alone, without what the extras buy', async () => {
  useHandlers()
  renderPage()
  const section = await screen.findByTestId('plan-section-debts')
  const mortgageRow = section.querySelector('[data-row-id="debt-1:6"]') as HTMLElement
  // a projected payoff reads as one, not as a payoff that happened
  expect(within(mortgageRow).getByTestId('debt-payoff')).toHaveTextContent(/^Paid off by Mar 2028$/)
  // the "N months earlier" note belongs to the plan editor, not the row header
  expect(within(mortgageRow).queryByTestId('debt-extra-effect')).toBeNull()
  const carRow = section.querySelector('[data-row-id="debt-2:6"]') as HTMLElement
  expect(within(carRow).getByTestId('debt-payoff')).toHaveTextContent(/^Paid off$/)
  expect(within(carRow).queryByTestId('debt-extra-effect')).toBeNull()
})

it('debt payments stay in the expense totals and the Balance', async () => {
  useHandlers()
  renderPage()
  await screen.findByTestId('plan-section-debts')
  const plan = debtPlan as unknown as BudgetPlanDto
  const ex = makePlanExchange(plan, [fixtureUsd, fixtureEur])
  const totals = planTotals(plan, ex)
  const balance = balanceRow(plan, totals, ex)
  const fmt = (v: string) => moneyFormat(v, fixtureUsd, { showCurrency: false, useNativePrecision: false })
  // without the debts July's expenses would be 45 + 125 + 15 + 5 + 35 EUR; the two debts add 1,000 + 0
  const withoutDebts = planTotals(fixtureWirePlan as unknown as BudgetPlanDto, ex)
  expect(totals[2].expenseActual).not.toBe(withoutDebts[2].expenseActual)
  for (let col = 0; col < 3; col++) {
    expect(screen.getByTestId(`plan-balance-${col}`)).toHaveTextContent(fmt(balance[col + 1]))
  }
})

it('a debt row\'s menu offers its owner Details, Mark as paid off and Delete', async () => {
  useHandlers()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage()
  await screen.findByTestId('plan-section-debts')
  await user.click(await screen.findByRole('button', { name: 'menu Mortgage' }))
  const items = await screen.findAllByRole('menuitem')
  expect(items.map((i) => i.textContent)).toEqual(['Details', 'Mark as paid off', 'Delete'])
})

it('on a tablet a debt cell opens the debt\'s sheet: Planned, Paid, Left and its owner\'s Details', async () => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023'), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  useHandlers()
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage()
  await screen.findByTestId('plan-section-debts')
  // July: planned 1,500, paid 1,000, 23,000 left
  await user.click(screen.getByTestId('plan-cell-debt-1:1'))
  const sheet = await screen.findByTestId('element-sheet')
  expect(within(sheet).getByTestId('sheet-figure-planned')).toHaveTextContent('1,500.00')
  expect(within(sheet).getByTestId('sheet-figure-paid')).toHaveTextContent('1,000.00')
  expect(within(sheet).getByTestId('sheet-figure-left')).toHaveTextContent('23,000.00')
  expect(within(sheet).getByTestId('sheet-state')).toHaveTextContent('23 payments left')
  expect(within(sheet).getByTestId('debt-extra-effect')).toBeInTheDocument()
  expect(within(sheet).getByRole('button', { name: 'Details' })).toBeInTheDocument()
  // Details is the one way in: no edit pencil beside it
  expect(within(screen.getByRole('dialog')).queryByRole('button', { name: 'Edit' })).toBeNull()
})

describe('a guest', () => {
  // the caller (u1) reads a budget owned by u9
  const guestMeta = {
    ...fixtureWireBudget.meta,
    ownerUserId: 'u9',
    access: [{ user: { id: 'u9', name: 'Anna', avatar: 'face:fuchsia' }, role: 'owner', isAccepted: 1 }, { user: fixtureUser, role: 'guest', isAccepted: 1 }],
  }
  const useGuestHandlers = () => {
    useHandlers({ ...debtPlan, meta: { ...fixtureWirePlan.meta, ...guestMeta } })
    server.use(http.get('*/api/v1/budget/get-budget', () => HttpResponse.json({ success: true, message: '', data: { item: { ...fixtureWireBudget, meta: guestMeta } } })))
  }

  it('gets only Details in a debt row\'s menu', async () => {
    useGuestHandlers()
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    renderPage()
    await screen.findByTestId('plan-section-debts')
    await user.click(await screen.findByRole('button', { name: 'menu Mortgage' }))
    const items = await screen.findAllByRole('menuitem')
    expect(items.map((i) => i.textContent)).toEqual(['Details'])
  })

  it('opening a debt cell\'s sheet sees Details, which opens the read-only review on its Overview', async () => {
    window.matchMedia = vi.fn().mockImplementation((q: string) => ({
      matches: q.includes('1023'), media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
    }))
    useUiStore.setState({ debtDetailsId: null, debtDetailsTab: 'settings' })
    useGuestHandlers()
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    renderPage()
    await screen.findByTestId('plan-section-debts')
    await user.click(screen.getByTestId('plan-cell-debt-1:1'))
    const sheet = await screen.findByTestId('element-sheet')
    expect(within(screen.getByRole('dialog')).queryByRole('button', { name: 'Edit' })).toBeNull()
    await user.click(within(sheet).getByRole('button', { name: 'Details' }))
    expect(useUiStore.getState().debtDetailsId).toBe('debt-1')
    expect(useUiStore.getState().debtDetailsTab).toBe('overview')
  })
})
