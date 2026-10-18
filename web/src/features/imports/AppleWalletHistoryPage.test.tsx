import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { useUiStore } from '@/app/uiStore'
import { AppleWalletHistoryPage } from './AppleWalletHistoryPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))

const wallet = {
  id: 's1', provider: 'apple-wallet', name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00',
  cards: [{ externalAccountId: 'amex gold', externalName: 'Amex Gold', externalCurrency: '', state: 'mapped', accountId: 'a1', queuedCount: 0, tapCount: 3, lastSeenAt: '2026-08-21 12:00:00' }],
}
const row = (id: string, over: Record<string, unknown>) => ({
  id, externalAccountId: 'amex gold', externalTransactionId: `tap-${id}`, transactionId: 't-' + id, status: 'linked',
  externalPayee: 'Shop', externalAmount: '5', externalCurrency: 'USD', externalPostedAt: '2026-08-21 12:00:00', ...over,
})

function renderPage(pages: Record<string, { items: unknown[]; hasMore: boolean }>, data: Record<string, unknown> = {}) {
  const offsets: string[] = []
  server.use(
    ...coreHandlers({ importSources: [wallet], ...data }),
    http.get('*/api/v1/import/get-source-history', ({ request }) => {
      const params = new URL(request.url).searchParams
      offsets.push(`${params.get('sourceId')}@${params.get('offset')}`)
      return HttpResponse.json({ success: true, message: '', data: pages[params.get('offset') ?? '0'] })
    }),
  )
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/apple-wallet/history', element: <AppleWalletHistoryPage /> }], { initialEntries: ['/settings/apple-wallet/history'] })
  render(<QueryClientProvider client={queryClient}><RouterProvider router={router} /></QueryClientProvider>)
  return offsets
}

// taps are stored in UTC and shown on the reader's clock, whatever zone the suite runs in
const localTime = (utc: string) => {
  const d = new Date(utc.replace(' ', 'T') + 'Z')
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = { IMPORT_APPLE_WALLET: true }
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({ matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
})

it('lists every tap by day with its card, time and outcome, and loads older taps on demand', async () => {
  const offsets = renderPage({
    0: {
      items: [
        row('1', { externalPayee: 'Blue Bottle', externalAmount: '6.5' }),
        row('2', { externalPayee: 'Refunded', transactionId: '' }),
        row('3', { externalPayee: 'Waiting', status: 'queued', transactionId: '', externalAccountId: 'apple card', externalPostedAt: '2026-08-20 09:05:00' }),
      ],
      hasMore: true,
    },
    3: { items: [row('4', { externalPayee: 'Oldest', status: 'skipped', transactionId: '', externalPostedAt: '2026-08-19 08:00:00' })], hasMore: false },
  })
  const user = userEvent.setup()
  const today = (await screen.findByText('21st August 2026')).closest('section') as HTMLElement
  expect(within(today).getByText('Blue Bottle')).toBeInTheDocument()
  expect(within(today).getByText('6.5 USD')).toBeInTheDocument()
  expect(within(today).getAllByText(`Amex Gold · ${localTime('2026-08-21 12:00:00')} · Imported`)).toHaveLength(1)
  expect(within(today).getByText('Refunded')).toHaveClass('line-through')
  // an unknown card shows its raw name
  const yesterday = screen.getByText('20th August 2026').closest('section') as HTMLElement
  expect(within(yesterday).getByText(`apple card · ${localTime('2026-08-20 09:05:00')} · Waiting for review`)).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Show more' }))
  expect(await screen.findByText('Oldest')).toBeInTheDocument()
  expect(screen.getByText(/Skipped$/)).toBeInTheDocument()
  await waitFor(() => expect(screen.queryByRole('button', { name: 'Show more' })).not.toBeInTheDocument())
  expect(offsets).toEqual(['s1@0', 's1@3'])
})

it('says so when nothing has arrived yet', async () => {
  renderPage({ 0: { items: [], hasMore: false } })
  expect(await screen.findByText('No imports yet.')).toBeInTheDocument()
})

it('an imported tap opens the regular transaction preview', async () => {
  renderPage({ 0: { items: [row('1', { externalPayee: 'Blue Bottle', transactionId: 't1' })], hasMore: false } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: /Blue Bottle/ }))
  // t1 is the fixture transaction "coffee beans"
  expect(await screen.findByText('coffee beans')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /Edit/ })).toBeInTheDocument()
})

it('a skipped tap explains it was not added and offers to create the transaction', async () => {
  useUiStore.setState({ transactionModal: null })
  const skipped = { linkId: '4', sourceId: 's1', externalAccountId: 'amex gold', accountId: 'a1', payee: 'Parking', amount: '3', currency: 'USD', type: 'expense', postedAt: '2026-08-19 08:00:00', reason: 'unmapped' }
  renderPage(
    { 0: { items: [row('4', { externalPayee: 'Parking', status: 'skipped', transactionId: '', externalAmount: '3' })], hasMore: false } },
    { importQueue: { queued: [], skipped: [skipped], failed: [] } },
  )
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: /Parking/ }))
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText("This transaction wasn't added to Econumo.")).toBeInTheDocument()
  expect(within(dialog).getByText('Amex Gold')).toBeInTheDocument()
  expect(within(dialog).getByText('3 USD')).toBeInTheDocument()
  await user.click(within(dialog).getByRole('button', { name: 'Create transaction' }))
  expect(useUiStore.getState().transactionModal).toMatchObject({ importQueued: { linkId: '4', type: 'expense', accountId: 'a1', payee: 'Parking' } })
})

it('a tap whose transaction was deleted says so, with nothing to create', async () => {
  renderPage({ 0: { items: [row('2', { externalPayee: 'Refunded', transactionId: '' })], hasMore: false } })
  const user = userEvent.setup()
  await user.click(await screen.findByRole('button', { name: /Refunded/ }))
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText('This transaction was imported and deleted later.')).toBeInTheDocument()
  expect(within(dialog).queryByRole('button', { name: 'Create transaction' })).toBeNull()
})
