import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { AppleWalletInboxPage } from './AppleWalletInboxPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))

const card = { externalAccountId: 'Apple Card', externalName: 'Apple Card', externalCurrency: 'USD', state: 'unmapped', accountId: '', queuedCount: 1, tapCount: 1, lastSeenAt: '2026-08-20 17:42:03' }
const wallet = { id: 's1', provider: 'apple-wallet', name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00', cards: [card] }
const bank = { id: 's2', provider: 'simplefin', name: 'Bank', status: 'active', createdAt: '2026-08-01 00:00:00', cards: [] }
const row = (over: Record<string, unknown>) => ({
  linkId: 'l1', sourceId: 's1', externalAccountId: 'Apple Card', accountId: '', payee: 'Blue Bottle', amount: '5',
  currency: 'USD', type: 'expense', postedAt: '2026-08-20 10:42:03', reason: 'unmapped', ...over,
})
const failed = (over: Record<string, unknown>) => ({ eventId: 'e1', sourceId: 's1', receivedAt: '2026-08-21 08:00:00', error: 'wallet failure', payload: '{}', ...over })

function renderPage(importQueue: unknown) {
  server.use(...coreHandlers({ importSources: [wallet, bank], importQueue }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/apple-wallet/inbox', element: <AppleWalletInboxPage /> }], { initialEntries: ['/settings/apple-wallet/inbox'] })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = { IMPORT_APPLE_WALLET: true, IMPORT_SIMPLEFIN: true }
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('lists only Apple Wallet rows: to review, failed and the folded skipped section', async () => {
  renderPage({
    queued: [row({}), row({ linkId: 'l2', sourceId: 's2', externalAccountId: 'Checking', payee: 'Bank payee' })],
    skipped: [row({ linkId: 'l3', payee: 'Skipped tap' })],
    failed: [failed({}), failed({ eventId: 'e2', sourceId: 's2', error: 'bank failure' })],
  })
  // the first render in the file pays the cold module load, so it gets a longer wait
  const toReview = (await screen.findByRole('heading', { name: 'To review' }, { timeout: 5000 })).closest('section') as HTMLElement
  expect(within(toReview).getByText('Blue Bottle')).toBeInTheDocument()
  expect(screen.queryByText('Bank payee')).toBeNull()
  expect(screen.getByText('wallet failure')).toBeInTheDocument()
  expect(screen.queryByText('bank failure')).toBeNull()
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Skipped (1)' }))
  expect(screen.getByText('Skipped tap')).toBeInTheDocument()
})

it('an unmapped card maps in place from its group header', async () => {
  renderPage({ queued: [row({})], skipped: [], failed: [] })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: 'Map to account' }))
  expect(await screen.findByLabelText('Account')).toBeInTheDocument()
})

it('reads "All caught up" when only other sources have rows waiting', async () => {
  renderPage({ queued: [row({ sourceId: 's2' })], skipped: [], failed: [] })
  expect(await screen.findByText('All caught up')).toBeInTheDocument()
})
