import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureCategories, fixtureEur, fixtureUser, fixtureUsd, fixtureWireBudget } from '@/test/fixtures'
import { HomePage } from '@/features/home/HomePage'
import { AI_REFRESH_DELAYS_MS, cancelAiRefresh } from './finishImport'
import { OnboardingPage } from './OnboardingPage'

let importedRows = 0
vi.mock('@/features/transactions/ImportCsvDialog', () => ({
  ImportCsvDialog: ({ open, onComplete }: { open: boolean; onComplete: (r: { imported: number }) => void }) =>
    open ? <button onClick={() => onComplete({ imported: importedRows })}>finish import</button> : null,
}))

// the onboarding option: "completed", else the answered steps ("" = none yet, stored as "started")
const userWith = (steps: string, completed = false) => ({
  ...fixtureUser,
  options: fixtureUser.options.map((o) => (o.name === 'onboarding' ? { ...o, value: completed ? 'completed' : steps || 'started' } : o)),
})

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/onboarding', element: <OnboardingPage /> },
      { path: '/budget', element: <div>BUDGET ROUTE</div> },
      { path: '/settings/categories', element: <div>CATEGORIES SETTINGS</div> },
      { path: '/settings/apple-wallet', element: <div>WALLET SETTINGS</div> },
      { path: '/settings/simplefin', element: <div>SIMPLEFIN SETTINGS</div> },
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

let posts: { path: string; value?: string; body?: unknown }[]

const markStep = (stored: string, step: string) =>
  [...stored.split(',').filter((k) => k !== '' && k !== 'started' && k !== step && k !== `!${step}`), step].join(',')

// get-user-data serves the stored option, an update-onboarding POST replaces it
function serve(initial: string) {
  posts = []
  const state = { stored: initial, completed: false, failComplete: false, budgets: [fixtureWireBudget.meta] as { id: string }[], categoryListCalls: 0, imported: false }
  server.use(...coreHandlers({ budgets: [], categories: [] }))
  // added later, so these answer ahead of the core handlers
  server.use(
    http.get('*/api/v1/budget/get-budget-list', () => ok({ items: state.budgets })),
    // the step routes mark their step server-side and answer with the user
    ...(['categories', 'accounts', 'plan'] as const).map((step) =>
      http.post(`*/api/v1/onboarding/save-${step}`, async ({ request }) => {
        const body = (await request.json()) as Record<string, unknown>
        posts.push({ path: `save-${step}`, body })
        state.stored = markStep(state.stored, step)
        return ok({ user: userWith(state.stored), budgetId: 'b1', ...(step === 'plan' ? {} : { [step]: {} }) })
      }),
    ),
    // finish-import as the server answers it: only `import` is marked, the budget is the existing one
    http.post('*/api/v1/onboarding/finish-import', async ({ request }) => {
      const body = await request.json()
      posts.push({ path: 'finish-import', body })
      state.stored = markStep(state.stored, 'import')
      state.imported = true
      return ok({ user: userWith(state.stored), budgetId: fixtureWireBudget.meta.id })
    }),
    // the CSV import brought these categories in
    http.get('*/api/v1/category/get-category-list', () => {
      state.categoryListCalls++
      return ok({ items: state.imported ? fixtureCategories : [] })
    }),
    http.get('*/api/v1/budget/get-budget', () => ok({ item: fixtureWireBudget })),
    http.get('*/api/v1/user/get-user-data', () => ok({ user: userWith(state.stored, state.completed) })),
    http.post('*/api/v1/user/update-onboarding', async ({ request }) => {
      const { value } = (await request.json()) as { value: string }
      posts.push({ path: 'update-onboarding', value })
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
      return ok({ user: userWith(state.stored, true) })
    }),
  )
  return state
}

function mockViewport(compact: boolean) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023') ? compact : false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

afterEach(() => cancelAiRefresh())

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
  expect(posts).toEqual([{ path: 'update-onboarding', value: '!import' }])
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
  expect(posts).toEqual([{ path: 'update-onboarding', value: '!accounts' }])
})

