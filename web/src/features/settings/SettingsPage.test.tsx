import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider, onlineManager } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { Toaster } from '@/components/ui/sonner'
import { useTransactions } from '@/features/transactions/queries'
import { resetRevisions } from '@/features/transactions/revisionTracking'
import { METRICS, trackEvent } from '@/lib/metrics'
import { loadBuckets } from '@/lib/transactionStore'
import { useUiStore } from '@/app/uiStore'
import type { AvailableUpdate } from '@/hooks/useAvailableUpdate'
import { SettingsPage } from './SettingsPage'

const mockUpdate = vi.hoisted(() => ({ value: null as AvailableUpdate | null }))
vi.mock('@/hooks/useAvailableUpdate', () => ({
  useAvailableUpdate: () => mockUpdate.value,
}))

vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

// the application shell keeps the transactions list mounted on every page
function TransactionsMounted() {
  useTransactions()
  return null
}

function mockViewport(compact: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: query.includes('1023') ? compact : false,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
}

function renderPage(retry: boolean | number = false) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry, retryDelay: 60_000 }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/settings', element: <><SettingsPage /><TransactionsMounted /><Toaster /></> },
      { path: '/settings/profile', element: <div>PROFILE PAGE</div> },
      { path: '/settings/budgets', element: <div>BUDGETS PAGE</div> },
      { path: '/settings/data', element: <div>DATA PAGE</div> },
      { path: '/settings/apple-wallet', element: <div>WALLET PAGE</div> },
      { path: '/logout', element: <div>LOGOUT ROUTE</div> },
    ],
    { initialEntries: ['/settings'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

beforeEach(() => {
  resetRevisions()
  vi.mocked(trackEvent).mockClear()
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  mockUpdate.value = null
  mockViewport(false)
})

it('renders the menu rows with exact labels and navigates', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText('Settings')).toBeInTheDocument()
  // Full sync moved to the sidebar footer refresh button
  expect(screen.queryByText('Full sync')).not.toBeInTheDocument()
  // grouped menu; currency moved to the profile page
  expect(screen.getByText('Finances')).toBeInTheDocument()
  expect(screen.getByText('Classification')).toBeInTheDocument()
  expect(screen.getByText('Data')).toBeInTheDocument()
  expect(screen.queryByText('Default currency')).not.toBeInTheDocument()
  expect(screen.getByText('Shared access')).toBeInTheDocument()
  expect(screen.getByText('Accounts')).toBeInTheDocument()
  expect(screen.getByText('Payees')).toBeInTheDocument()
  await user.click(screen.getByText('Budgets & Plans'))
  expect(await screen.findByText('BUDGETS PAGE')).toBeInTheDocument()
})

it('the last row opens the feedback dialog, unless the server turns feedback off', async () => {
  const user = userEvent.setup()
  useUiStore.setState({ feedbackOpen: false })
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Send feedback' }))
  expect(useUiStore.getState().feedbackOpen).toBe(true)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.FEEDBACK_OPEN)
  cleanup()

  window.econumoConfig = { FEEDBACK: false }
  renderPage()
  expect(await screen.findByText('Data')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Send feedback' })).not.toBeInTheDocument()
})

it('hides non-semver version text while linking to the Econumo website', async () => {
  window.econumoConfig = { VERSION: 'dev' }
  renderPage()
  const label = await screen.findByRole('link', { name: 'Econumo' })
  expect(label).toHaveAttribute('href', 'https://econumo.com')
})

it('links a semver version to the Econumo website', async () => {
  window.econumoConfig = { VERSION: 'v1.2.3' }
  renderPage()
  const label = await screen.findByText('Econumo v1.2.3')
  expect(label).toHaveAttribute('href', 'https://econumo.com')
})

