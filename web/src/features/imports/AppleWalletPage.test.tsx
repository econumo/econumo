import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
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

it('links to its import history, and no longer to an inbox of its own', async () => {
  server.use(...coreHandlers({ importSources: [wireSource], importQueue: { queued: [], skipped: [], failed: [] } }))
  renderPage()
  expect(await screen.findByText('Connected')).toBeInTheDocument()
  expect(screen.getByRole('link', { name: 'Import history' })).toHaveAttribute('href', '/settings/apple-wallet/history')
  expect(screen.queryByRole('link', { name: /Inbox/ })).toBeNull()
})

it('before the first tap the status names the current setup step and the open guide follows it, with no history link or card list', async () => {
  server.use(...coreHandlers({ importSources: [{ ...wireSource, cards: [] }], importQueue: { queued: [], skipped: [], failed: [] } }))
  renderPage()
  expect(await screen.findByText('Setting up')).toBeInTheDocument()
  expect(screen.getByText('Step 1 of 5: Install the Shortcut')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Install the Shortcut' })).toHaveAttribute('aria-expanded', 'true')
  expect(screen.queryByRole('button', { name: /Setup guide/ })).toBeNull()
  expect(screen.queryByRole('link', { name: 'Import history' })).toBeNull()
  expect(screen.queryByRole('heading', { name: 'Cards' })).toBeNull()
})

it('automatically completes step 4 when polling receives the first non-failed payment', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install', 'configure', 'automate', 'immediate']))
  const user = userEvent.setup()
  server.use(...coreHandlers({ importQueue: { queued: [], skipped: [], failed: [{ eventId: 'e1', sourceId: 's1', receivedAt: '2026-08-20 17:40:00', error: 'amount must be a positive number', payload: '{}' }] } }))
  let arrived = false
  let sourceRequests = 0
  server.use(http.get('*/api/v1/import/get-source-list', () => {
    sourceRequests++
    return HttpResponse.json({ success: true, message: '', data: { items: [{ ...wireSource, cards: arrived ? wireSource.cards : [] }] } })
  }))
  renderPage()
  expect(await screen.findByText('Step 4 of 5: Make the first payment')).toBeInTheDocument()
  await waitFor(() => expect(sourceRequests).toBeGreaterThan(1))
  expect(screen.getByRole('button', { name: 'Make the first payment' }).closest('li')).toHaveAttribute('data-state', 'current')
  expect(screen.getByRole('status')).toHaveTextContent('Waiting for the first payment')
  expect(screen.queryByRole('button', { name: 'Mark as done' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Switch the automation to Run Immediately' }))
  expect(screen.getByRole('button', { name: 'Done' })).toBeDisabled()

  arrived = true
  act(() => document.dispatchEvent(new Event('visibilitychange')))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Make the first payment' }).closest('li')).toHaveAttribute('data-state', 'done'))
  expect(screen.getByRole('button', { name: 'Switch the automation to Run Immediately' }).closest('li')).toHaveAttribute('data-state', 'current')
  expect(screen.getByRole('button', { name: 'Done' })).toBeEnabled()
  expect(screen.queryByText('Waiting for the first payment…')).not.toBeInTheDocument()
})
