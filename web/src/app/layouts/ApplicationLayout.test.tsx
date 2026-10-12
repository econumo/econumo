import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { IDBFactory } from 'fake-indexeddb'
import { delay, http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureTransactions, fixtureUser } from '@/test/fixtures'
import { QUERY_CACHE_KEY, clearPersistedQueryCache, createPersistOptions, refreshRestoredQueries } from '@/lib/queryPersist'
import { STORE_ACCOUNT_TRANSACTIONS, resetDbForTests } from '@/lib/db'
import { loadBuckets, putBuckets, resetTransactionStoreForTests } from '@/lib/transactionStore'
import { queryKeys } from '@/app/queryKeys'
import type { TransactionDto } from '@/api/dto/transaction'
import { useTransactions } from '@/features/transactions/queries'
import { installTransactionTracking, resetRevisions } from '@/features/transactions/revisionTracking'
import { econumoPackage } from '@/lib/package'
import type { AvailableUpdate } from '@/hooks/useAvailableUpdate'
import { useSidebarStore, useUiStore } from '@/app/uiStore'
import { useBudgetPeriodStore } from '@/features/budgets/budgetStore'
import { ApplicationLayout } from './ApplicationLayout'

const mockUpdate = vi.hoisted(() => ({ value: null as AvailableUpdate | null }))
vi.mock('@/hooks/useAvailableUpdate', () => ({
  useAvailableUpdate: () => mockUpdate.value,
}))

function mockViewport(compact: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: query.includes('1023') ? compact : false,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
}

