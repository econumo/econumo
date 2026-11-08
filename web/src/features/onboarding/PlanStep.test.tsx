import { StrictMode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget } from '@/test/fixtures'
import * as metrics from '@/lib/metrics'
import { queryKeys } from '@/app/queryKeys'
import { PlanStep } from './PlanStep'

const pendingUser = {
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'onboarding' ? { ...o, value: '' } : o)),
}

const ok = (data: unknown = {}) => HttpResponse.json({ success: true, message: '', data })

const element = (id: string, type: number, name: string, budgeted: string, position: number) => ({
  id, type, name, icon: 'category', currencyId: null, isArchived: 0, folderId: null, position,
  budgeted, isBudgeted: budgeted === '' ? 0 : 1, available: '0', spent: '0', budgetSpent: '0', ownerUserId: 'u1', children: [],
})

const defaultElements = (salary: string) => [
  element('cat-food', 1, 'Groceries', '200', 0),
  element('cat-rent', 1, 'Housing', '', 1),
  element('env-1', 0, 'An envelope', '50', 2),
  element('cat-salary', 3, 'Salary', salary, 3),
]

function wireBudget(id: string, elements: unknown[], periodStartDay = 1) {
  return {
    ...fixtureWireBudget,
    meta: { ...fixtureWireBudget.meta, id, periodStartDay },
    structure: { folders: [], labels: [], elements },
  }
}

