import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { ImportsDataPage } from './ImportsDataPage'

vi.mock('@/hooks/useIsCompact', () => ({ useIsCompact: () => false }))

const simplefinSource = {
  id: 's2', provider: 'simplefin', name: 'My Bank', status: 'active', createdAt: '2026-09-01 00:00:00',
  lastSyncedAt: '', credentialCiphertext: 'ciphertext', cards: [],
}
const wrappedKey = { wrappedDataKey: 'wrapped', kdf: 'pbkdf2', updatedAt: '2026-09-01 00:00:00' }

function renderPage(overrides: Parameters<typeof coreHandlers>[0] = {}) {
  server.use(...coreHandlers(overrides))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [{ path: '/settings/data', element: <ImportsDataPage /> }, { path: '/settings/simplefin', element: <div>SIMPLEFIN PAGE</div> }],
    { initialEntries: ['/settings/data'] },
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
