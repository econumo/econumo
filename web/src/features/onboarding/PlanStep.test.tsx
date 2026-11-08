import { StrictMode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser, fixtureWireBudget } from '@/test/fixtures'
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

function wireBudget(salary: string) {
  return {
    ...fixtureWireBudget,
    meta: { ...fixtureWireBudget.meta, id: 'b-new' },
    structure: {
      folders: [],
      labels: [],
      elements: [
        element('cat-food', 1, 'Groceries', '200', 0),
        element('cat-rent', 1, 'Housing', '', 1),
        element('env-1', 0, 'An envelope', '50', 2),
        element('cat-salary', 3, 'Salary', salary, 3),
      ],
    },
  }
}

const period = (() => {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-01`
})()

let creates: unknown[]
let limits: { budgetId: string; elementId: string; period: string; amount: string | null }[]
let budgetPeriods: string[]

function setup({ salary = '3000', accounts }: { salary?: string; accounts?: unknown[] } = {}) {
  creates = []
  limits = []
  budgetPeriods = []
  server.use(...coreHandlers({ user: pendingUser, budgets: [], ...(accounts ? { accounts } : {}) }))
  server.use(
    http.post('*/api/v1/budget/create-budget', async ({ request }) => {
      creates.push(await request.json())
      return ok({ item: wireBudget(salary).meta })
    }),
    http.get('*/api/v1/budget/get-budget', ({ request }) => {
      budgetPeriods.push(new URL(request.url).searchParams.get('date') ?? '')
      return ok({ item: wireBudget(salary) })
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
