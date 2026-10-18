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
  return render(
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

it.each([false, true])('uses the same setup card before and after receiving payments (payments received: %s)', async (hasPayments) => {
  server.use(...coreHandlers({ importSources: [{ ...wireSource, cards: hasPayments ? wireSource.cards : [] }] }))
  renderPage()
  const instructions = await screen.findByRole('link', { name: 'Setup instructions' })
  expect(instructions).toHaveAttribute('href', 'https://econumo.com/docs/user-guide/apple-wallet')
  expect(instructions).toHaveAttribute('target', '_blank')
  expect(screen.getAllByRole('link', { name: 'Setup instructions' })).toHaveLength(1)
  const card = instructions.closest('.rounded-lg')!
  const hint = screen.getByText(/Open this page on your iPhone/)
  expect(hint).toBeVisible()
  expect(hint.nextElementSibling).toContainElement(instructions)
  expect(card).toContainElement(screen.getByRole('button', { name: 'Install the Shortcut' }))
  expect(card).toContainElement(screen.getByRole('button', { name: 'Make the first payment' }))
  expect(instructions).toBeVisible()
  expect(screen.getByRole('heading', { name: 'Setup guide', level: 2 })).toBeVisible()
  expect(screen.queryByRole('button', { name: /Setup guide/ })).toBeNull()
  if (hasPayments) expect(card).toContainElement(screen.getByRole('button', { name: 'Configure again' }))
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

it('shows the card list without the Connected / Last tap status block', async () => {
  server.use(...coreHandlers({ importSources: [wireSource] }))
  renderPage()
  expect(await screen.findByRole('heading', { name: 'Needs mapping' })).toBeInTheDocument()
  expect(screen.queryByText('Connected')).not.toBeInTheDocument()
  expect(screen.queryByText(/^Last tap /)).not.toBeInTheDocument()
  expect(screen.getByText('Apple Card')).toBeInTheDocument()
  expect(screen.getByText(/^2 to review/)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Install the Shortcut' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Make the first payment' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Disconnect Apple Wallet' })).toBeInTheDocument()
})

it('links to its import history, and no longer to an inbox of its own', async () => {
  server.use(...coreHandlers({ importSources: [wireSource], importQueue: { queued: [], skipped: [], failed: [] } }))
  renderPage()
  expect(await screen.findByRole('link', { name: 'Import history' })).toHaveAttribute('href', '/settings/apple-wallet/history')
  expect(screen.queryByRole('link', { name: /Inbox/ })).toBeNull()
})

it('before the first tap it shows the open setup guide without a status block, history link or card list', async () => {
  server.use(...coreHandlers({ importSources: [{ ...wireSource, cards: [] }], importQueue: { queued: [], skipped: [], failed: [] } }))
  renderPage()
  expect(await screen.findByRole('button', { name: 'Install the Shortcut' })).toHaveAttribute('aria-expanded', 'true')
  expect(screen.queryByText('Setting up')).not.toBeInTheDocument()
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
  expect(await screen.findByRole('button', { name: 'Make the first payment' })).toHaveAttribute('aria-expanded', 'true')
  await waitFor(() => expect(sourceRequests).toBeGreaterThan(1))
  expect(screen.getByRole('button', { name: 'Make the first payment' }).closest('li')).toHaveAttribute('data-state', 'current')
  expect(screen.getByRole('status')).toHaveTextContent('Waiting for the first payment')
  expect(screen.queryByRole('button', { name: 'Done' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Switch the automation to Run Immediately' }))
  expect(screen.getByRole('button', { name: 'Done' })).toBeDisabled()

  arrived = true
  act(() => document.dispatchEvent(new Event('visibilitychange')))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Make the first payment' }).closest('li')).toHaveAttribute('data-state', 'done'))
  expect(screen.getByRole('button', { name: 'Switch the automation to Run Immediately' }).closest('li')).toHaveAttribute('data-state', 'current')
  expect(screen.getByRole('button', { name: 'Done' })).toBeEnabled()
  expect(screen.queryByText('Waiting for the first payment…')).not.toBeInTheDocument()
})

it('Configure again persists a full reset, ignores previous events, and requires a new payment', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['immediate']))
  const user = userEvent.setup()
  const oldFailure = { eventId: 'old-check', sourceId: 's1', receivedAt: '2026-08-20 17:40:00', error: 'account is required', payload: '{}' }
  const oldQueue = { queued: [{ sourceId: 's1', linkId: 'old-queued' }], skipped: [{ sourceId: 's1', linkId: 'old-skipped' }], failed: [oldFailure] }
  let taps = 3
  let newCheck = false
  let sourceRequests = 0
  server.use(...coreHandlers())
  server.use(
    http.get('*/api/v1/import/get-source-list', () => {
      sourceRequests++
      return HttpResponse.json({ success: true, message: '', data: { items: [{ ...wireSource, cards: [{ ...wireSource.cards[0], tapCount: taps }] }] } })
    }),
    http.get('*/api/v1/import/get-queued-event-list', () => HttpResponse.json({
      success: true, message: '', data: { ...oldQueue, failed: [...oldQueue.failed, ...(newCheck ? [{ ...oldFailure, eventId: 'new-check' }] : [])] },
    })),
    http.post('*/api/v1/import/discard-event', async ({ request }) => {
      expect(await request.json()).toEqual({ eventId: 'new-check' })
      newCheck = false
      return HttpResponse.json({ success: true, message: '', data: oldQueue })
    }),
  )
  const view = renderPage()
  const immediateStep = () => screen.getByRole('button', { name: 'Switch the automation to Run Immediately' }).closest('li')
  const guide = await screen.findByRole('button', { name: 'Setup guide' })
  expect(guide).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByRole('button', { name: 'Install the Shortcut' })).not.toBeInTheDocument()
  await user.click(guide)
  expect(immediateStep()).toHaveAttribute('data-state', 'done')
  await user.click(screen.getByRole('button', { name: 'Configure again' }))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Install the Shortcut' })).toHaveAttribute('aria-expanded', 'true'))
  expect(immediateStep()).toHaveAttribute('data-state', 'todo')
  expect(screen.getByRole('link', { name: 'Import history' })).toBeInTheDocument()
  expect(screen.getByText('Apple Card')).toBeInTheDocument()

  view.unmount()
  renderPage()
  expect(await screen.findByRole('button', { name: 'Install the Shortcut' })).toHaveAttribute('aria-expanded', 'true')
  expect(immediateStep()).toHaveAttribute('data-state', 'todo')
  const beforeConfigure = sourceRequests
  await user.click(screen.getByRole('button', { name: 'Done' }))
  await waitFor(() => expect(sourceRequests).toBeGreaterThan(beforeConfigure))
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Waiting for the Shortcut to reach Econumo'))
  expect(screen.getByRole('button', { name: 'Install the Shortcut' }).closest('li')).toHaveAttribute('data-state', 'done')
  expect(screen.getByRole('button', { name: 'Create the automation' }).closest('li')).toHaveAttribute('data-state', 'todo')

  newCheck = true
  act(() => document.dispatchEvent(new Event('visibilitychange')))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Create the automation' })).toHaveAttribute('aria-expanded', 'true'))
  await user.click(screen.getByRole('button', { name: 'Done' }))
  expect(screen.getByRole('status')).toHaveTextContent('Waiting for the first payment')
  await user.click(screen.getByRole('button', { name: 'Switch the automation to Run Immediately' }))
  expect(screen.getByRole('button', { name: 'Done' })).toBeDisabled()

  taps++
  act(() => document.dispatchEvent(new Event('visibilitychange')))
  await waitFor(() => expect(screen.getByRole('button', { name: 'Done' })).toBeEnabled())
  expect(screen.getByRole('button', { name: 'Make the first payment' }).closest('li')).toHaveAttribute('data-state', 'done')
  await user.click(screen.getByRole('button', { name: 'Done' }))
  expect(immediateStep()).toHaveAttribute('data-state', 'done')
})
