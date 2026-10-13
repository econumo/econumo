import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http } from 'msw'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers, fixtureAccounts } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { Toaster } from '@/components/ui/sonner'
import { useTransactions } from '@/features/transactions/queries'
import { resetRevisions } from '@/features/transactions/revisionTracking'
import { METRICS, trackEvent } from '@/lib/metrics'
import { ImportsDataPage } from './ImportsDataPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))
vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

// the application shell keeps the transactions list mounted on every page
function TransactionsMounted() {
  useTransactions()
  return null
}

const simplefinSource = {
  id: 's2', provider: 'simplefin', name: 'My Bank', status: 'active', createdAt: '2026-09-01 00:00:00',
  lastSyncedAt: '', credentialCiphertext: 'ciphertext', cards: [],
}
const wrappedKey = { wrappedDataKey: 'wrapped', kdf: 'pbkdf2', updatedAt: '2026-09-01 00:00:00' }

function renderPage(overrides: Parameters<typeof coreHandlers>[0] = {}) {
  server.use(...coreHandlers(overrides))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/settings/data', element: <><ImportsDataPage /><TransactionsMounted /><Toaster /></> },
      { path: '/settings/simplefin', element: <div>SIMPLEFIN PAGE</div> },
    ],
    { initialEntries: ['/settings/data'] },
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
  window.econumoConfig = { IMPORT_APPLE_WALLET: true, IMPORT_SIMPLEFIN: true }
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('renders the CSV rows and opens the import dialog', async () => {
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText('Import & export')).toBeInTheDocument()
  expect(screen.getByText('Export CSV')).toBeInTheDocument()
  await user.click(screen.getByText('Import CSV'))
  expect(await screen.findByText('Maximum file size: 10 MB')).toBeInTheDocument()
})

it('holds no Apple Wallet section — that has its own page', async () => {
  renderPage()
  await screen.findByText('Import & export')
  expect(screen.queryByText('Apple Wallet')).not.toBeInTheDocument()
})

it('Sync all is hidden without a pull source and routes to SimpleFIN settings while the device is locked', async () => {
  renderPage()
  expect(await screen.findByText('Import & export')).toBeInTheDocument()
  expect(screen.queryByText('Sync bank connections')).toBeNull()
})

it('Sync all navigates to SimpleFIN settings when the device key is locked', async () => {
  const user = userEvent.setup()
  renderPage({ importSources: [simplefinSource], importCredentialKey: wrappedKey })
  await user.click(await screen.findByText('Sync bank connections'))
  expect(await screen.findByText('SIMPLEFIN PAGE')).toBeInTheDocument()
})

it('with transaction import off: only the CSV rows, and no import endpoint is called', async () => {
  window.econumoConfig = {}
  const importCalls: string[] = []
  const record = ({ request }: { request: Request }) => {
    if (request.url.includes('/api/v1/import/')) importCalls.push(request.url)
  }
  server.events.on('request:start', record)
  onTestFinished(() => server.events.removeListener('request:start', record))
  renderPage({ importSources: [simplefinSource] })
  expect(await screen.findByText('Import CSV')).toBeInTheDocument()
  expect(screen.getByText('Export CSV')).toBeInTheDocument()
  expect(screen.queryByText('Sync bank connections')).toBeNull()
  expect(screen.queryByText('Import rules')).toBeNull()
  expect(screen.queryByText('Import history')).toBeNull()
  expect(importCalls).toEqual([])
})

describe('Reload all data', () => {
  it('is the last row, with its hint', async () => {
    renderPage()
    const row = await screen.findByRole('button', { name: /Reload all data/ })
    expect(row).toHaveTextContent('Downloads everything from the server again. Use it if something looks out of date.')
    expect(screen.getAllByRole('button').at(-1)).toBe(row)
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

    await user.click(screen.getByRole('button', { name: /Reload all data/ }))
    expect(await screen.findByText('Reloading all data from the server…')).toBeInTheDocument()
    await waitFor(() => expect(queryClient.isFetching()).toBe(0))
    expect(asked).toEqual(fixtureAccounts.map((a) => a.id))
    expect(trackEvent).toHaveBeenCalledTimes(1)
    expect(trackEvent).toHaveBeenCalledWith(METRICS.DATA_FULL_RELOAD, { source: 'settings' })
  })

  it('cannot be pressed again while the transactions are loading', async () => {
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
    const row = screen.getByRole('button', { name: /Reload all data/ })
    await user.click(row)
    await waitFor(() => expect(row).toBeDisabled())
    release()
    await waitFor(() => expect(row).toBeEnabled())
    expect(trackEvent).toHaveBeenCalledTimes(1)
  })
})
