import type { ReactElement } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, Outlet, RouterProvider } from 'react-router'
import { delay, http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { InboxButton } from './InboxButton'
import { InboxPage } from './InboxPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))

const queued = (over: Record<string, unknown> = {}) => ({
  linkId: 'l1', sourceId: 's1', externalAccountId: 'Apple Card', accountId: '', payee: 'Blue Bottle', amount: '12.5',
  currency: 'USD', type: 'expense', postedAt: '2026-08-20 10:42:03', reason: 'unmapped', ...over,
})
const failedEvent = { eventId: 'e9', sourceId: 's1', receivedAt: '2026-08-21 08:00:00', error: 'amount: This value should not be blank.', payload: '{"account":"Apple Card","amount":""}' }
const syncProblemSource = (over: Record<string, unknown> = {}) => ({
  id: 's2', provider: 'simplefin', name: 'Bank', status: 'active', createdAt: '2026-08-01 00:00:00',
  lastSyncedAt: '', lastRunStatus: 'failed', lastRunAt: '2026-09-29 09:14:00', lastRunError: 'token expired',
  lastRunErrorAccountId: '', credentialCiphertext: '', cards: [], ...over,
})

function renderInbox(extraRoutes: { path: string; element: ReactElement }[] = []) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [{ path: '/inbox', element: <InboxPage /> }, ...extraRoutes],
    { initialEntries: ['/inbox'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

const pendingOwner = { id: 'u2', avatar: 'pets:sky', name: 'Partner' }
const pendingBudget = {
  id: 'b-pending', ownerUserId: 'u2', name: 'Shared budget', startedAt: '2026-01-01 00:00:00', currencyId: 'cur-usd',
  access: [
    { user: pendingOwner, role: 'owner', isAccepted: 1 },
    { user: { id: 'u1', avatar: 'face:emerald', name: 'Ada' }, role: 'admin', isAccepted: 0 },
  ],
}
const acceptedBudget = {
  ...pendingBudget,
  access: [pendingBudget.access[0], { ...pendingBudget.access[1], isAccepted: 1 }],
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = { TRANSACTION_IMPORT: true }
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('renders the sections in a fixed order: Sync problems, Failed imports, To review, Skipped', async () => {
  server.use(...coreHandlers({
    importSources: [syncProblemSource()],
    importQueue: { queued: [queued()], skipped: [queued({ linkId: 'l2' })], failed: [failedEvent] },
  }))
  renderInbox()
  await screen.findByText('Bank sync failed')
  const headings = screen.getAllByRole('heading', { level: 2 }).map((h) => h.textContent)
  expect(headings).toEqual(['Sync problems', 'Failed imports', 'To review', 'Skipped (1)'])
})

it('does not render empty sections', async () => {
  server.use(...coreHandlers({ importQueue: { queued: [queued()], skipped: [], failed: [] } }))
  renderInbox()
  expect(await screen.findByText('Blue Bottle')).toBeInTheDocument()
  expect(screen.queryByText('Failed imports')).toBeNull()
  expect(screen.queryByText('Sync problems')).toBeNull()
})

it('shows All caught up only once the queue has resolved, and not before', async () => {
  server.use(...coreHandlers())
  server.use(http.get('*/api/v1/import/get-queued-event-list', async () => {
    await delay(50)
    return HttpResponse.json({ success: true, message: '', data: { queued: [], skipped: [], failed: [] } })
  }))
  renderInbox()
  expect(screen.queryByText('All caught up')).toBeNull()
  expect(await screen.findByText('All caught up')).toBeInTheDocument()
})

it('keeps the skipped list collapsed until its header is toggled', async () => {
  server.use(...coreHandlers({ importQueue: { queued: [], skipped: [queued()], failed: [] } }))
  renderInbox()
  const toggle = await screen.findByRole('button', { name: 'Skipped (1)' })
  expect(screen.queryByRole('button', { name: 'Restore Blue Bottle' })).toBeNull()
  const user = userEvent.setup()
  await user.click(toggle)
  expect(await screen.findByRole('button', { name: 'Restore Blue Bottle' })).toBeInTheDocument()
})

it('does not show All caught up next to the error when a background refetch fails on an empty cached queue', async () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  server.use(...coreHandlers({ importQueue: { queued: [], skipped: [], failed: [] } }))
  const router = createMemoryRouter([{ path: '/inbox', element: <InboxPage /> }], { initialEntries: ['/inbox'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  expect(await screen.findByText('All caught up')).toBeInTheDocument()

  server.use(http.get('*/api/v1/import/get-queued-event-list', () =>
    HttpResponse.json({ success: false, message: 'Something went wrong. Please try again.', code: 500, errors: {} }, { status: 500 })))
  await queryClient.refetchQueries({ queryKey: ['importQueue'] })

  await screen.findByText('Something went wrong. Please try again.')
  expect(screen.queryByText('All caught up')).toBeNull()
})

it('shows the generic error with a working retry when the queue fails to load', async () => {
  let calls = 0
  server.use(...coreHandlers())
  server.use(http.get('*/api/v1/import/get-queued-event-list', () => {
    calls += 1
    return HttpResponse.json({ success: false, message: 'Something went wrong. Please try again.', code: 500, errors: {} }, { status: 500 })
  }))
  renderInbox()
  expect(await screen.findByText('Something went wrong. Please try again.')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  expect(screen.queryByText('All caught up')).toBeNull()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  await waitFor(() => expect(calls).toBeGreaterThanOrEqual(2))
})

it('shows the error and Retry (never blank, never All caught up) when the source list fails with an empty queue', async () => {
  let sourceCalls = 0
  let queueCalls = 0
  server.use(...coreHandlers({ importQueue: { queued: [], skipped: [], failed: [] } }))
  server.use(
    http.get('*/api/v1/import/get-source-list', () => {
      sourceCalls += 1
      return HttpResponse.json({ success: false, message: 'Something went wrong. Please try again.', code: 500, errors: {} }, { status: 500 })
    }),
    http.get('*/api/v1/import/get-queued-event-list', () => {
      queueCalls += 1
      return HttpResponse.json({ success: true, message: '', data: { queued: [], skipped: [], failed: [] } })
    }),
  )
  renderInbox()
  expect(await screen.findByText('Something went wrong. Please try again.')).toBeInTheDocument()
  expect(screen.queryByText('All caught up')).toBeNull()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Retry' }))
  await waitFor(() => expect(sourceCalls).toBeGreaterThanOrEqual(2))
  await waitFor(() => expect(queueCalls).toBeGreaterThanOrEqual(2))
})

it('keeps a cached queued row visible alongside the error notice when a background refetch fails', async () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  server.use(...coreHandlers({ importQueue: { queued: [queued()], skipped: [], failed: [] } }))
  const router = createMemoryRouter([{ path: '/inbox', element: <InboxPage /> }], { initialEntries: ['/inbox'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  expect(await screen.findByText('Blue Bottle')).toBeInTheDocument()

  server.use(http.get('*/api/v1/import/get-queued-event-list', () =>
    HttpResponse.json({ success: false, message: 'Something went wrong. Please try again.', code: 500, errors: {} }, { status: 500 })))
  await queryClient.refetchQueries({ queryKey: ['importQueue'] })

  await screen.findByText('Something went wrong. Please try again.')
  expect(screen.getByText('Blue Bottle')).toBeInTheDocument()
})

it('a failed sync problem links to SimpleFIN settings and shows the run error', async () => {
  server.use(...coreHandlers({ importSources: [syncProblemSource()] }))
  renderInbox()
  expect(await screen.findByText('Bank sync failed')).toBeInTheDocument()
  expect(screen.getByText('token expired')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: /Bank sync failed/ })).toHaveAttribute('href', '/settings/simplefin')
})

it('a per-account run error names the card by its external name', async () => {
  server.use(...coreHandlers({
    importSources: [syncProblemSource({
      lastRunError: 'Import failed for this account',
      lastRunErrorAccountId: 'acct-9',
      cards: [{ externalAccountId: 'acct-9', externalName: 'Chase Checking', externalCurrency: '', state: 'mapped', accountId: 'a1', queuedCount: 0, tapCount: 1, lastSeenAt: '' }],
    })],
  }))
  renderInbox()
  expect(await screen.findByText('Chase Checking: Import failed for this account')).toBeInTheDocument()
})

it('a per-account run error falls back to the raw external id when the card is unknown', async () => {
  server.use(...coreHandlers({
    importSources: [syncProblemSource({ lastRunError: 'Import failed for this account', lastRunErrorAccountId: 'acct-9', cards: [] })],
  }))
  renderInbox()
  expect(await screen.findByText('acct-9: Import failed for this account')).toBeInTheDocument()
})

it('a partial sync problem is worded differently', async () => {
  server.use(...coreHandlers({ importSources: [syncProblemSource({ lastRunStatus: 'partial' })] }))
  renderInbox()
  expect(await screen.findByText('Bank synced with errors')).toBeInTheDocument()
})

it('puts Sharing before To review, and accepting a budget invite drops it, navigates to the budget page, and lowers the Inbox count', async () => {
  let budgetListCalls = 0
  server.use(...coreHandlers({ importQueue: { queued: [queued()], skipped: [], failed: [] } }))
  server.use(
    http.get('*/api/v1/budget/get-budget-list', () => {
      budgetListCalls += 1
      return HttpResponse.json({ success: true, message: '', data: { items: [budgetListCalls === 1 ? pendingBudget : acceptedBudget] } })
    }),
    http.post('*/api/v1/budget/accept-access', async () =>
      HttpResponse.json({ success: true, message: '', data: { items: [acceptedBudget] } }),
    ),
    http.post('*/api/v1/user/update-budget', async () =>
      HttpResponse.json({ success: true, message: '', data: { user: { id: 'u1', name: 'Ada', avatar: 'face:emerald', options: [] } } }),
    ),
  )
  // InboxButton is mounted in a layout wrapping both routes so it survives
  // the post-accept navigation away from InboxPage, letting the badge count
  // be observed both before and after the mutation clears the invite.
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      {
        element: (
          <div>
            <InboxButton variant="row" />
            <Outlet />
          </div>
        ),
        children: [
          { path: '/inbox', element: <InboxPage /> },
          { path: '/budget', element: <div>BUDGET PAGE</div> },
        ],
      },
    ],
    { initialEntries: ['/inbox'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )

  await screen.findByText('Blue Bottle')
  await screen.findByText('Partner invited you')
  const headings = screen.getAllByRole('heading', { level: 2 }).map((h) => h.textContent)
  expect(headings.indexOf('Sharing')).toBeGreaterThanOrEqual(0)
  expect(headings.indexOf('Sharing')).toBeLessThan(headings.indexOf('To review'))
  await waitFor(() => expect(screen.getByTestId('inbox-badge')).toHaveTextContent('2'))

  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Accept' }))
  expect(await screen.findByText('BUDGET PAGE')).toBeInTheDocument()

  expect(screen.queryByText('Partner invited you')).toBeNull()
  await waitFor(() => expect(screen.getByTestId('inbox-badge')).toHaveTextContent('1'))
})

it('shows only the Sharing section when there is a pending invite and no imports', async () => {
  server.use(...coreHandlers({ budgets: [pendingBudget] }))
  renderInbox()
  expect(await screen.findByText('Partner invited you')).toBeInTheDocument()
  expect(screen.queryByText('To review')).toBeNull()
  expect(screen.queryByText('Failed imports')).toBeNull()
  expect(screen.queryByText('Sync problems')).toBeNull()
  expect(screen.queryByText('All caught up')).toBeNull()
})