it('Continue on Categories ticks the step from the user save-categories answers with, writing no option', async () => {
  serve('import')
  // a refetched user would still show the step open: the tick can only come from the answer
  server.use(http.get('*/api/v1/user/get-user-data', () => ok({ user: userWith('import') })))
  const user = userEvent.setup()
  renderPage()
  await heading('What do you spend on?')
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  expect(await heading("Where's your money right now?")).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /Categories/ })).toHaveAttribute('data-state', 'done')
  expect(posts.map((p) => p.path)).toEqual(['save-categories'])
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
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
  // the server marks the plan step itself: the client writes no option for it
  expect(posts).toEqual([
    { path: 'save-plan', body: { id: expect.any(String), budgetId: 'b1', amounts: [] } },
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

describe('Connect your accounts', () => {
  const MAC_CHROME = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36'
  beforeEach(() => {
    vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue(MAC_CHROME)
  })
  afterEach(() => {
    vi.restoreAllMocks()
    window.econumoConfig = {}
  })
  const connectHeading = () => heading('Connect your accounts')

  it('with Categories and Accounts done, the Apple Wallet link skips the open steps, completes onboarding and opens its settings', async () => {
    const state = serve('import,categories,accounts')
    const user = userEvent.setup()
    renderPage()
    await heading('Plan this month')
    await user.click(screen.getByRole('button', { name: /Connect/ }))
    await connectHeading()
    await user.click(screen.getByRole('button', { name: 'Set up Apple Wallet' }))
    expect(await screen.findByText('WALLET SETTINGS')).toBeInTheDocument()
    expect(screen.queryByText('BUDGET ROUTE')).not.toBeInTheDocument()
    expect(posts.at(-1)).toEqual({ path: 'complete-onboarding' })
    expect(state.stored.split(',').sort()).toEqual(['!plan', 'accounts', 'categories', 'connect', 'import'])
  })

  it('with Categories not done, the link only opens the settings and leaves onboarding open', async () => {
    serve('import,!categories,accounts,plan')
    const user = userEvent.setup()
    renderPage()
    await connectHeading()
    await user.click(screen.getByRole('button', { name: 'Set up Apple Wallet' }))
    expect(await screen.findByText('WALLET SETTINGS')).toBeInTheDocument()
    expect(posts).toEqual([])
  })

  it('shows SimpleFIN on a secure page and the MCP address, and Complete setup finishes on the budget', async () => {
    window.econumoConfig = { IMPORT_SIMPLEFIN: true, MCP_URL: `/mcp` }
    Object.defineProperty(window, 'isSecureContext', { value: true, configurable: true })
    const state = serve('import,categories,accounts,plan')
    const user = userEvent.setup()
    renderPage()
    await connectHeading()
    expect(screen.getByText(`/mcp`)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Set up SimpleFIN' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'SimpleFIN Bridge website' })).toHaveAttribute('href', 'https://beta-bridge.simplefin.org/')
    // the page's own Complete setup gives way to the step's
    expect(screen.getAllByRole('button', { name: 'Complete setup' })).toHaveLength(1)
    await user.click(screen.getByRole('button', { name: 'Complete setup' }))
    expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
    expect(state.stored.split(',').sort()).toEqual(['accounts', 'categories', 'connect', 'import', 'plan'])
    Object.defineProperty(window, 'isSecureContext', { value: false, configurable: true })
  })

  it('Complete setup on a reopened skipped Connect marks it done and skips the rest', async () => {
    const state = serve('import,categories,accounts,!apple_wallet')
    const user = userEvent.setup()
    renderPage()
    await heading('Plan this month')
    await user.click(screen.getByRole('button', { name: /Connect/ }))
    await connectHeading()
    await user.click(screen.getByRole('button', { name: 'Complete setup' }))
    expect(await screen.findByText('BUDGET ROUTE')).toBeInTheDocument()
    expect(state.stored.split(',').sort()).toEqual(['!apple_wallet', '!plan', 'accounts', 'categories', 'connect', 'import'])
  })

  it('a failed completion shows the error and stays; Finish then lands on the Wallet settings', async () => {
    const state = serve('import,categories,accounts,plan')
    state.failComplete = true
    const user = userEvent.setup()
    renderPage()
    await connectHeading()
    await user.click(screen.getByRole('button', { name: 'Set up Apple Wallet' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Try again')
    expect(screen.queryByText('WALLET SETTINGS')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Finish' }))
    expect(await screen.findByText('WALLET SETTINGS')).toBeInTheDocument()
  })
})

it('an import with rows finishes it on the server and opens Categories and Accounts as review steps, ticking neither', async () => {
  serve('')
  importedRows = 12
  const user = userEvent.setup()
  renderPage()
  await heading('Bring in your history')
  await user.click(screen.getByRole('button', { name: 'Choose a CSV file' }))
  await user.click(await screen.findByRole('button', { name: 'finish import' }))
  expect(await heading('What do you spend on?')).toBeInTheDocument()
  expect(posts).toEqual([{ path: 'finish-import', body: { id: expect.any(String) } }])
  expect(screen.getByRole('button', { name: /Import/ })).toHaveAttribute('data-state', 'done')
  expect(screen.getByRole('button', { name: /Categories/ })).toHaveAttribute('aria-current', 'step')
  expect(screen.getByRole('button', { name: /Accounts/ })).not.toHaveAttribute('data-state', 'done')

  // the imported categories are reviewed: archive one, the archived one stays archived
  await user.click(await screen.findByRole('button', { name: 'Archive Food' }))
  expect(screen.getByRole('button', { name: 'Restore Old' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  expect(await heading("Where's your money right now?")).toBeInTheDocument()
  expect(posts[1]).toMatchObject({ path: 'save-categories', body: { categories: [], archive: ['cat-food'] } })
  expect((posts[1].body as Record<string, unknown>).restore).toBeUndefined()

  // the accounts the import created are reviewed: one goes to the hidden Archive folder
  await user.click(await screen.findByRole('button', { name: 'Archive Cash' }))
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))
  expect(await heading('Plan this month')).toBeInTheDocument()
  expect(posts[2]).toMatchObject({ path: 'save-accounts', body: { accounts: [], hide: ['a1'] } })
  expect(posts.filter((p) => p.path === 'update-onboarding')).toEqual([])
})

it('a failed finish-import stays on the import step with a retry that replays the same operation id', async () => {
  serve('')
  importedRows = 5
  let fail = true
  const ids: string[] = []
  server.use(
    http.post('*/api/v1/onboarding/finish-import', async ({ request }) => {
      ids.push(((await request.json()) as { id: string }).id)
      if (fail) {
        fail = false
        return HttpResponse.json({ success: false, message: 'boom', code: 0, exceptionType: 'Error' }, { status: 500 })
      }
      return ok({ user: userWith('import'), budgetId: fixtureWireBudget.meta.id })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await heading('Bring in your history')
  await user.click(screen.getByRole('button', { name: 'Choose a CSV file' }))
  await user.click(await screen.findByRole('button', { name: 'finish import' }))
  expect(await screen.findByRole('alert')).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'Bring in your history' })).toBeInTheDocument()
  // the rows are already in: importing again before the retry would duplicate them
  expect(screen.getByRole('button', { name: 'Choose a CSV file' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: 'Try again' }))
  expect(await heading('What do you spend on?')).toBeInTheDocument()
  expect(ids).toHaveLength(2)
  expect(ids[1]).toBe(ids[0])
})

async function importAndCountRefetches(ai: boolean) {
  window.econumoConfig = { AI_ENABLED: ai }
  const state = serve('')
  importedRows = 3
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  renderPage()
  await heading('Bring in your history')
  await user.click(screen.getByRole('button', { name: 'Choose a CSV file' }))
  await user.click(await screen.findByRole('button', { name: 'finish import' }))
  expect(await heading('What do you spend on?')).toBeInTheDocument()
  await screen.findByRole('button', { name: 'Archive Food' })
  return state
}

it('with AI, refetches the categories at each AI refresh delay after the import, and no more', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    const state = await importAndCountRefetches(true)
    const before = state.categoryListCalls
    let elapsed = 0
    for (const [i, delay] of AI_REFRESH_DELAYS_MS.entries()) {
      await vi.advanceTimersByTimeAsync(delay - elapsed)
      elapsed = delay
      await waitFor(() => expect(state.categoryListCalls).toBe(before + i + 1))
    }
    await vi.advanceTimersByTimeAsync(120_000)
    expect(state.categoryListCalls).toBe(before + AI_REFRESH_DELAYS_MS.length)
  } finally {
    vi.useRealTimers()
  }
})

it('without AI, an import schedules no extra refetches', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    const state = await importAndCountRefetches(false)
    const before = state.categoryListCalls
    await vi.advanceTimersByTimeAsync(120_000)
    expect(state.categoryListCalls).toBe(before)
  } finally {
    vi.useRealTimers()
  }
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

// A user exactly as registration leaves them: onboarding "started". Every handler answers in the server's shape and keeps
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
  const markStep = (step: string) => {
    const value = state.options.find((o) => o.name === 'onboarding')?.value ?? ''
    setOption('onboarding', [...value.split(',').filter((k) => k !== '' && k !== 'started' && k !== step && k !== `!${step}`), step].join(','))
  }
  server.use(...coreHandlers({ user: user(), budgets: [], categories: [], accounts: [], folders: [] }))
  server.use(
    http.get('*/api/v1/user/get-user-data', () => ok({ user: user() })),
    http.post('*/api/v1/user/update-onboarding', async ({ request }) => {
      const { value } = (await request.json()) as { value: string }
      log.push({ path: 'update-onboarding', body: value })
      setOption('onboarding', value)
      return ok({ user: user() })
    }),
    http.post('*/api/v1/user/complete-onboarding', () => {
      log.push({ path: 'complete-onboarding' })
      setOption('onboarding', 'completed')
      return ok({ user: user() })
    }),
    http.get('*/api/v1/category/get-category-list', () => ok({ items: state.categories })),
    http.get('*/api/v1/account/get-folder-list', () => ok({ items: state.folders })),
    http.get('*/api/v1/account/get-account-list', () => ok({ items: state.accounts })),
    http.get('*/api/v1/budget/get-budget-list', () => ok({ items: state.budgets })),
    // save-categories as the server answers it: the rows created in one transaction, the step marked
    http.post('*/api/v1/onboarding/save-categories', async ({ request }) => {
      const body = (await request.json()) as { id: string; categories: { key: string; name: string; type: string; icon: string }[] }
      log.push({ path: 'save-categories', body: body.categories.length })
      const ids: Record<string, string> = {}
      for (const row of body.categories) {
        const item = { id: nextId('cat'), ownerUserId: 'u1', name: row.name, position: state.categories.length, type: row.type, icon: row.icon, isArchived: 0, createdAt: '2026-11-08 21:56:10', updatedAt: '2026-11-08 21:56:10' }
        state.categories.push(item)
        ids[row.key] = item.id
      }
      markStep('categories')
      return ok({ user: user(), categories: ids, budgetId: '' })
    }),
    // save-accounts creates the accounts, the "General" folder and, with no budget yet, the budget over them
    http.post('*/api/v1/onboarding/save-accounts', async ({ request }) => {
      const body = (await request.json()) as { accounts: { key: string; name: string; currencyId: string; balance: string; icon: string }[]; currencyIds: string[]; mainCurrencyId: string }
      log.push({ path: 'save-accounts', body: { accounts: body.accounts.map((a) => [a.name, a.balance]), currencyIds: body.currencyIds, main: body.mainCurrencyId } })
      if (state.folders.length === 0) state.folders.push({ id: 'folder-general', name: 'General', position: 0, isVisible: 1 })
      const ids: Record<string, string> = {}
      for (const row of body.accounts) {
        const id = nextId('acc')
        state.accounts.push({
          id, owner, folderId: 'folder-general', name: row.name, position: state.accounts.length,
          currency: row.currencyId === 'cur-eur' ? fixtureEur : fixtureUsd, balance: row.balance, type: 2, icon: row.icon,
          sharedAccess: [], transactionsRevision: 1, transactionsCount: 1,
        })
        ids[row.key] = id
      }
      const budgetId = 'b-onb'
      state.budgets.push({ id: budgetId, ownerUserId: 'u1', name: 'My budget', startedAt: '2026-11-01 00:00:00', endedAt: '', currencyId: body.mainCurrencyId, isArchived: 0, periodStartDay: 1, access: [{ user: owner, role: 'owner', isAccepted: 1 }] })
      // the server makes a new budget the user's default
      setOption('budget', budgetId)
      markStep('accounts')
      return ok({ user: user(), accounts: ids, budgetId })
    }),
    http.post('*/api/v1/onboarding/save-plan', async ({ request }) => {
      const body = (await request.json()) as { budgetId: string; amounts: { categoryId: string; amount: string }[] }
      log.push({ path: 'save-plan', body: [body.budgetId, body.amounts.map((a) => [a.categoryId, a.amount])] })
      for (const a of body.amounts) state.plans[a.categoryId] = a.amount
      markStep('plan')
      return ok({ user: user(), budgetId: body.budgetId })
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
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))

  expect(await heading("Where's your money right now?")).toBeInTheDocument()
  expect(screen.getByRole('textbox', { name: 'Account name' })).toHaveValue('Cash')
  await user.type(screen.getByRole('textbox', { name: 'Balance' }), '100')
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))

  expect(await heading('Plan this month')).toBeInTheDocument()
  const salary = await screen.findByRole('textbox', { name: 'Salary' }, { timeout: 5000 })
  expect(within(screen.getByTestId('group-income')).getAllByRole('textbox').map((i) => i.getAttribute('aria-label'))).toEqual(['Salary', 'Bonus', 'Other income'])
  expect(salary).toHaveValue('')
  expect(screen.getByRole('textbox', { name: 'Groceries' })).toHaveValue('')
  await user.type(salary, '3000')
  await user.type(screen.getByRole('textbox', { name: 'Groceries' }), '400')
  await user.click(screen.getByRole('button', { name: 'Save and continue' }))

  expect(await screen.findByText('BUDGET ROUTE', {}, { timeout: 5000 })).toBeInTheDocument()
  const salaryId = state.categories.find((c) => c.name === 'Salary')?.id
  const groceriesId = state.categories.find((c) => c.name === 'Groceries')?.id
  // Skip still writes the option; each done step is marked by its own route, never written again by the client
  expect(log).toEqual([
    { path: 'update-onboarding', body: '!import' },
    { path: 'save-categories', body: 14 },
    { path: 'save-accounts', body: { accounts: [['Cash', '100']], currencyIds: ['cur-usd'], main: 'cur-usd' } },
    { path: 'save-plan', body: ['b-onb', [[salaryId, '3000'], [groceriesId, '400']]] },
    { path: 'complete-onboarding' },
  ])
  expect(state.options.find((o) => o.name === 'onboarding')?.value).toBe('completed')
})
