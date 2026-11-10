import { StrictMode } from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget } from '@/test/fixtures'
import * as metrics from '@/lib/metrics'
import { queryKeys } from '@/app/queryKeys'
import { PlanStep } from './PlanStep'
import { stepRoutes } from './onboardingRoutes.test-utils'

const pendingUser = {
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'onboarding' ? { ...o, value: '' } : o)),
}

const ok = (data: unknown = {}) => HttpResponse.json({ success: true, message: '', data })

// get-category-list and get-budget-plan as the server sends them (captured
// from a running instance): get-budget lists expense categories only, the plan
// lists both kinds with planned "" for no plan, an envelope's categories as
// its children, plus the synthetic uncategorized income row.
const category = (id: string, type: 'expense' | 'income', name: string, position: number, extra: Record<string, unknown> = {}) => ({
  id, ownerUserId: 'u1', name, position, type, icon: 'category', isArchived: 0,
  createdAt: '2026-11-08 21:37:17', updatedAt: '2026-11-08 21:37:17', ...extra,
})

const defaultCategories = () => [
  category('cat-food', 'expense', 'Groceries', 0),
  category('cat-rent', 'expense', 'Housing', 1),
  category('cat-coffee', 'expense', 'Coffee', 2),
  category('cat-old', 'expense', 'Old', 3, { isArchived: 1 }),
  category('cat-theirs', 'expense', 'Partner groceries', 4, { ownerUserId: 'u2' }),
  category('cat-salary', 'income', 'Salary', 5),
]

const planElement = (id: string, type: number, name: string, planned: string, position: number, children: unknown[] = []) => ({
  id, type, name, icon: 'category', currencyId: 'cur-usd', isArchived: 0, folderId: null, position, ownerUserId: 'u1',
  cells: [{ actual: '0', planned, upcoming: '0' }], children,
})

const defaultPlans = (salary: string) => ({ 'cat-food': '200', 'cat-rent': '', 'cat-salary': salary } as Record<string, string>)

function wirePlan(id: string, cats: { id: string; type: string; name: string; ownerUserId: string }[], plans: Record<string, string>, month: string, periodStartDay = 1) {
  const coffee = cats.find((c) => c.id === 'cat-coffee')
  const own = cats.filter((c) => c.id !== 'cat-coffee')
  return {
    meta: { ...fixtureWireBudget.meta, id, periodStartDay },
    months: [month],
    openingBalances: [{ currencyId: 'cur-usd', amount: '0' }],
    savingsOpeningBalances: [{ currencyId: 'cur-usd', amount: '0' }],
    currencyRates: [{ period: month, rates: [] }],
    transfers: [{ period: month, items: [] }],
    savingsFlows: [],
    savingsIncomeExpense: [],
    structure: {
      folders: [],
      savings: [],
      elements: [
        ...own.map((c, i) => planElement(c.id, c.type === 'income' ? 3 : 1, c.name, plans[c.id] ?? '', i)),
        ...(coffee
          ? [planElement('env-1', 0, 'An envelope', '50', own.length, [{ id: 'cat-coffee', type: 1, name: 'Coffee', icon: 'category', isArchived: 0, ownerUserId: 'u1', cells: [{ actual: '0', upcoming: '0' }] }])]
          : []),
        planElement('uncategorized', 3, 'Uncategorized', '', own.length + 1),
      ],
    },
  }
}