const period = (() => {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`
})()

let creates: unknown[]
let limits: { budgetId: string; elementId: string; period: string; amount: string | null }[]
let budgetPeriods: string[]
let listCalls: number

interface Setup {
  salary?: string
  accounts?: unknown[]
  elements?: unknown[]
  // an existing default budget instead of a fresh one
  existing?: { id: string; periodStartDay: number }
  // a budget list already cached (fresh) when the step opens
  cachedBudgets?: unknown[]
  // the server lists the budget to the first list call only
  listOnce?: boolean
}

function setup({ salary = '3000', accounts, elements = defaultElements(salary), existing, cachedBudgets, listOnce }: Setup = {}) {
  creates = []
  limits = []
  budgetPeriods = []
  listCalls = 0
  const user = existing
    ? { ...pendingUser, options: pendingUser.options.map((o) => (o.name === 'budget' ? { ...o, value: existing.id } : o)) }
    : pendingUser
  const metas = () =>
    existing
      ? [wireBudget(existing.id, elements, existing.periodStartDay).meta]
      : creates.map((c) => wireBudget((c as { id: string }).id, elements).meta)
  const byId = (id: string) => metas().find((m) => m.id === id)
  server.use(...coreHandlers({ user, budgets: [], ...(accounts ? { accounts } : {}) }))
  server.use(
    http.get('*/api/v1/budget/get-budget-list', () => {
      listCalls++
      return ok({ items: listOnce && listCalls > 1 ? [] : metas() })
    }),
    http.post('*/api/v1/budget/create-budget', async ({ request }) => {
      const body = (await request.json()) as { id: string }
      creates.push(body)
      return ok({ item: byId(body.id) })
    }),
    http.get('*/api/v1/budget/get-budget', ({ request }) => {
      const url = new URL(request.url)
      budgetPeriods.push(url.searchParams.get('date') ?? '')
      const id = url.searchParams.get('id') ?? ''
      return ok({ item: wireBudget(id, elements, byId(id)?.periodStartDay ?? 1) })
    }),
    http.get('*/api/v1/oauth/get-identity-list', () => ok([])),
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      limits.push((await request.json()) as (typeof limits)[number])
      return ok()
    }),
  )
  const onDone = vi.fn(() => Promise.resolve())
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
  return { onDone, onSkip, onGoTo }
}

const createdId = () => (creates[0] as { id: string }).id

const input = (name: string) => screen.findByRole('textbox', { name }, { timeout: 5000 })

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

afterEach(() => {
  vi.useRealTimers()
})

it('creates the budget once on entering and loads this month', async () => {
  setup()
  await input('Groceries')
  expect(creates).toHaveLength(1)
  expect(budgetPeriods).toContain(period)
})

it('lists income and expense categories in their groups, prefilled from the plan', async () => {
  setup()
  expect(await input('Groceries')).toHaveValue('200')
  expect(screen.getByRole('textbox', { name: 'Housing' })).toHaveValue('')
  expect(screen.getByRole('textbox', { name: 'Salary' })).toHaveValue('3000')
  expect(screen.queryByRole('textbox', { name: 'An envelope' })).not.toBeInTheDocument()
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
  expect(limits).toEqual([{ budgetId: createdId(), elementId: 'cat-rent', period, amount: '0' }])
})

it('writes every changed row with the first of the month', async () => {
  const user = userEvent.setup()
  const { onDone } = setup()
  await input('Groceries')
  await user.clear(screen.getByRole('textbox', { name: 'Groceries' }))
  await user.type(screen.getByRole('textbox', { name: 'Groceries' }), '250.5')
  await user.clear(screen.getByRole('textbox', { name: 'Salary' }))
  await user.type(screen.getByRole('textbox', { name: 'Salary' }), '3200')
  await user.click(screen.getByRole('button', { name: 'Save plan' }))
  await waitFor(() => expect(onDone).toHaveBeenCalled())
  expect(limits).toEqual([
    { budgetId: createdId(), elementId: 'cat-salary', period, amount: '3200' },
    { budgetId: createdId(), elementId: 'cat-food', period, amount: '250.5' },
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

it('without an account it asks for one and opens the Accounts step', async () => {
  const user = userEvent.setup()
  const { onGoTo } = setup({ accounts: [] })
  await user.click(await screen.findByRole('button', { name: 'Add an account' }, { timeout: 5000 }))
  expect(onGoTo).toHaveBeenCalledWith('accounts')
  expect(screen.getByText('A budget needs at least one account. Add one first.')).toBeInTheDocument()
  expect(creates).toHaveLength(0)
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
  const { onDone } = setup({ existing: { id: 'b-old', periodStartDay: 15 } })
  await input('Groceries')
  expect(creates).toHaveLength(0)
  expect(budgetPeriods).toEqual(expect.arrayContaining(['2026-10-01']))
  expect(budgetPeriods).not.toContain('2026-11-01')
  expect(screen.getByText('Expenses · October')).toBeInTheDocument()
  await user.type(screen.getByRole('textbox', { name: 'Housing' }), '10')
  await user.click(screen.getByRole('button', { name: 'Save plan' }))
  await waitFor(() => expect(onDone).toHaveBeenCalled())
  expect(limits).toEqual([{ budgetId: 'b-old', elementId: 'cat-rent', period: '2026-10-01', amount: '10' }])
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
  const { onGoTo } = setup({ elements: [element('env-1', 0, 'An envelope', '50', 0)] })
  await user.click(await screen.findByRole('button', { name: 'Add categories' }, { timeout: 5000 }))
  expect(onGoTo).toHaveBeenCalledWith('categories')
  expect(screen.getByText('There are no categories to plan yet. Add a few first.')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Save plan' })).toBeDisabled()
})

it('reports the plan only once the step is answered', async () => {
  const user = userEvent.setup()
  const { onDone } = setup()
  let resolve = () => {}
  onDone.mockImplementation(() => new Promise<void>((r) => (resolve = r)))
  const tracked: unknown[] = []
  const spy = vi.spyOn(metrics, 'trackEvent').mockImplementation((...args) => void tracked.push(args))
  await input('Groceries')
  await user.type(screen.getByRole('textbox', { name: 'Housing' }), '5')
  await user.click(screen.getByRole('button', { name: 'Save plan' }))
  await waitFor(() => expect(onDone).toHaveBeenCalled())
  expect(tracked).toEqual([])
  resolve()
  await waitFor(() => expect(tracked).toEqual([[metrics.METRICS.ONBOARDING_PLAN_COMPLETE, { rows: 1 }]]))
  spy.mockRestore()
})

it('a cached budget list without the budget is refetched before planning', async () => {
  setup({ existing: { id: 'b-old', periodStartDay: 1 }, cachedBudgets: [] })
  expect(await input('Groceries')).toBeInTheDocument()
  expect(budgetPeriods).toContain(period)
})

it('a budget the list never shows is an error with retry, and Skip stays', async () => {
  const user = userEvent.setup()
  const { onSkip } = setup({ existing: { id: 'b-old', periodStartDay: 1 }, cachedBudgets: [], listOnce: true })
  expect(await screen.findByRole('button', { name: 'Try again' }, { timeout: 5000 })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Save plan' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Skip for now' }))
  expect(onSkip).toHaveBeenCalled()
})
