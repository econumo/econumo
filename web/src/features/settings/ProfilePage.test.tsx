import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider, onlineManager } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts, fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { Toaster } from '@/components/ui/sonner'
import { useTransactions } from '@/features/transactions/queries'
import { resetRevisions } from '@/features/transactions/revisionTracking'
import { METRICS, trackEvent } from '@/lib/metrics'
import { loadBuckets } from '@/lib/transactionStore'
import { recordPathname, resetNavTracking } from '@/lib/navigation'
import i18n from '@/app/i18n'
import { ProfilePage } from './ProfilePage'

vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

// the application shell keeps the transactions list mounted on every page
function TransactionsMounted() {
  useTransactions()
  return null
}

function mockViewport(compact = false) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023') ? compact : false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderWithHistory(initialEntries: string[], initialIndex: number) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/', element: <div>HOME ROUTE</div> },
      { path: '/account/:id', element: <div>ACCOUNT ROUTE</div> },
      { path: '/settings', element: <div>SETTINGS HUB ROUTE</div> },
      { path: '/settings/profile', element: <ProfilePage /> },
    ],
    { initialEntries, initialIndex },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/settings/profile', element: <ProfilePage /> },
      { path: '/logout', element: <div>LOGOUT ROUTE</div> },
      { path: '/settings/profile/change-password', element: <div>CHANGE PASSWORD ROUTE</div> },
    ],
    { initialEntries: ['/settings/profile'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

// retryDelay: a failed reload must stay failed for as long as a test looks at it
function renderReloadPage(retry: boolean | number = false) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry, retryDelay: 60_000 }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [{ path: '/settings/profile', element: <><ProfilePage /><TransactionsMounted /><Toaster /></> }],
    { initialEntries: ['/settings/profile'] },
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
  mockViewport()
  resetNavTracking()
})

it('titles the page "Personal settings"', async () => {
  renderPage()
  expect(await screen.findByText('Personal settings')).toBeInTheDocument()
  expect(screen.queryByText('Profile')).not.toBeInTheDocument()
})

it('mobile back returns to the settings hub when the user came from it', async () => {
  mockViewport(true)
  const user = userEvent.setup()
  recordPathname('/settings')
  recordPathname('/settings/profile')
  renderWithHistory(['/settings', '/settings/profile'], 1)
  await user.click(await screen.findByRole('button', { name: 'back' }))
  expect(await screen.findByText('SETTINGS HUB ROUTE')).toBeInTheDocument()
})

it('mobile back falls back to the main screen when the origin is any other page', async () => {
  mockViewport(true)
  const user = userEvent.setup()
  recordPathname('/account/a1')
  recordPathname('/settings/profile')
  renderWithHistory(['/account/a1', '/settings/profile'], 1)
  await user.click(await screen.findByRole('button', { name: 'back' }))
  expect(await screen.findByText('HOME ROUTE')).toBeInTheDocument()
})

it('mobile back falls back to the main screen on a deep link', async () => {
  mockViewport(true)
  const user = userEvent.setup()
  recordPathname('/settings/profile')
  renderWithHistory(['/settings/profile'], 0)
  await user.click(await screen.findByRole('button', { name: 'back' }))
  expect(await screen.findByText('HOME ROUTE')).toBeInTheDocument()
})

it('saves the name on blur and updates the cache', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/update-name', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { user: { ...fixtureUser, name: 'Grace' } } })
    }),
  )
  const user = userEvent.setup()
  const queryClient = renderPage()
  const nameInput = await screen.findByLabelText('Name')
  await waitFor(() => expect(nameInput).toHaveValue('Ada'))
  await user.clear(nameInput)
  await user.type(nameInput, 'Grace')
  await user.tab()
  await waitFor(() => expect(body).toEqual({ name: 'Grace' }))
  await waitFor(() => expect(queryClient.getQueryData<{ name: string }>(queryKeys.user)!.name).toBe('Grace'))
})

