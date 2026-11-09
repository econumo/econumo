import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories, fixtureEur, fixtureUser, fixtureUsd, fixtureWireBudget } from '@/test/fixtures'
import { HomePage } from '@/features/home/HomePage'
import { OnboardingPage } from './OnboardingPage'

let importedRows = 0
vi.mock('@/features/transactions/ImportCsvDialog', () => ({
  ImportCsvDialog: ({ open, onComplete }: { open: boolean; onComplete: (r: { imported: number }) => void }) =>
    open ? <button onClick={() => onComplete({ imported: importedRows })}>finish import</button> : null,
}))

const userWith = (gettingStarted: string, onboarding = '') => ({
  ...fixtureUser,
  options: [
    ...fixtureUser.options.map((o) => (o.name === 'onboarding' ? { ...o, value: onboarding } : o)),
    { name: 'getting_started', value: gettingStarted },
  ],
})

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/onboarding', element: <OnboardingPage /> },
      { path: '/budget', element: <div>BUDGET ROUTE</div> },
      { path: '/settings/categories', element: <div>CATEGORIES SETTINGS</div> },
    ],
    { initialEntries: ['/onboarding'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

const ok = (data: unknown = {}) => HttpResponse.json({ success: true, message: '', data })

let posts: { path: string; value?: string }[]

// get-user-data serves the stored option, a getting-started POST replaces it
function serve(initial: string) {
  posts = []
  const state = { stored: initial, completed: false, failComplete: false, budgets: [] as { id: string }[] }
  server.use(...coreHandlers({ budgets: [], categories: [] }))
  // added later, so these answer ahead of the core handlers
  server.use(
    http.get('*/api/v1/budget/get-budget-list', () => ok({ items: state.budgets })),
    http.post('*/api/v1/budget/create-budget', async ({ request }) => {
      const { id } = (await request.json()) as { id: string }
      state.budgets.push({ ...fixtureWireBudget.meta, id })
      return ok({ item: { ...fixtureWireBudget.meta, id } })
    }),
    http.get('*/api/v1/budget/get-budget', () => ok({ item: fixtureWireBudget })),
    http.get('*/api/v1/user/get-user-data', () => ok({ user: userWith(state.stored, state.completed ? 'completed' : '') })),
    http.post('*/api/v1/user/update-getting-started', async ({ request }) => {
      const { value } = (await request.json()) as { value: string }
      posts.push({ path: 'update-getting-started', value })
      state.stored = value
      return ok({ user: userWith(value) })
    }),
    http.post('*/api/v1/user/complete-onboarding', () => {
      if (state.failComplete) {
        state.failComplete = false
        return HttpResponse.json({ success: false, message: 'Try again', code: 400, errors: {} }, { status: 400 })
      }
      posts.push({ path: 'complete-onboarding' })
      state.completed = true
      return ok({ user: userWith(state.stored, 'completed') })
    }),
  )
  return state
}

function mockViewport(compact: boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023') ? compact : false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport(false)
})

// the categories and accounts steps wait for their lists, which a loaded machine can take a while to serve
const heading = (name: string) => screen.findByRole('heading', { name }, { timeout: 5000 })

it('opens on the first open step with the stepper above it', async () => {
  serve('import')
  renderPage()
  expect(await heading('What do you spend on?')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /Categories/ })).toHaveAttribute('aria-current', 'step')
  expect(screen.getByRole('button', { name: /Import/ })).toHaveAttribute('data-state', 'done')
  expect(screen.getByRole('button', { name: 'Complete setup' })).toBeInTheDocument()
})

it('skipping a step records it and moves to the next open step', async () => {
  serve('')
  const user = userEvent.setup()
  renderPage()
  expect(await heading('Bring in your history')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: "I don't have one" }))
  expect(await heading('What do you spend on?')).toBeInTheDocument()
  expect(posts).toEqual([{ path: 'update-getting-started', value: '!import' }])
})

it('a revisited step moves on to the next open step after it', async () => {
  serve('')
  const user = userEvent.setup()
  renderPage()
  await heading('Bring in your history')
  await user.click(screen.getByRole('button', { name: /Accounts/ }))
  expect(await heading("Where's your money right now?")).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Skip' }))
  expect(await heading('Plan this month')).toBeInTheDocument()
  expect(posts).toEqual([{ path: 'update-getting-started', value: '!accounts' }])
})

