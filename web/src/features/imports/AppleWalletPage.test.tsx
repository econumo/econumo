import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { AppleWalletPage } from './AppleWalletPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))

const wireSource = {
  id: 's1', provider: 'apple-wallet', name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00',
  cards: [{ externalAccountId: 'wallet', externalName: 'Apple Card', externalCurrency: 'USD', state: 'unmapped', accountId: '', queuedCount: 2, tapCount: 3, lastSeenAt: '2026-08-20 17:42:03' }],
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/apple-wallet', element: <AppleWalletPage /> }], { initialEntries: ['/settings/apple-wallet'] })
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

it('offers Apple Wallet setup when no source exists', async () => {
  server.use(...coreHandlers())
  renderPage()
  expect(await screen.findByRole('heading', { name: 'Apple Wallet' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Set up Apple Wallet' })).toBeInTheDocument()
  expect(screen.queryByText('Cards')).not.toBeInTheDocument()
  expect(screen.queryByRole('link', { name: /^Inbox/ })).toBeNull()
  expect(screen.queryByRole('button', { name: 'Disconnect Apple Wallet' })).toBeNull()
})

it('shows the connected state and the card list when a source exists', async () => {
  server.use(...coreHandlers({ importSources: [wireSource] }))
  renderPage()
  expect(await screen.findByText('Connected')).toBeInTheDocument()
  expect(screen.getByText(/^Last tap /)).toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'Needs mapping' })).toBeInTheDocument()
  expect(screen.getByText('Apple Card')).toBeInTheDocument()
  expect(screen.getByText(/^2 to review/)).toBeInTheDocument()
  // a card has arrived, so the setup guide is folded and Disconnect sits at the bottom
  expect(screen.getByRole('button', { name: /Setup guide/ })).toHaveAttribute('aria-expanded', 'false')
  expect(screen.getByRole('button', { name: 'Disconnect Apple Wallet' })).toBeInTheDocument()
})

it('links to its own inbox with the count of this source\'s rows waiting there', async () => {
  const row = { linkId: 'l1', sourceId: 's1', externalAccountId: 'wallet', accountId: '', payee: 'Blue Bottle', amount: '5', currency: 'USD', type: 'expense', postedAt: '2026-08-20 17:42:03', reason: 'unmapped' }
  const failed = { eventId: 'e1', sourceId: 's1', receivedAt: '2026-08-21 08:00:00', error: 'bad', payload: '{}' }
  server.use(...coreHandlers({ importSources: [wireSource], importQueue: { queued: [row, { ...row, linkId: 'l2' }, { ...row, linkId: 'l3', sourceId: 'other' }], skipped: [], failed: [failed] } }))
  renderPage()
  const link = await screen.findByRole('link', { name: /^Inbox/ })
  expect(link).toHaveAttribute('href', '/settings/apple-wallet/inbox')
  await waitFor(() => expect(link).toHaveTextContent('Inbox3'))
})

it('the inbox row carries no count when nothing from this source is waiting', async () => {
  server.use(...coreHandlers({ importSources: [wireSource], importQueue: { queued: [], skipped: [], failed: [] } }))
  renderPage()
  expect(await screen.findByText('Connected')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: /^Inbox/ })).toHaveTextContent(/^Inbox$/)
})

it('before the first tap the status names the current setup step and the open guide follows it, with no inbox or card list', async () => {
  server.use(...coreHandlers({ importSources: [{ ...wireSource, cards: [] }], importQueue: { queued: [], skipped: [], failed: [] } }))
  renderPage()
  expect(await screen.findByText('Setting up')).toBeInTheDocument()
  expect(screen.getByText('Step 1 of 5: Install the Shortcut')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Install the Shortcut' })).toHaveAttribute('aria-expanded', 'true')
  expect(screen.queryByRole('button', { name: /Setup guide/ })).toBeNull()
  expect(screen.queryByRole('link', { name: /^Inbox/ })).toBeNull()
  expect(screen.queryByRole('heading', { name: 'Cards' })).toBeNull()
})