it('shows a transient checkmark after the name is saved, hidden again on edit', async () => {
  server.use(
    http.post('*/api/v1/user/update-name', () =>
      HttpResponse.json({ success: true, message: '', data: { user: { ...fixtureUser, name: 'Grace' } } }),
    ),
  )
  const user = userEvent.setup()
  renderPage()
  const nameInput = await screen.findByLabelText('Name')
  await waitFor(() => expect(nameInput).toHaveValue('Ada'))
  expect(screen.getByTestId('name-saved')).toHaveClass('opacity-0')
  await user.clear(nameInput)
  await user.type(nameInput, 'Grace')
  await user.tab()
  await waitFor(() => expect(screen.getByTestId('name-saved')).toHaveClass('opacity-100'))
  await user.type(nameInput, '!')
  expect(screen.getByTestId('name-saved')).toHaveClass('opacity-0')
})

it('surfaces server field errors under the name input', async () => {
  server.use(
    http.post('*/api/v1/user/update-name', () =>
      HttpResponse.json(
        {
          success: false,
          message: 'Form validation error',
          code: 400,
          errors: { name: ['This value is too long.'] },
        },
        { status: 400 },
      ),
    ),
  )
  const user = userEvent.setup()
  renderPage()
  const nameInput = await screen.findByLabelText('Name')
  await waitFor(() => expect(nameInput).toHaveValue('Ada'))
  await user.clear(nameInput)
  await user.type(nameInput, 'A very long name over twenty chars')
  await user.tab()
  expect(await screen.findByText('This value is too long.')).toBeInTheDocument()
})

it('changing the default currency posts the code and updates the cache', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/update-currency', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({
        success: true, message: '',
        data: { user: { ...fixtureUser, options: fixtureUser.options.map((o) => (o.name === 'currency_id' ? { ...o, value: 'cur-eur' } : o)) } },
      })
    }),
  )
  const user = userEvent.setup()
  const queryClient = renderPage()
  await screen.findByText('Currency')
  // the row shows the current code; clicking it opens the search dialog
  await waitFor(() => expect(screen.getByText('USD')).toBeInTheDocument())
  await user.click(screen.getByText('Currency'))
  await user.click(await screen.findByText('EUR, €, Euro'))
  await waitFor(() => expect(body).toEqual({ currency: 'EUR' }))
  await waitFor(() => {
    const cached = queryClient.getQueryData<typeof fixtureUser>(queryKeys.user)!
    expect(cached.options.find((o) => o.name === 'currency_id')!.value).toBe('cur-eur')
  })
})

it('Language row opens a dialog to switch locale and closes on pick', async () => {
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByText('Language'))
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText('English')).toBeInTheDocument()
  await user.click(within(dialog).getByText('Русский'))
  await screen.findByText('Язык')
  expect(document.documentElement.lang).toBe('ru')
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  // restore for later tests in this file
  await i18n.changeLanguage('en')
  document.documentElement.lang = 'en'
})

it('clicking the avatar opens the avatar picker dialog', async () => {
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Change avatar' }))
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText('Choose your avatar')).toBeInTheDocument()
})

it('toggles the analytics preference', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/update-analytics', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({
        success: true, message: '',
        data: { user: { ...fixtureUser, options: [...fixtureUser.options, { name: 'analytics', value: '0' }] } },
      })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  const toggle = await screen.findByRole('switch', { name: /share usage statistics/i })
  expect(toggle).toBeChecked()

  await user.click(toggle)

  await waitFor(() => expect(body).toEqual({ enabled: false }))
})

it('logout confirm has the exact copy and navigates', async () => {
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByText('Log out'))
  expect(await screen.findByText('Are you sure you want to log out?')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Stay' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Log out' }))
  expect(await screen.findByText('LOGOUT ROUTE')).toBeInTheDocument()
})

it('shows a Sign-in methods row in the Security group', async () => {
  renderPage()
  expect(await screen.findByText('Sign-in methods')).toBeInTheDocument()
  expect(screen.getByText('Change password')).toBeInTheDocument()
})