const period = (() => {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`
})()

let saves: { id: string; budgetId?: string; amounts: { categoryId: string; amount: string }[] }[]
let budgetPeriods: string[]
let routes: ReturnType<typeof stepRoutes>

interface Setup {
  salary?: string
  accounts?: unknown[]
  categories?: ReturnType<typeof category>[]
  plans?: Record<string, string>
  // the user's default budget; null = none (the accounts step was skipped)
  budget?: { id: string; periodStartDay: number; sharedAs?: 'guest' | 'user' } | null
  // a budget list already cached (fresh) when the step opens
  cachedBudgets?: unknown[]
  // get-plan-suggestions items; `gate` holds the answer until it resolves
  suggestions?: { categoryId: string; amount: string }[]
  gate?: Promise<void>
}

function setup({ salary = '3000', accounts, categories = defaultCategories(), plans = defaultPlans(salary), budget = { id: 'b1', periodStartDay: 1 }, cachedBudgets, suggestions = [], gate }: Setup = {}) {
  budgetPeriods = []
  routes = stepRoutes({ option: 'categories,accounts', budgetId: budget?.id })
  saves = []
  const user = budget
    ? { ...pendingUser, options: pendingUser.options.map((o) => (o.name === 'budget' ? { ...o, value: budget.id } : o)) }
    : pendingUser
  const shared = (role: 'guest' | 'user') => ({
    ownerUserId: 'u2',
    access: [
      { user: { id: 'u2', name: 'Partner', avatar: 'face:emerald' }, role: 'owner', isAccepted: 1 },
      { user: { id: 'u1', name: 'Ada', avatar: 'face:emerald' }, role, isAccepted: 1 },
    ],
  })
  const metas = budget
    ? [{ ...fixtureWireBudget.meta, id: budget.id, periodStartDay: budget.periodStartDay, ...(budget.sharedAs ? shared(budget.sharedAs) : {}) }]
    : []
  server.use(...coreHandlers({ user, budgets: metas, categories, ...(accounts ? { accounts } : {}) }))
  server.use(
    http.get('*/api/v1/budget/get-budget-list', () => ok({ items: metas })),
    http.get('*/api/v1/budget/get-budget-plan', ({ request }) => {
      const url = new URL(request.url)
      const from = url.searchParams.get('from') ?? ''
      budgetPeriods.push(from)
      const id = url.searchParams.get('id') ?? ''
      return ok({ item: wirePlan(id, categories, plans, from, metas.find((m) => m.id === id)?.periodStartDay ?? 1) })
    }),
    http.get('*/api/v1/oauth/get-identity-list', () => ok([])),
    // as the server answers: the budget asked for and a decimal string per category
    http.get('*/api/v1/onboarding/get-plan-suggestions', async ({ request }) => {
      await gate
      return ok({ budgetId: new URL(request.url).searchParams.get('budgetId') ?? '', items: suggestions })
    }),
    ...routes.handlers,
  )
  const onDone = vi.fn(() => {
    saves = routes.srv.calls.map((c) => c.body)
    return Promise.resolve()
  })
  const onSkip = vi.fn(() => Promise.resolve())
  const onGoTo = vi.fn()
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  if (cachedBudgets) queryClient.setQueryData(queryKeys.budgets, cachedBudgets)
  render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <PlanStep onDone={onDone} onSkip={onSkip} onGoTo={onGoTo} />
      </QueryClientProvider>
    </StrictMode>,
  )
  return { onDone, onSkip, onGoTo, queryClient }
}

const input = (name: string) => screen.findByRole('textbox', { name }, { timeout: 5000 })

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

afterEach(() => {
  vi.useRealTimers()
})

it('loads this month of the default budget and writes nothing on entering', async () => {
  setup()
  await input('Groceries')
  expect(budgetPeriods).toContain(period)
  expect(routes.srv.calls).toEqual([])
})

it('lists income and expense categories in their groups, prefilled from the plan', async () => {
  setup()
  expect(await input('Groceries')).toHaveValue('200')
  expect(screen.getByRole('textbox', { name: 'Housing' })).toHaveValue('')
  expect(screen.getByRole('textbox', { name: 'Salary' })).toHaveValue('3000')
  for (const hidden of ['An envelope', 'Coffee', 'Old', 'Partner groceries', 'Uncategorized']) {
    expect(screen.queryByRole('textbox', { name: hidden })).not.toBeInTheDocument()
  }
  expect(within(screen.getByTestId('group-income')).getByRole('textbox', { name: 'Salary' })).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'Plan this month' })).toBeInTheDocument()
  expect(screen.getByText(/^Income · /)).toBeInTheDocument()
  expect(screen.getByText(/^Expenses · /)).toBeInTheDocument()
})

it('plans spending against expected income when there is any', async () => {
  const user = userEvent.setup()
  setup()
  await input('Groceries')
  expect(screen.getByText('Planned 200.00 $ of 3,000.00 $ expected income')).toBeInTheDocument()
  await user.type(screen.getByRole('textbox', { name: 'Housing' }), '1800')
  expect(screen.getByText('Planned 2,000.00 $ of 3,000.00 $ expected income')).toBeInTheDocument()
})

it('plans against the money in the accounts when no income is planned', async () => {
  setup({ salary: '' })
  await input('Groceries')
  expect(screen.getByText(/^Planned 200\.00 \$ of [\d,.]+ \$ in your accounts$/)).toBeInTheDocument()
  // own accounts only: the fixture's USD accounts plus the EUR one converted
  expect(screen.getByText(/of 2,20\d\.\d\d \$ in your accounts/)).toBeInTheDocument()
})

it('saves changed amounts only, a typed zero but never a blank, then moves on', async () => {
  const user = userEvent.setup()
  const { onDone } = setup()
  await input('Groceries')
  await user.type(screen.getByRole('textbox', { name: 'Housing' }), '0')
  await user.clear(screen.getByRole('textbox', { name: 'Salary' }))
  await user.click(screen.getByRole('button', { name: 'Save plan' }))
  await waitFor(() => expect(onDone).toHaveBeenCalled())
  expect(saves).toEqual([{ id: expect.any(String), budgetId: 'b1', amounts: [{ categoryId: 'cat-rent', amount: '0' }] }])
})

it('keeps a stored plan digit for digit and leaves an untouched one alone', async () => {
  const user = userEvent.setup()
  const { onDone } = setup({ plans: { 'cat-food': '1234.123456789', 'cat-rent': '0', 'cat-salary': '' } })
  expect(await input('Groceries')).toHaveValue('1234.123456789')
  // a stored zero plan reads as zero, and typing it again writes nothing
  expect(screen.getByRole('textbox', { name: 'Housing' })).toHaveValue('0')
  expect(screen.getByRole('textbox', { name: 'Salary' })).toHaveValue('')
  await user.type(screen.getByRole('textbox', { name: 'Salary' }), '0')
  await user.click(screen.getByRole('button', { name: 'Save plan' }))
  await waitFor(() => expect(onDone).toHaveBeenCalled())
  expect(saves.map((b) => b.amounts)).toEqual([[{ categoryId: 'cat-salary', amount: '0' }]])
})

it('writes every changed row in one save-plan', async () => {
  const user = userEvent.setup()
  const { onDone } = setup()
  await input('Groceries')
  await user.clear(screen.getByRole('textbox', { name: 'Groceries' }))
  await user.type(screen.getByRole('textbox', { name: 'Groceries' }), '250.5')
  await user.clear(screen.getByRole('textbox', { name: 'Salary' }))
  await user.type(screen.getByRole('textbox', { name: 'Salary' }), '3200')
  await user.click(screen.getByRole('button', { name: 'Save plan' }))
  await waitFor(() => expect(onDone).toHaveBeenCalled())
  expect(saves.map((b) => b.amounts)).toEqual([
    [
      { categoryId: 'cat-salary', amount: '3200' },
      { categoryId: 'cat-food', amount: '250.5' },
    ],
  ])
})

it('refuses to save an amount it cannot read', async () => {
  const user = userEvent.setup()
  const { onDone } = setup()
  await input('Groceries')
  await user.type(screen.getByRole('textbox', { name: 'Housing' }), 'abc')
  expect(screen.getByRole('button', { name: 'Save plan' })).toBeDisabled()
  expect(onDone).not.toHaveBeenCalled()
})

it('without a budget it asks for an account and opens the Accounts step, and Skip stays', async () => {
  const user = userEvent.setup()
  const { onGoTo, onSkip } = setup({ accounts: [], budget: null })
  await user.click(await screen.findByRole('button', { name: 'Add an account' }, { timeout: 5000 }))
  expect(onGoTo).toHaveBeenCalledWith('accounts')
  expect(screen.getByText('A budget needs at least one account. Add one first.')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Save plan' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Skip for now' }))
  expect(onSkip).toHaveBeenCalled()
  expect(routes.srv.calls).toEqual([])
})

it('a budget shared read-only is not planned: it asks for an account instead', async () => {
  setup({ budget: { id: 'b-shared', periodStartDay: 1, sharedAs: 'guest' } })
  expect(await screen.findByRole('button', { name: 'Add an account' }, { timeout: 5000 })).toBeInTheDocument()
  expect(budgetPeriods).toEqual([])
})

it('a budget shared with edit rights is planned', async () => {
  setup({ budget: { id: 'b-shared', periodStartDay: 1, sharedAs: 'user' } })
  expect(await input('Groceries')).toBeInTheDocument()
})

it('Skip for now skips the step', async () => {
  const user = userEvent.setup()
  const { onSkip } = setup()
  await input('Groceries')
  await user.click(screen.getByRole('button', { name: 'Skip for now' }))
  expect(onSkip).toHaveBeenCalled()
})

it('plans the current period of a reused budget whose months start on the 15th', async () => {
  vi.useFakeTimers({ toFake: ['Date'], now: new Date(2026, 10, 10, 12) })
  const user = userEvent.setup()
  const { onDone } = setup({ budget: { id: 'b-old', periodStartDay: 15 } })
  await input('Groceries')
  expect(budgetPeriods).toEqual(expect.arrayContaining(['2026-10-01']))
  expect(budgetPeriods).not.toContain('2026-11-01')
  expect(screen.getByText('Expenses · October')).toBeInTheDocument()
  await user.type(screen.getByRole('textbox', { name: 'Housing' }), '10')
  await user.click(screen.getByRole('button', { name: 'Save plan' }))
  await waitFor(() => expect(onDone).toHaveBeenCalled())
  // the server writes its own current period of the budget; the client names the budget only
  expect(saves).toEqual([{ id: expect.any(String), budgetId: 'b-old', amounts: [{ categoryId: 'cat-rent', amount: '10' }] }])
})

it('refuses a negative amount', async () => {
  const user = userEvent.setup()
  setup()
  await input('Groceries')
  await user.type(screen.getByRole('textbox', { name: 'Housing' }), '-5')
  expect(screen.getByRole('button', { name: 'Save plan' })).toBeDisabled()
  expect(screen.getByText('Enter an amount of zero or more, for example 250.50')).toBeInTheDocument()
})

it('with no categories to plan it points to the Categories step', async () => {
  const user = userEvent.setup()
  const { onGoTo } = setup({ categories: [category('cat-old', 'expense', 'Old', 0, { isArchived: 1 })] })
  await user.click(await screen.findByRole('button', { name: 'Add categories' }, { timeout: 5000 }))
  expect(onGoTo).toHaveBeenCalledWith('categories')
  expect(screen.getByText('There are no categories to plan yet. Add a few first.')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Save plan' })).toBeDisabled()
})

it('reports the plan once save-plan succeeds and takes the user it answers with', async () => {
  const user = userEvent.setup()
  const tracked: unknown[] = []
  const spy = vi.spyOn(metrics, 'trackEvent').mockImplementation((...args) => void tracked.push(args))
  const { onDone, queryClient } = setup()
  await input('Groceries')
  routes.srv.failNext.plan = 'http500'
  await user.type(screen.getByRole('textbox', { name: 'Housing' }), '5')
  await user.click(screen.getByRole('button', { name: 'Save plan' }))
  expect(await screen.findByRole('alert')).toBeInTheDocument()
  expect(tracked).toEqual([])
  expect(onDone).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Save plan' }))
  await waitFor(() => expect(onDone).toHaveBeenCalled())
  expect(tracked).toEqual([[metrics.METRICS.ONBOARDING_PLAN_COMPLETE, { rows: 1 }]])
  // the retry replays the operation id of the failed save
  expect(routes.srv.calls.map((c) => c.body.id)).toEqual([routes.srv.calls[0].body.id, routes.srv.calls[0].body.id])
  const option = queryClient.getQueryData<typeof fixtureUser>(queryKeys.user)?.options.find((o) => o.name === 'onboarding')?.value
  expect(option).toBe('categories,accounts,plan')
  spy.mockRestore()
})

it('a cached budget list without the budget is refetched before planning', async () => {
  setup({ cachedBudgets: [] })
  expect(await input('Groceries')).toBeInTheDocument()
  expect(budgetPeriods).toContain(period)
})

describe('plan suggestions', () => {
  const suggestions = [
    { categoryId: 'cat-food', amount: '40' },
    { categoryId: 'cat-rent', amount: '6.67' },
    { categoryId: 'cat-old', amount: '99' },
    { categoryId: 'cat-salary', amount: '2500' },
  ]

  it('prefill only the empty inputs, never over a stored plan, and are saved as typed amounts', async () => {
    const user = userEvent.setup()
    const { onDone } = setup({ suggestions })
    await waitFor(() => expect(screen.getByRole('textbox', { name: 'Housing' })).toHaveValue('6.67'), { timeout: 5000 })
    expect(screen.getByRole('textbox', { name: 'Groceries' })).toHaveValue('200')
    expect(screen.getByRole('textbox', { name: 'Salary' })).toHaveValue('3000')
    // an archived category is never planned, suggested or not
    expect(screen.queryByRole('textbox', { name: 'Old' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Save plan' }))
    await waitFor(() => expect(onDone).toHaveBeenCalled())
    expect(saves.map((b) => b.amounts)).toEqual([[{ categoryId: 'cat-rent', amount: '6.67' }]])
  })

  it('arriving late, they leave what the user already typed or cleared alone', async () => {
    const user = userEvent.setup()
    let release = () => undefined as void
    const gate = new Promise<void>((resolve) => {
      release = resolve
    })
    const { onDone } = setup({ suggestions, gate, plans: { 'cat-food': '', 'cat-rent': '', 'cat-salary': '' } })
    await input('Groceries')
    await user.type(screen.getByRole('textbox', { name: 'Housing' }), '5')
    await user.type(screen.getByRole('textbox', { name: 'Groceries' }), '1')
    await user.clear(screen.getByRole('textbox', { name: 'Groceries' }))
    release()
    await waitFor(() => expect(screen.getByRole('textbox', { name: 'Salary' })).toHaveValue('2500'))
    expect(screen.getByRole('textbox', { name: 'Housing' })).toHaveValue('5')
    expect(screen.getByRole('textbox', { name: 'Groceries' })).toHaveValue('')
    await user.click(screen.getByRole('button', { name: 'Save plan' }))
    await waitFor(() => expect(onDone).toHaveBeenCalled())
    expect(saves.map((b) => b.amounts)).toEqual([[
      { categoryId: 'cat-salary', amount: '2500' },
      { categoryId: 'cat-rent', amount: '5' },
    ]])
  })
})
