import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { toast } from 'sonner'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import { ImportQueuePage } from './ImportQueuePage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const queued = (over: Record<string, unknown> = {}) => ({
  linkId: 'l1', sourceId: 's1', externalAccountId: 'Apple Card', accountId: '', payee: 'Blue Bottle', amount: '12.5',
  currency: 'USD', type: 'expense', postedAt: '2026-08-20 10:42:03', reason: 'unmapped', ...over,
})
const failed = { eventId: 'e9', sourceId: 's1', receivedAt: '2026-08-21 08:00:00', error: 'amount: This value should not be blank.', payload: '{"account":"Apple Card","amount":""}' }
const source = { id: 's1', provider: 'apple-wallet', name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00', cards: [
  { externalAccountId: 'Apple Card', externalName: 'Apple Card', externalCurrency: 'USD', state: 'unmapped', accountId: '', queuedCount: 1, tapCount: 1, lastSeenAt: '2026-08-20 10:42:03' },
] }

function renderPage(importQueue: unknown) {
  server.use(...coreHandlers({ importSources: [source], importQueue }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [{ path: '/imports/queue', element: <ImportQueuePage /> }, { path: '/settings/apple-wallet', element: <div>WALLET PAGE</div> }],
    { initialEntries: ['/imports/queue'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  useUiStore.setState({ transactionModal: null })
  vi.mocked(toast.error).mockClear()
  vi.mocked(toast.success).mockClear()
})

it('shows the empty state when nothing is queued', async () => {
  renderPage({ queued: [], skipped: [], failed: [] })
  expect(await screen.findByText('Nothing to review.')).toBeInTheDocument()
})

it('groups queued rows by card, formats amounts with moneyFormat, and opens the prefilled transaction dialog on tap', async () => {
  renderPage({ queued: [queued(), queued({ linkId: 'l2', payee: 'Whole Foods', amount: '1234.5' })], skipped: [], failed: [] })
  const user = userEvent.setup()
  expect(await screen.findByText('Apple Card')).toBeInTheDocument()
  expect(screen.getAllByText(/Card not mapped/)).toHaveLength(2)
  // thousand separator only shows up once amounts route through moneyFormat
  expect(screen.getByText('-1,234.5 USD')).toBeInTheDocument()
  await user.click(screen.getByText('Blue Bottle'))
  const params = useUiStore.getState().transactionModal
  expect(params?.importQueued).toEqual({ linkId: 'l1', type: 'expense', accountId: '', amount: '12.5', currency: 'USD', payee: 'Blue Bottle', date: '2026-08-20 10:42:03' })
})

it('an unmapped card header links to the mapping page', async () => {
  renderPage({ queued: [queued()], skipped: [], failed: [] })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('link', { name: 'Map to account' }))
  expect(await screen.findByText('WALLET PAGE')).toBeInTheDocument()
})

it('skip posts skip-queued-event; a skipped row offers Restore, labelled with the card', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/import/skip-queued-event', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { queued: [], skipped: [queued()], failed: [] } })
  }))
  renderPage({ queued: [queued()], skipped: [], failed: [] })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Skip Blue Bottle' }))
  await waitFor(() => expect(body).toEqual({ linkId: 'l1' }))
  expect(await screen.findByRole('button', { name: 'Restore Blue Bottle' })).toBeInTheDocument()
  // the skipped section has no group header, so the card name rides in the row itself
  expect(screen.getByText(/Apple Card ·/)).toBeInTheDocument()
})

it('a failed skip toasts the server error', async () => {
  server.use(http.post('*/api/v1/import/skip-queued-event', () =>
    HttpResponse.json({ success: false, message: 'Row already handled.', code: 400, errors: {} }, { status: 400 })))
  renderPage({ queued: [queued()], skipped: [], failed: [] })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Skip Blue Bottle' }))
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Row already handled.'))
})

it('needs-attention rows show the error and payload; retry toasts the outcome; discard removes', async () => {
  let retried: unknown
  let discarded: unknown
  server.use(
    http.post('*/api/v1/import/retry-event', async ({ request }) => {
      retried = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { status: 'failed', eventId: 'e9' } })
    }),
    http.post('*/api/v1/import/discard-event', async ({ request }) => {
      discarded = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { queued: [], skipped: [], failed: [] } })
    }),
  )
  renderPage({ queued: [], skipped: [], failed: [failed] })
  const user = userEvent.setup()
  const section = (await screen.findByText('Needs attention')).parentElement as HTMLElement
  expect(within(section).getByText('amount: This value should not be blank.')).toBeInTheDocument()
  expect(within(section).getByText(/"account":"Apple Card"/)).toBeInTheDocument()
  await user.click(within(section).getByRole('button', { name: /^Retry/ }))
  await waitFor(() => expect(retried).toEqual({ eventId: 'e9' }))
  await user.click(within(section).getByRole('button', { name: /^Discard/ }))
  await waitFor(() => expect(discarded).toEqual({ eventId: 'e9' }))
  expect(await screen.findByText('Nothing to review.')).toBeInTheDocument()
})

it('shows an error state with a retry button when the queue fails to load', async () => {
  renderPage({ queued: [], skipped: [], failed: [] })
  // registered after renderPage's own coreHandlers so it wins the override;
  // both calls are synchronous, so this still lands before the request fires
  server.use(http.get('*/api/v1/import/get-queued-event-list', () =>
    HttpResponse.json({ success: false, message: 'Something went wrong. Please try again.', code: 500, errors: {} }, { status: 500 })))
  expect(await screen.findByText('Something went wrong. Please try again.')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  expect(screen.queryByText('Nothing to review.')).toBeNull()
})