it('links to the API docs at the configured backend host', async () => {
  window.econumoConfig = { ALLOW_CUSTOM_API: 'true' }
  localStorage.setItem('selfHosted', JSON.stringify(true))
  localStorage.setItem('backendHost', JSON.stringify('https://api.example.test'))
  renderPage()
  const link = await screen.findByRole('link', { name: 'API' })
  expect(link).toHaveAttribute('href', 'https://api.example.test/api/doc')
})

it('the Data group links to the Import & export page', async () => {
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByText('Import & export'))
  expect(await screen.findByText('DATA PAGE')).toBeInTheDocument()
})

it('the Data group always shows Apple Wallet and Import rules, SimpleFIN only while it is on', async () => {
  renderPage()
  expect(await screen.findByText('Apple Wallet')).toBeInTheDocument()
  expect(screen.getByText('Import rules')).toBeInTheDocument()
  expect(screen.queryByText('SimpleFIN')).not.toBeInTheDocument()
})

it('shows SimpleFIN while it is on', async () => {
  window.econumoConfig = { IMPORT_SIMPLEFIN: true }
  renderPage()
  expect(await screen.findByText('SimpleFIN')).toBeInTheDocument()
  expect(screen.getByText('Apple Wallet')).toBeInTheDocument()
})

it('the Data group links to the Apple Wallet page', async () => {
  window.econumoConfig = { IMPORT_SIMPLEFIN: true }
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByText('Apple Wallet'))
  expect(await screen.findByText('WALLET PAGE')).toBeInTheDocument()
})

it('shows the new-version menu row above the Finances group when an update is available', async () => {
  mockUpdate.value = { version: 'v9.9.9', url: 'https://econumo.com/releases/v9.9.9/' }
  renderPage()
  const link = await screen.findByRole('link', { name: /v9\.9\.9/ })
  expect(link).toHaveAttribute('href', 'https://econumo.com/releases/v9.9.9/')
  expect(link).toHaveAttribute('target', '_blank')
  const finances = screen.getByText('Finances')
  expect(link.compareDocumentPosition(finances) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

it('shows no new-version link when no update is available', async () => {
  renderPage()
  expect(await screen.findByText('Settings')).toBeInTheDocument()
  expect(screen.queryByText(/New version/)).not.toBeInTheDocument()
})

function utcIn(days: number): string {
  return new Date(Date.now() + days * 86_400_000).toISOString().slice(0, 19).replace('T', ' ')
}

it('hides the Billing group when BILLING_URL is empty', async () => {
  mockViewport(false)
  renderPage()
  expect(await screen.findByText('Finances')).toBeInTheDocument()
  expect(screen.queryByText('Billing')).not.toBeInTheDocument()
})

it('shows the Billing group with the portal row for full access', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  mockViewport(false)
  renderPage()
  expect(await screen.findByText('Billing')).toBeInTheDocument()
  expect(screen.getByText('Manage subscription')).toBeInTheDocument()
  expect(screen.queryByText(/Trial/)).not.toBeInTheDocument()
})

it('shows the trial status hint from day one', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(...coreHandlers({ user: { ...fixtureUser, accessUntil: utcIn(40) } }))
  mockViewport(false)
  renderPage()
  expect(await screen.findByText(/Subscription ends/)).toBeInTheDocument()
})

it('shows the read-only status hint', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(...coreHandlers({ user: { ...fixtureUser, accessLevel: 'readonly', accessUntil: '' } }))
  mockViewport(false)
  renderPage()
  expect(await screen.findByText('Billing')).toBeInTheDocument()
  expect(await screen.findByText(/^Expired$/)).toBeInTheDocument()
})

it('the header card links to the profile page and shows name, email and the Personal settings hint', async () => {
  renderPage()
  const link = await screen.findByRole('link', { name: /Ada/ })
  expect(link).toHaveAttribute('href', '/settings/profile')
  expect(within(link).getByText(fixtureUser.email)).toBeInTheDocument()
  expect(within(link).getByText('Personal settings')).toBeInTheDocument()
})