function renderShell(path: string) {
  const router = createMemoryRouter(
    [
      {
        element: <ApplicationLayout />,
        children: [
          { path: '/', element: <div>HOME CONTENT</div> },
          { path: '/account/:id', element: <div>ACCOUNT CONTENT</div> },
        ],
      },
    ],
    { initialEntries: [path] },
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

function TransactionProbe() {
  const { data } = useTransactions()
  return <ul>{data?.map((t) => <li key={t.id}>{t.description || t.id}</li>)}</ul>
}

const uninstalls: (() => void)[] = []

function renderShellPersisted(home: ReactNode = <div>HOME CONTENT</div>) {
  const router = createMemoryRouter(
    [{ element: <ApplicationLayout />, children: [{ path: '/', element: home }] }],
    { initialEntries: ['/'] },
  )
  // a fresh QueryClient per call = a page reload (in-memory cache is gone)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  uninstalls.push(installTransactionTracking(queryClient))
  const view = render(
    <PersistQueryClientProvider
      client={queryClient}
      persistOptions={createPersistOptions()}
      onSuccess={() => refreshRestoredQueries(queryClient)}
    >
      <RouterProvider router={router} />
    </PersistQueryClientProvider>,
  )
  return { ...view, queryClient }
}

// the persister throttles its writes by a second
const PERSIST_WAIT = { timeout: 3000 }

function persistedQueryKeys(): unknown[][] {
  const raw = localStorage.getItem(QUERY_CACHE_KEY)
  return raw ? (JSON.parse(raw) as { clientState: { queries: { queryKey: unknown[] }[] } }).clientState.queries.map((q) => q.queryKey) : []
}

// boot #1 on a cold cache: everything downloads, the snapshot and the buckets are written
async function coldBoot() {
  const first = renderShellPersisted(<TransactionProbe />)
  expect(await screen.findByText('coffee beans')).toBeInTheDocument()
  await waitFor(() => expect(persistedQueryKeys()).toContainEqual(['accounts']), PERSIST_WAIT)
  await waitFor(() => expect(persistedQueryKeys()).toContainEqual(['user']), PERSIST_WAIT)
  await waitFor(async () => expect((await loadBuckets(fixtureUser.id)).size).toBe(fixtureAccounts.length))
  first.unmount()
  uninstalls.splice(0).forEach((u) => u())
  // a reload starts with an empty module state
  resetRevisions()
}

beforeEach(() => {
  localStorage.clear()
  globalThis.indexedDB = new IDBFactory()
  resetDbForTests()
  resetTransactionStoreForTests()
  resetRevisions()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  mockUpdate.value = null
  // the sidebar-collapsed flag and the search dialog live in module-level
  // zustand stores, so they survive across tests in this file independent of
  // localStorage.clear()
  useSidebarStore.setState({ collapsed: false })
  useUiStore.setState({ searchOpen: false })
  useBudgetPeriodStore.setState({ lastMode: 'budget' })
})

afterEach(() => {
  server.events.removeAllListeners()
  uninstalls.splice(0).forEach((u) => u())
  vi.restoreAllMocks()
})

it('the Budget & Plan link opens the budget view last opened on this device', async () => {
  mockViewport(false)
  renderShell('/')
  expect(await screen.findByRole('link', { name: 'Budget & Plan' })).toHaveAttribute('href', '/budget')
  useBudgetPeriodStore.setState({ lastMode: 'plan' })
  await waitFor(() => expect(screen.getByRole('link', { name: 'Budget & Plan' })).toHaveAttribute('href', '/plan'))
})

it('sizes the shell with dvh, never svh', async () => {
  mockViewport(false)
  const { container } = renderShell('/')
  expect(await screen.findByText('Cash')).toBeInTheDocument()
  // iOS caches svh per window and hands a standalone PWA a stale, too-short
  // one after Apple's cross-site POST callback; dvh stays the real viewport.
  const shell = container.firstElementChild as HTMLElement
  expect(shell.className).toContain('h-dvh')
  expect(shell.className).not.toContain('h-svh')
})

it('shows the loading gate, then the sidebar tree with folder totals', async () => {
  mockViewport(false)
  renderShell('/')
  expect(screen.getByText('Loading your data')).toBeInTheDocument()

  expect(await screen.findByText('Cash')).toBeInTheDocument()
  expect(screen.getByText('General')).toBeInTheDocument()
  expect(screen.getByText('Savings')).toBeInTheDocument()
  // hidden folder and its account are absent
  expect(screen.queryByText('Hidden')).not.toBeInTheDocument()
  expect(screen.queryByText('Under the mattress')).not.toBeInTheDocument()
  // mixed-currency folder total: 2000 USD + 90 EUR / 0.9 = 2100 USD
  expect(screen.getByText('2,100.00 $')).toBeInTheDocument()
  // single-currency folder total (and the matching account row): native
  expect(screen.getAllByText('100.50 $').length).toBeGreaterThanOrEqual(2)
  // top row + nav
  expect(screen.queryByText('Ada')).not.toBeInTheDocument()
  expect(screen.queryByText(fixtureUser.email)).not.toBeInTheDocument()
  expect(screen.getByText('Budget & Plan')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Settings' })).toBeInTheDocument()
})

it('shows the Inbox link in the full sidebar, and between the Home mark and Budget links in the icon rail', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  renderShell('/account/a1')
  expect(await screen.findByText('Cash')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Inbox' })).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'toggle sidebar' }))
  expect(screen.queryByText('Cash')).not.toBeInTheDocument()
  const railLinks = screen.getAllByRole('link')
  const homeIndex = railLinks.findIndex((link) => link.getAttribute('href') === '/')
  const inboxIndex = railLinks.findIndex((link) => link.getAttribute('href') === '/inbox')
  const budgetIndex = railLinks.findIndex((link) => link.getAttribute('href') === '/budget')
  expect(homeIndex).toBeGreaterThanOrEqual(0)
  expect(inboxIndex).toBeGreaterThan(homeIndex)
  expect(budgetIndex).toBeGreaterThan(inboxIndex)

  const search = screen.getByRole('button', { name: 'Search' })
  const inbox = screen.getByRole('link', { name: 'Inbox' })
  expect(search.compareDocumentPosition(inbox) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

it('puts Inbox above the onboarding link in the icon rail when onboarding is incomplete', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  server.use(...coreHandlers({ user: { ...fixtureUser, options: [...fixtureUser.options.filter((o) => o.name !== 'onboarding'), { name: 'onboarding', value: 'started' }] } }))
  renderShell('/account/a1')
  expect(await screen.findByText('Cash')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'toggle sidebar' }))
  expect(screen.queryByText('Cash')).not.toBeInTheDocument()
  const railLinks = screen.getAllByRole('link')
  const homeIndex = railLinks.findIndex((link) => link.getAttribute('href') === '/')
  const inboxIndex = railLinks.findIndex((link) => link.getAttribute('href') === '/inbox')
  const onboardingIndex = railLinks.findIndex((link) => link.getAttribute('href') === '/onboarding')
  const budgetIndex = railLinks.findIndex((link) => link.getAttribute('href') === '/budget')
  expect(homeIndex).toBeGreaterThanOrEqual(0)
  expect(onboardingIndex).toBeGreaterThanOrEqual(0)
  expect(inboxIndex).toBeGreaterThan(homeIndex)
  expect(onboardingIndex).toBeGreaterThan(inboxIndex)
  expect(budgetIndex).toBeGreaterThan(onboardingIndex)

  const search = screen.getByRole('button', { name: 'Search' })
  const inbox = screen.getByRole('link', { name: 'Inbox' })
  const onboarding = screen.getByRole('link', { name: 'Getting started' })
  expect(search.compareDocumentPosition(inbox) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(inbox.compareDocumentPosition(onboarding) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

it('the top row shows a Home link with the logo and version, a Search button and Inbox, and no user name/profile link', async () => {
  mockViewport(false)
  renderShell('/')
  expect(await screen.findByText('Cash')).toBeInTheDocument()
  const topRow = screen.getByTestId('sidebar-top-row')
  const homeLink = within(topRow).getByRole('link', { name: 'Econumo' })
  expect(homeLink).toHaveAttribute('href', '/')
  expect(within(homeLink).getByText(econumoPackage().label)).toBeInTheDocument()
  expect(within(topRow).getByRole('button', { name: 'Search' })).toBeInTheDocument()
  expect(within(topRow).getByRole('link', { name: 'Inbox' })).toBeInTheDocument()
  const profileLinks = screen.queryAllByRole('link').filter((link) => link.getAttribute('href') === '/settings/profile')
  expect(profileLinks).toHaveLength(0)
  expect(screen.queryByText('Ada')).not.toBeInTheDocument()
  expect(screen.queryByText(fixtureUser.email)).not.toBeInTheDocument()
})

it('clicking the top row Search button opens global search', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  renderShell('/')
  expect(await screen.findByText('Cash')).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() => expect(useUiStore.getState().searchOpen).toBe(true))
})

it('a reload with a persisted cache skips the boot loader and refreshes in the background', async () => {
  mockViewport(false)
  let accountFetches = 0
  server.use(
    http.get('*/api/v1/account/get-account-list', () => {
      accountFetches++
      return HttpResponse.json({ success: true, message: '', data: { items: fixtureAccounts } })
    }),
  )
  // boot #1: cold cache — the gate shows, data loads from the network and persists
  const first = renderShellPersisted()
  expect(await screen.findByText('Cash')).toBeInTheDocument()
  await waitFor(() => expect(localStorage.getItem(QUERY_CACHE_KEY)).not.toBeNull())
  const coldBootFetches = accountFetches
  first.unmount()

  // boot #2: fresh QueryClient (= page reload) — restored from localStorage,
  // data on screen at once, no blocking gate, and a background refetch fires
  // even though staleTime still considers the restored data fresh
  renderShellPersisted()
  expect(screen.queryByText('Loading your data')).not.toBeInTheDocument()
  expect(await screen.findByText('Cash')).toBeInTheDocument()
  expect(screen.queryByText('Loading your data')).not.toBeInTheDocument()
  await waitFor(() => expect(accountFetches).toBeGreaterThan(coldBootFetches))
})

it('a reload starts from the stored transactions, before any response arrives', async () => {
  mockViewport(false)
  await coldBoot()

  // boot #2: the backend never answers — the restored snapshot and the buckets carry the page
  server.use(http.all('*/api/*', async () => {
    await delay('infinite')
    return HttpResponse.error()
  }))
  renderShellPersisted(<TransactionProbe />)
  expect(screen.queryByText('Loading your data')).not.toBeInTheDocument()
  expect(await screen.findByText('coffee beans')).toBeInTheDocument()
  expect(screen.getByText('Cash')).toBeInTheDocument()
  expect(screen.queryByText('Loading your data')).not.toBeInTheDocument()
})

it('a reload with unchanged revisions downloads no transactions and writes no bucket', async () => {
  mockViewport(false)
  await coldBoot()

  const downloads: string[] = []
  server.events.on('request:start', ({ request }) => {
    const path = new URL(request.url).pathname
    if (path.includes('/api/v1/transaction/')) {
      downloads.push(path)
    }
  })
  const put = vi.spyOn(IDBObjectStore.prototype, 'put')
  let accountFetches = 0
  server.use(
    http.get('*/api/v1/account/get-account-list', () => {
      accountFetches++
      return HttpResponse.json({ success: true, message: '', data: { items: fixtureAccounts } })
    }),
  )
  const { queryClient } = renderShellPersisted(<TransactionProbe />)
  expect(await screen.findByText('coffee beans')).toBeInTheDocument()
  // the background refresh ran its sync to the end
  await waitFor(() => expect(accountFetches).toBeGreaterThan(0))
  await waitFor(() => expect(queryClient.isFetching()).toBe(0))
  // past the write-through debounce
  await new Promise((r) => setTimeout(r, 700))
  expect(downloads).toEqual([])
  expect(put.mock.contexts.filter((store) => (store as IDBObjectStore).name === STORE_ACCOUNT_TRANSACTIONS)).toEqual([])
})

it('a local edit right after a reload writes only the touched account', async () => {
  mockViewport(false)
  server.use(...coreHandlers({
    transactions: [...fixtureTransactions, { ...fixtureTransactions[0], id: 't-bank', accountId: 'a2', description: 'bank fee' }],
  }))
  await coldBoot()

  server.use(http.all('*/api/*', async () => {
    await delay('infinite')
    return HttpResponse.error()
  }))
  const { queryClient } = renderShellPersisted(<TransactionProbe />)
  expect(await screen.findByText('coffee beans')).toBeInTheDocument()
  const put = vi.spyOn(IDBObjectStore.prototype, 'put')
  queryClient.setQueryData<TransactionDto[]>(queryKeys.transactions, (prev) =>
    prev?.map((t) => (t.id === 't1' ? { ...t, description: 'tea leaves' } : t)))
  expect(await screen.findByText('tea leaves')).toBeInTheDocument()
  await waitFor(() => expect(put).toHaveBeenCalled(), { timeout: 2000 })
  const written = put.mock.calls
    .filter((_, i) => (put.mock.contexts[i] as IDBObjectStore).name === STORE_ACCOUNT_TRANSACTIONS)
    .map(([bucket]) => (bucket as { accountId: string }).accountId)
  expect(written).toEqual(['a1'])
})

it('never keeps transactions in the localStorage snapshot', async () => {
  mockViewport(false)
  renderShellPersisted(<TransactionProbe />)
  expect(await screen.findByText('coffee beans')).toBeInTheDocument()
  await waitFor(() => expect(persistedQueryKeys()).toContainEqual(['accounts']), PERSIST_WAIT)
  await waitFor(() => expect(persistedQueryKeys()).toContainEqual(['user']), PERSIST_WAIT)
  expect(persistedQueryKeys()).not.toContainEqual(queryKeys.transactions)
})

it('a snapshot of another user never shows the stored transactions', async () => {
  mockViewport(false)
  await coldBoot()

  // the snapshot now belongs to u2 (a 401 sent u1 to the login page, u2 signed in without a clear)
  const raw = JSON.parse(localStorage.getItem(QUERY_CACHE_KEY)!) as { clientState: { queries: { queryKey: unknown[]; state: { data: unknown } }[] } }
  for (const q of raw.clientState.queries) {
    if (q.queryKey[0] === 'user') {
      q.state.data = { ...fixtureUser, id: 'u2' }
    }
  }
  localStorage.setItem(QUERY_CACHE_KEY, JSON.stringify(raw))
  server.use(http.all('*/api/*', async () => {
    await delay('infinite')
    return HttpResponse.error()
  }))
  const { queryClient } = renderShellPersisted(<TransactionProbe />)
  // nothing to start from: the first boot's loader waits for the download
  expect(await screen.findByText('Loading your data')).toBeInTheDocument()
  expect(queryClient.getQueryData(queryKeys.transactions)).toBeUndefined()
  expect(screen.queryByText('coffee beans')).not.toBeInTheDocument()
})

it('clearPersistedQueryCache empties the stored transactions and the snapshot', async () => {
  localStorage.setItem(QUERY_CACHE_KEY, '{}')
  await putBuckets(fixtureUser.id, [{ accountId: 'a1', revision: 1, items: [], authors: [] }])
  expect((await loadBuckets(fixtureUser.id)).size).toBe(1)

  await clearPersistedQueryCache()

  expect(localStorage.getItem(QUERY_CACHE_KEY)).toBeNull()
  expect((await loadBuckets(fixtureUser.id)).size).toBe(0)
})

it('the boot loader grows a logout escape after three seconds when the backend is unreachable', async () => {
  mockViewport(false)
  server.use(http.get('*/api/*', () => HttpResponse.error()))
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    renderShell('/')
    expect(screen.getByText('Loading your data')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Log out' })).not.toBeInTheDocument()
    await vi.advanceTimersByTimeAsync(3000)
    const link = await screen.findByRole('link', { name: 'Log out' })
    expect(link).toHaveAttribute('href', '/logout')
    // the escape floats at the bottom of the screen so its appearance never
    // shifts the loader
    expect(screen.getByText(/Having trouble\?/)).toBeInTheDocument()
    expect(link.closest('div')?.className).toContain('fixed')
  } finally {
    vi.useRealTimers()
  }
})

it('no logout escape appears once the app has loaded', async () => {
  mockViewport(false)
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    renderShell('/')
    expect(await screen.findByText('Cash')).toBeInTheDocument()
    await vi.advanceTimersByTimeAsync(3000)
    expect(screen.queryByRole('link', { name: 'Log out' })).not.toBeInTheDocument()
  } finally {
    vi.useRealTimers()
  }
})

it('the sync icon turns into an amber warning while background refreshes fail, and recovers on success', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  renderShell('/')
  expect(await screen.findByText('Cash')).toBeInTheDocument()
  const syncButton = screen.getByRole('button', { name: 'sync' })
  expect(syncButton.title).toContain('Full sync')
  expect(syncButton.className).not.toContain('amber')

  server.use(http.get('*/api/*', () => HttpResponse.error()))
  await user.click(syncButton)
  await waitFor(() => expect(syncButton.title).toContain("Can't reach the server"))
  expect(syncButton.className).toContain('amber')

  server.resetHandlers()
  server.use(...coreHandlers())
  await user.click(syncButton)
  await waitFor(() => expect(syncButton.title).toContain('Full sync'))
  expect(syncButton.className).not.toContain('amber')
})

it('desktop shows sidebar and workspace together', async () => {
  mockViewport(false)
  renderShell('/account/a1')
  await waitFor(() => expect(screen.getByTestId('sidebar')).toBeInTheDocument())
  expect(screen.getByTestId('workspace')).toBeInTheDocument()
  expect(screen.getByText('ACCOUNT CONTENT')).toBeInTheDocument()
})

it('desktop divider click collapses the sidebar to an icon rail and back', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  renderShell('/account/a1')
  expect(await screen.findByText('Cash')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'toggle sidebar' }))
  // account names are gone, only icons remain
  expect(screen.queryByText('Cash')).not.toBeInTheDocument()
  expect(screen.queryByText('Budget & Plan')).not.toBeInTheDocument()
  // the account is still reachable as an icon button, the Home mark still shown
  expect(screen.getByRole('button', { name: 'Cash' })).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Econumo' })).toHaveAttribute('href', '/')

  await user.click(screen.getByRole('button', { name: 'toggle sidebar' }))
  expect(await screen.findByText('Cash')).toBeInTheDocument()
  expect(screen.getByText('Budget & Plan')).toBeInTheDocument()
})

