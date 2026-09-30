import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { delay, http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
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
  credentialCiphertext: '', cards: [], ...over,
})

function renderInbox() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/inbox', element: <InboxPage /> }], { initialEntries: ['/inbox'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
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

it('a failed sync problem links to SimpleFIN settings and shows the run error', async () => {
  server.use(...coreHandlers({ importSources: [syncProblemSource()] }))
  renderInbox()
  expect(await screen.findByText('Bank sync failed')).toBeInTheDocument()
  expect(screen.getByText('token expired')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: /Bank sync failed/ })).toHaveAttribute('href', '/settings/simplefin')
})

it('a partial sync problem is worded differently', async () => {
  server.use(...coreHandlers({ importSources: [syncProblemSource({ lastRunStatus: 'partial' })] }))
  renderInbox()
  expect(await screen.findByText('Bank synced with errors')).toBeInTheDocument()
})