it('shows the Connected apps row only when the server serves an MCP address', async () => {
  window.econumoConfig = { MCP_URL: 'https://money.example.test/mcp' }
  renderPage()
  expect(await screen.findByText('Connected apps')).toBeInTheDocument()
})

it('hides the Connected apps row when the server serves no MCP address', async () => {
  window.econumoConfig = { MCP_URL: '' }
  renderPage()
  expect(await screen.findByText('Sign-in methods')).toBeInTheDocument()
  expect(screen.queryByText('Connected apps')).not.toBeInTheDocument()
})

it('replaces "Change password" with "Set a password" when the user has no password', async () => {
  server.use(...coreHandlers({ user: { ...fixtureUser, hasPassword: false } }))
  renderPage()
  expect(await screen.findByText('Set a password')).toBeInTheDocument()
  expect(screen.getByText('Sign-in methods')).toBeInTheDocument()
  expect(screen.queryByText('Change password')).not.toBeInTheDocument()
})

it('offers neither password entry when password sign-in is disabled', async () => {
  window.econumoConfig = { PASSWORD_LOGIN: false }
  server.use(...coreHandlers({ user: { ...fixtureUser, hasPassword: false } }))
  renderPage()
  expect(await screen.findByText('Sign-in methods')).toBeInTheDocument()
  expect(screen.queryByText('Change password')).not.toBeInTheDocument()
  expect(screen.queryByText('Set a password')).not.toBeInTheDocument()
})

it('"Set a password" opens the recovery dialog with the email locked', async () => {
  server.use(...coreHandlers({ user: { ...fixtureUser, hasPassword: false } }))
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByText('Set a password'))
  const dialog = await screen.findByRole('dialog')
  const emailField = within(dialog).getByLabelText(/e-?mail/i)
  expect(emailField).toHaveValue(fixtureUser.email)
  expect(emailField).toBeDisabled()
})

it('signs out after a passwordless user sets a password (the reset revoked this session)', async () => {
  server.use(
    ...coreHandlers({ user: { ...fixtureUser, hasPassword: false } }),
    http.post('*/api/v1/user/remind-password', () => HttpResponse.json({ success: true, message: '', data: {} })),
    http.post('*/api/v1/user/reset-password', () => HttpResponse.json({ success: true, message: '', data: {} })),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByText('Set a password'))
  await user.click(screen.getByRole('button', { name: 'Send code' }))
  await user.type(await screen.findByLabelText(/code/i), '123456')
  await user.type(screen.getByLabelText(/new password/i), 'Password123!')
  await user.click(screen.getByRole('button', { name: 'Reset password' }))
  expect(await screen.findByText('LOGOUT ROUTE')).toBeInTheDocument()
})

describe('Clear cache and reload', () => {
  it('is the only row of the This device group, at the bottom of the page', async () => {
    renderReloadPage()
    const row = await screen.findByRole('button', { name: 'Clear cache and reload' })
    const label = screen.getByText('This device')
    expect(label.nextElementSibling).toContainElement(row)
    expect(label.nextElementSibling!.children).toHaveLength(1)
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
    const queryClient = renderReloadPage()
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
    const queryClient = renderReloadPage()
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
    const queryClient = renderReloadPage()
    await waitFor(() => expect(queryClient.getQueryState(queryKeys.transactions)?.status).toBe('success'))
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    server.use(http.get('*/api/v1/account/get-account-list', () => new Promise<Response>(() => {})))
    void queryClient.invalidateQueries()
    await waitFor(() => expect(queryClient.isFetching({ queryKey: queryKeys.transactions })).toBe(1))
    expect(screen.getByRole('button', { name: 'Clear cache and reload' })).toBeEnabled()
  })

  it('is enabled again once the reload fails, while it keeps retrying', async () => {
    const user = userEvent.setup()
    const queryClient = renderReloadPage(true)
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

  it('a second press during the reload counts nothing and reloads nothing more', async () => {
    let release: () => void = () => {}
    const gate = new Promise<void>((r) => { release = r })
    const queryClient = renderReloadPage()
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
    const queryClient = renderReloadPage()
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
    const queryClient = renderReloadPage()
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
