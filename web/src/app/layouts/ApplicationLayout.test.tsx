import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client'
import { createSyncStoragePersister } from '@tanstack/query-sync-storage-persister'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureUser } from '@/test/fixtures'
import { QUERY_CACHE_KEY, refreshRestoredQueries } from '@/lib/queryPersist'
import type { AvailableUpdate } from '@/hooks/useAvailableUpdate'
import { useSidebarStore } from '@/app/uiStore'
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
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

function renderShellPersisted() {
  const router = createMemoryRouter(
    [{ element: <ApplicationLayout />, children: [{ path: '/', element: <div>HOME CONTENT</div> }] }],
    { initialEntries: ['/'] },
  )
  const persister = createSyncStoragePersister({ storage: localStorage, key: QUERY_CACHE_KEY, throttleTime: 0 })
  // a fresh QueryClient per call = a page reload (in-memory cache is gone)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <PersistQueryClientProvider
      client={queryClient}
      persistOptions={{ persister }}
      onSuccess={() => refreshRestoredQueries(queryClient)}
    >
      <RouterProvider router={router} />
    </PersistQueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  mockUpdate.value = null
  // the sidebar-collapsed flag lives in a module-level zustand store, so it
  // survives across tests in this file independent of localStorage.clear()
  useSidebarStore.setState({ collapsed: false })
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
  // user block + nav
  expect(screen.getByText('Ada')).toBeInTheDocument()
  expect(screen.getByText('Budget')).toBeInTheDocument()
  expect(screen.getByText('Settings')).toBeInTheDocument()
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
  // account names and the user name are gone, only icons remain
  expect(screen.queryByText('Cash')).not.toBeInTheDocument()
  expect(screen.queryByText('Ada')).not.toBeInTheDocument()
  expect(screen.queryByText('Budget')).not.toBeInTheDocument()
  // the account is still reachable as an icon button, avatar still shown
  expect(screen.getByRole('button', { name: 'Cash' })).toBeInTheDocument()
  expect(screen.getByTestId('user-avatar')).toHaveAttribute('data-avatar', fixtureUser.avatar)

  await user.click(screen.getByRole('button', { name: 'toggle sidebar' }))
  expect(await screen.findByText('Cash')).toBeInTheDocument()
  expect(screen.getByText('Ada')).toBeInTheDocument()
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
  const settingsLink = screen.getByText('Settings').closest('a')
  expect(settingsLink?.querySelector('[data-testid="update-dot"]')).toBeInTheDocument()
})

it('shows no update dot on the full-footer Settings link when no update is available', async () => {
  mockViewport(false)
  renderShell('/')
  expect(await screen.findByText('Cash')).toBeInTheDocument()
  const settingsLink = screen.getByText('Settings').closest('a')
  expect(settingsLink?.querySelector('[data-testid="update-dot"]')).not.toBeInTheDocument()
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