it('compact viewport shows only the sidebar at / and only the workspace elsewhere', async () => {
  mockViewport(true)
  renderShell('/')
  await waitFor(() => expect(screen.getByTestId('sidebar')).toBeInTheDocument())
  expect(screen.queryByTestId('workspace')).not.toBeInTheDocument()
})

it('compact viewport hides the sidebar on content routes', async () => {
  mockViewport(true)
  renderShell('/account/a1')
  await waitFor(() => expect(screen.getByTestId('workspace')).toBeInTheDocument())
  expect(screen.queryByTestId('sidebar')).not.toBeInTheDocument()
})

it('shows an update dot on the full-footer Settings link when an update is available', async () => {
  mockViewport(false)
  mockUpdate.value = { version: 'v9.9.9', url: 'https://econumo.com/releases/v9.9.9/' }
  renderShell('/')
  expect(await screen.findByText('Cash')).toBeInTheDocument()
  const settingsLink = screen.getByRole('link', { name: 'Settings' })
  expect(settingsLink.querySelector('[data-testid="update-dot"]')).toBeInTheDocument()
})

it('shows no update dot on the full-footer Settings link when no update is available', async () => {
  mockViewport(false)
  renderShell('/')
  expect(await screen.findByText('Cash')).toBeInTheDocument()
  const settingsLink = screen.getByRole('link', { name: 'Settings' })
  expect(settingsLink.querySelector('[data-testid="update-dot"]')).not.toBeInTheDocument()
})

it('shows an update dot on the icon-rail Settings gear when an update is available', async () => {
  mockViewport(false)
  mockUpdate.value = { version: 'v9.9.9', url: 'https://econumo.com/releases/v9.9.9/' }
  const user = userEvent.setup()
  renderShell('/account/a1')
  expect(await screen.findByText('Cash')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'toggle sidebar' }))
  expect(screen.queryByText('Cash')).not.toBeInTheDocument()
  expect(screen.getByTestId('update-dot')).toBeInTheDocument()
})

it('shows no update dot on the icon-rail Settings gear when no update is available', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  renderShell('/account/a1')
  expect(await screen.findByText('Cash')).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'toggle sidebar' }))
  expect(screen.queryByText('Cash')).not.toBeInTheDocument()
  expect(screen.queryByTestId('update-dot')).not.toBeInTheDocument()
})