it('the Log out button opens the confirm dialog and confirming navigates to the logout route', async () => {
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Log out' }))
  expect(await screen.findByText('Are you sure you want to log out?')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Stay' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Log out' }))
  expect(await screen.findByText('LOGOUT ROUTE')).toBeInTheDocument()
})

describe('Clear cache and reload', () => {
  it('is the last row of the Data group, below the import rows', async () => {
    window.econumoConfig = { IMPORT_SIMPLEFIN: true }
    renderPage()
    const row = await screen.findByRole('button', { name: 'Clear cache and reload' })
    const group = row.closest('nav')!
    expect(group.lastElementChild).toBe(row)
    expect(within(group).getByText('Import rules')).toBeInTheDocument()
  })

  it('downloads every account again, one per request, says so and counts it', async () => {
    const user = userEvent.setup()
    const asked: (string | null)[] = []
    const record = ({ request }: { request: Request }) => {
      const url = new URL(request.url)
      if (url.pathname.endsWith('/get-account-transaction-list')) asked.push(url.searchParams.get('accountIds'))
    }
    server.events.on('request:start', record)
    onTestFinished(() => server.events.removeListener('request:start', record))
    const queryClient = renderPage()
    await waitFor(() => expect(queryClient.getQueryState(queryKeys.transactions)?.status).toBe('success'))
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    asked.length = 0

    await user.click(screen.getByRole('button', { name: 'Clear cache and reload' }))
    expect(await screen.findByText('Clearing the cache and reloading…')).toBeInTheDocument()
    // the download starts only after the store is cleared, so nothing may be fetching yet
    await waitFor(() => expect(asked).toEqual(fixtureAccounts.map((a) => a.id)))
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    expect(trackEvent).toHaveBeenCalledTimes(1)
    expect(trackEvent).toHaveBeenCalledWith(METRICS.DATA_FULL_RELOAD, { source: 'settings' })
  })

  it('cannot be pressed again while its reload runs', async () => {
    const user = userEvent.setup()
    let release: () => void = () => {}
    const gate = new Promise<void>((r) => { release = r })
    const queryClient = renderPage()
    await waitFor(() => expect(queryClient.getQueryState(queryKeys.transactions)?.status).toBe('success'))
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    server.use(http.get('*/api/v1/transaction/get-account-transaction-list', async () => {
      await gate
      return undefined
    }))
    const row = screen.getByRole('button', { name: 'Clear cache and reload' })
    await user.click(row)
    await waitFor(() => expect(row).toBeDisabled())
    release()
    await waitFor(() => expect(row).toBeEnabled())
    expect(trackEvent).toHaveBeenCalledTimes(1)
  })

  it('stays usable while an ordinary refresh hangs', async () => {
    const queryClient = renderPage()
    await waitFor(() => expect(queryClient.getQueryState(queryKeys.transactions)?.status).toBe('success'))
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    server.use(http.get('*/api/v1/account/get-account-list', () => new Promise<Response>(() => {})))
    void queryClient.invalidateQueries()
    await waitFor(() => expect(queryClient.isFetching({ queryKey: queryKeys.transactions })).toBe(1))
    expect(screen.getByRole('button', { name: 'Clear cache and reload' })).toBeEnabled()
  })

  it('is enabled again once the reload fails, while it keeps retrying', async () => {
    const user = userEvent.setup()
    const queryClient = renderPage(true)
    await waitFor(() => expect(queryClient.getQueryState(queryKeys.transactions)?.status).toBe('success'))
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    server.use(http.get('*/api/v1/transaction/get-account-transaction-list', () =>
      HttpResponse.json({ success: false, message: 'boom', code: 0, exceptionType: 'x' }, { status: 500 })))
    const row = screen.getByRole('button', { name: 'Clear cache and reload' })
    await user.click(row)
    await waitFor(() => expect(queryClient.getQueryState(queryKeys.transactions)?.fetchFailureCount).toBe(1))
    await waitFor(() => expect(row).toBeEnabled())
    expect(queryClient.isFetching({ queryKey: queryKeys.transactions })).toBe(1)
  })

  it('keeps every stored transaction when the server cannot be reached', async () => {
    const user = userEvent.setup()
    const queryClient = renderPage()
    await waitFor(() => expect(queryClient.getQueryState(queryKeys.transactions)?.status).toBe('success'))
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    await waitFor(async () => expect((await loadBuckets('u1')).size).toBe(fixtureAccounts.length))
    const before = await loadBuckets('u1')
    // online as far as the browser knows, but every download fails
    server.use(http.get('*/api/v1/transaction/get-account-transaction-list', () => HttpResponse.error()))
    await user.click(screen.getByRole('button', { name: 'Clear cache and reload' }))
    await waitFor(() => expect(queryClient.getQueryState(queryKeys.transactions)?.status).toBe('error'))
    const after = await loadBuckets('u1')
    expect([...after.keys()].sort()).toEqual([...before.keys()].sort())
    for (const [id, bucket] of before) {
      expect(after.get(id)!.items).toEqual(bucket.items)
    }
  })

  it('a second press during the reload counts nothing and reloads nothing more', async () => {
    let release: () => void = () => {}
    const gate = new Promise<void>((r) => { release = r })
    const queryClient = renderPage()
    await waitFor(() => expect(queryClient.getQueryState(queryKeys.transactions)?.status).toBe('success'))
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    server.use(http.get('*/api/v1/transaction/get-account-transaction-list', async () => {
      await gate
      return undefined
    }))
    const row = screen.getByRole('button', { name: 'Clear cache and reload' })
    // two presses before the row can re-render as disabled
    row.click()
    row.click()
    release()
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    expect(trackEvent).toHaveBeenCalledTimes(1)
    expect(screen.getAllByText('Clearing the cache and reloading…')).toHaveLength(1)
  })

  it('offline, keeps the stored data and says to try again online', async () => {
    const user = userEvent.setup()
    const queryClient = renderPage()
    await waitFor(() => expect(queryClient.getQueryState(queryKeys.transactions)?.status).toBe('success'))
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    await waitFor(async () => expect((await loadBuckets('u1')).size).toBe(fixtureAccounts.length))
    const asked: string[] = []
    const record = ({ request }: { request: Request }) => {
      if (request.url.includes('/get-account-transaction-list')) asked.push(request.url)
    }
    server.events.on('request:start', record)
    onTestFinished(() => server.events.removeListener('request:start', record))
    const offline = vi.spyOn(navigator, 'onLine', 'get').mockReturnValue(false)
    onTestFinished(() => offline.mockRestore())

    await user.click(screen.getByRole('button', { name: 'Clear cache and reload' }))
    expect(await screen.findByText("You're offline. Clear the cache and reload when you're back online.")).toBeInTheDocument()
    expect(screen.queryByText('Clearing the cache and reloading…')).not.toBeInTheDocument()
    expect(trackEvent).not.toHaveBeenCalled()
    expect((await loadBuckets('u1')).size).toBe(fixtureAccounts.length)
    expect(asked).toEqual([])
    expect(screen.getByRole('button', { name: 'Clear cache and reload' })).toBeEnabled()
  })

  it('unlocks when the connection drops during the reload and the download pauses', async () => {
    const user = userEvent.setup()
    const queryClient = renderPage()
    await waitFor(() => expect(queryClient.getQueryState(queryKeys.transactions)?.status).toBe('success'))
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    onlineManager.setOnline(false)
    onTestFinished(() => onlineManager.setOnline(true))
    const row = screen.getByRole('button', { name: 'Clear cache and reload' })
    await user.click(row)
    await waitFor(() => expect(queryClient.getQueryState(queryKeys.transactions)?.fetchStatus).toBe('paused'))
    await waitFor(() => expect(row).toBeEnabled())
  })
})