it('a done step cannot be opened again', async () => {
  serve('import')
  const user = userEvent.setup()
  renderPage()
  await heading('What do you spend on?')
  await user.click(screen.getByRole('button', { name: /Import/ }))
  expect(screen.getByRole('heading', { name: 'What do you spend on?' })).toBeInTheDocument()
  expect(screen.queryByRole('heading', { name: 'Bring in your history' })).not.toBeInTheDocument()
  expect(screen.getByRole('button', { name: /Categories/ })).toHaveAttribute('aria-current', 'step')
})

it('a link from Budget & plan to a done Categories step opens the categories settings instead', async () => {
  serve('import,categories,accounts')
  server.use(
    http.get('*/api/v1/budget/get-budget-plan', ({ request }) => {
      const from = new URL(request.url).searchParams.get('from') ?? ''
      return ok({
        item: {
          meta: fixtureWireBudget.meta, months: [from], openingBalances: [], savingsOpeningBalances: [], currencyRates: [{ period: from, rates: [] }], transfers: [{ period: from, items: [] }],
          savingsFlows: [], savingsIncomeExpense: [], structure: { folders: [], savings: [], elements: [] },
        },
      })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await heading('Plan this month')
  await user.click(await screen.findByRole('button', { name: 'Add categories' }, { timeout: 5000 }))
  expect(await screen.findByText('CATEGORIES SETTINGS')).toBeInTheDocument()
})

it('resolving the last open step completes onboarding and lands on the budget', async () => {
  serve('import,!categories,accounts')
  // the user's categories and the plan as the server sends them
  server.use(
    http.get('*/api/v1/category/get-category-list', () => ok({ items: fixtureCategories })),
    http.get('*/api/v1/budget/get-budget-plan', ({ request }) => {
      const from = new URL(request.url).searchParams.get('from') ?? ''
      const cell = [{ actual: '0', planned: '', upcoming: '0' }]
      const el = (id: string, type: number, name: string, position: number) => ({ id, type, name, icon: 'category', currencyId: 'cur-usd', isArchived: 0, folderId: null, position, ownerUserId: 'u1', cells: cell, children: [] })
      return ok({
        item: {
          meta: fixtureWireBudget.meta, months: [from], openingBalances: [], savingsOpeningBalances: [], currencyRates: [{ period: from, rates: [] }], transfers: [{ period: from, items: [] }],
          savingsFlows: [], savingsIncomeExpense: [],
          structure: { folders: [], savings: [], elements: [el('cat-food', 1, 'Food', 0), el('cat-salary', 3, 'Salary', 1), el('uncategorized', 3, 'Uncategorized', 2)] },
        },
      })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  expect(await heading('Plan this month')).toBeInTheDocument()
  // the heading shows while the budget loads; Save waits for its rows
  await screen.findByRole('textbox', { name: 'Food' }, { timeout: 5000 })
  await user.click(screen.getByRole('button', { name: 'Save plan' }))
  expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
  expect(posts).toEqual([
    { path: 'update-getting-started', value: 'import,!categories,accounts,plan' },
    { path: 'complete-onboarding' },
  ])
})

it('Complete setup skips every open step, then completes onboarding', async () => {
  const state = serve('import')
  const user = userEvent.setup()
  renderPage()
  await heading('What do you spend on?')
  await user.click(screen.getByRole('button', { name: 'Complete setup' }))
  expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
  expect(posts.at(-1)).toEqual({ path: 'complete-onboarding' })
  await waitFor(() => expect(state.stored.split(',').sort()).toEqual(['!accounts', '!categories', '!plan', 'import']))
})

it('an import with rows settles import, categories and accounts in one request', async () => {
  serve('')
  importedRows = 12
  const user = userEvent.setup()
  renderPage()
  await heading('Bring in your history')
  await user.click(screen.getByRole('button', { name: 'Choose a CSV file' }))
  await user.click(await screen.findByRole('button', { name: 'finish import' }))
  expect(await heading('Plan this month')).toBeInTheDocument()
  expect(posts).toEqual([{ path: 'update-getting-started', value: 'import,categories,accounts' }])
})

it('an import that brought nothing in marks nothing', async () => {
  serve('')
  importedRows = 0
  const user = userEvent.setup()
  renderPage()
  await heading('Bring in your history')
  await user.click(screen.getByRole('button', { name: 'Choose a CSV file' }))
  await user.click(await screen.findByRole('button', { name: 'finish import' }))
  expect(screen.getByRole('heading', { name: 'Bring in your history' })).toBeInTheDocument()
  expect(posts).toEqual([])
})

// HomePage pulls in the whole budget page, which a loaded machine can take a while to render
it('/ renders the getting started page for a not-yet-onboarded user', { timeout: 15000 }, async () => {
  serve('')
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter([{ path: '/', element: <HomePage /> }], { initialEntries: ['/'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  expect(await heading('Bring in your history')).toBeInTheDocument()
})

it('completes at once when every step was already answered', async () => {
  serve('import,!categories,accounts,!plan')
  renderPage()
  expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
  expect(posts).toEqual([{ path: 'complete-onboarding' }])
})

it('a failed completion shows the error and a Finish button that retries', async () => {
  const state = serve('import,!categories,accounts,!plan')
  state.failComplete = true
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByRole('alert')).toHaveTextContent('Try again')
  expect(posts).toEqual([])
  await user.click(screen.getByRole('button', { name: 'Finish' }))
  expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
  expect(posts).toEqual([{ path: 'complete-onboarding' }])
})

it('a phone gets a menu button that opens the navigation without finishing', async () => {
  mockViewport(true)
  serve('')
  renderPage()
  await heading('Bring in your history')
  expect(screen.getByRole('link', { name: 'Menu' })).toHaveAttribute('href', '/?menu=1')
})

// A user exactly as registration leaves them: onboarding "started" and no
// getting_started option. Every handler answers in the server's shape and keeps
// its state, so the walk below exercises what a real new user goes through.
function serveRegisteredUser() {
  const log: { path: string; body?: unknown }[] = []
  const owner = { id: 'u1', avatar: 'face:emerald', name: 'Ada' }
  const state = {
    options: fixtureUser.options.filter((o) => o.name !== 'onboarding').concat({ name: 'onboarding', value: 'started' }) as { name: string; value: string | null }[],
    categories: [] as Record<string, unknown>[],
    accounts: [] as Record<string, unknown>[],
    folders: [] as Record<string, unknown>[],
    budgets: [] as Record<string, unknown>[],
    plans: {} as Record<string, string>,
  }
  const user = () => ({ ...fixtureUser, options: state.options })
  const setOption = (name: string, value: string | null) => {
    state.options = state.options.some((o) => o.name === name)
      ? state.options.map((o) => (o.name === name ? { ...o, value } : o))
      : [...state.options, { name, value }]
  }
  let n = 0
  const nextId = (p: string) => `${p}-${++n}`
  server.use(...coreHandlers({ user: user(), budgets: [], categories: [], accounts: [], folders: [] }))
  server.use(
    http.get('*/api/v1/user/get-user-data', () => ok({ user: user() })),
    http.post('*/api/v1/user/update-getting-started', async ({ request }) => {
      const { value } = (await request.json()) as { value: string }
      log.push({ path: 'update-getting-started', body: value })
      setOption('getting_started', value)
      return ok({ user: user() })
    }),
    http.post('*/api/v1/user/complete-onboarding', () => {
      log.push({ path: 'complete-onboarding' })
      setOption('onboarding', 'completed')
      return ok({ user: user() })
    }),
    http.get('*/api/v1/category/get-category-list', () => ok({ items: state.categories })),
    http.post('*/api/v1/category/create-category', async ({ request }) => {
      const body = (await request.json()) as { name: string; type: string; icon: string }
      // the server mints its own id: the posted one is the operation id
      const item = { id: nextId('cat'), ownerUserId: 'u1', name: body.name, position: state.categories.length, type: body.type, icon: body.icon, isArchived: 0, createdAt: '2026-11-08 21:56:10', updatedAt: '2026-11-08 21:56:10' }
      state.categories.push(item)
      return ok({ item })
    }),
    http.get('*/api/v1/account/get-folder-list', () => ok({ items: state.folders })),
    http.get('*/api/v1/account/get-account-list', () => ok({ items: state.accounts })),
    http.post('*/api/v1/account/create-account', async ({ request }) => {
      const body = (await request.json()) as { name: string; currencyId: string; balance: string; icon: string; folderId: string | null }
      if (!body.folderId) state.folders.push({ id: 'folder-general', name: 'General', position: 0, isVisible: 1 })
      const id = nextId('acc')
      const item = {
        id, owner, folderId: body.folderId ?? 'folder-general', name: body.name, position: state.accounts.length,
        currency: body.currencyId === 'cur-eur' ? fixtureEur : fixtureUsd, balance: body.balance, type: 2, icon: body.icon,
        sharedAccess: [], transactionsRevision: 1, transactionsCount: 1,
      }
      state.accounts.push(item)
      return ok({ item, transaction: null })
    }),
    http.post('*/api/v1/user/update-currency', () => ok({ user: user() })),
    http.post('*/api/v1/currency/hide-all-currencies', () => ok()),
    http.post('*/api/v1/currency/show-currency', () => ok()),
    http.get('*/api/v1/budget/get-budget-list', () => ok({ items: state.budgets })),
    http.post('*/api/v1/budget/create-budget', async ({ request }) => {
      const body = (await request.json()) as { id: string; name: string; currencyId: string; accountIds: string[] }
      log.push({ path: 'create-budget', body: body.accountIds })
      const meta = { id: body.id, ownerUserId: 'u1', name: body.name, startedAt: '2026-11-01 00:00:00', endedAt: '', currencyId: body.currencyId, isArchived: 0, periodStartDay: 1, access: [{ user: owner, role: 'owner', isAccepted: 1 }] }
      state.budgets.push(meta)
      // the server makes a new budget the user's default
      setOption('budget', body.id)
      return ok({ item: { meta } })
    }),
    // get-budget-plan lists both kinds of categories, planned "" when unset
    http.get('*/api/v1/budget/get-budget-plan', ({ request }) => {
      const url = new URL(request.url)
      const meta = state.budgets.find((b) => b.id === url.searchParams.get('id'))
      const from = url.searchParams.get('from') ?? ''
      const elements = state.categories.map((c, i) => ({
        id: c.id, type: c.type === 'income' ? 3 : 1, name: c.name, icon: c.icon, currencyId: 'cur-usd', isArchived: 0, folderId: null, position: i, ownerUserId: 'u1',
        cells: [{ actual: '0', planned: state.plans[c.id as string] ?? '', upcoming: '0' }], children: [],
      }))
      return ok({
        item: {
          meta, months: [from], openingBalances: [], savingsOpeningBalances: [], currencyRates: [{ period: from, rates: [] }], transfers: [{ period: from, items: [] }],
          savingsFlows: [], savingsIncomeExpense: [], structure: { folders: [], savings: [], elements },
        },
      })
    }),
    http.post('*/api/v1/budget/set-limit', async ({ request }) => {
      const body = (await request.json()) as { elementId: string; amount: string }
      log.push({ path: 'set-limit', body: [body.elementId, body.amount] })
      state.plans[body.elementId] = body.amount
      return ok()
    }),
  )
  return { log, state }
}

it('a registered user without the option walks every step and the answers and completion are stored', { timeout: 20000 }, async () => {
  const { log, state } = serveRegisteredUser()
  const user = userEvent.setup()
  renderPage()
  expect(await heading('Bring in your history')).toBeInTheDocument()
  expect(screen.getByRole('heading', { level: 1, name: 'Getting started' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: "I don't have one" }))

  expect(await heading('What do you spend on?')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Continue' }))

  expect(await heading("Where's your money right now?")).toBeInTheDocument()
  expect(screen.getByRole('textbox', { name: 'Account name' })).toHaveValue('Cash')
  await user.type(screen.getByRole('textbox', { name: 'Balance' }), '100')
  await user.click(screen.getByRole('button', { name: 'Continue' }))

  expect(await heading('Plan this month')).toBeInTheDocument()
  const salary = await screen.findByRole('textbox', { name: 'Salary' }, { timeout: 5000 })
  expect(within(screen.getByTestId('group-income')).getAllByRole('textbox').map((i) => i.getAttribute('aria-label'))).toEqual(['Salary', 'Bonus', 'Other income'])
  expect(salary).toHaveValue('')
  expect(screen.getByRole('textbox', { name: 'Groceries' })).toHaveValue('')
  await user.type(salary, '3000')
  await user.type(screen.getByRole('textbox', { name: 'Groceries' }), '400')
  await user.click(screen.getByRole('button', { name: 'Save plan' }))

  expect(await screen.findByText('BUDGET ROUTE', {}, { timeout: 5000 })).toBeInTheDocument()
  const salaryId = state.categories.find((c) => c.name === 'Salary')?.id
  const groceriesId = state.categories.find((c) => c.name === 'Groceries')?.id
  expect(log).toEqual([
    { path: 'update-getting-started', body: '!import' },
    { path: 'update-getting-started', body: '!import,categories' },
    { path: 'update-getting-started', body: '!import,categories,accounts' },
    { path: 'create-budget', body: ['acc-15'] },
    { path: 'set-limit', body: [salaryId, '3000'] },
    { path: 'set-limit', body: [groceriesId, '400'] },
    { path: 'update-getting-started', body: '!import,categories,accounts,plan' },
    { path: 'complete-onboarding' },
  ])
  expect(state.options.find((o) => o.name === 'onboarding')?.value).toBe('completed')
})
