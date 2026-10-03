import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { getConnectedApps, revokeConnectedApp } from '@/api/authserver'
import { METRICS, trackEvent } from '@/lib/metrics'
import { ConnectedAppsPage } from './ConnectedAppsPage'

vi.mock('@/api/authserver', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/api/authserver')>()),
  getConnectedApps: vi.fn(),
  revokeConnectedApp: vi.fn(),
}))
vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent: vi.fn(),
}))

const apps = [
  { id: 'g1', clientName: 'Claude', redirectHost: 'claude.ai', isLoopback: false, createdAt: '2026-10-01 10:00:00', lastUsedAt: '2026-10-03 09:00:00' },
  { id: 'g2', clientName: 'Codex', redirectHost: '127.0.0.1', isLoopback: true, createdAt: '2026-10-02 10:00:00', lastUsedAt: '2026-10-02 10:00:00' },
]

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/profile/connected-apps', element: <ConnectedAppsPage /> }], {
    initialEntries: ['/settings/profile/connected-apps'],
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  vi.mocked(getConnectedApps).mockReset()
  vi.mocked(revokeConnectedApp).mockReset().mockResolvedValue(undefined)
  vi.mocked(trackEvent).mockClear()
  localStorage.clear()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

test('lists connected apps with where they send you', async () => {
  vi.mocked(getConnectedApps).mockResolvedValue(apps)
  renderPage()
  expect(await screen.findByText('Claude')).toBeInTheDocument()
  expect(screen.getByText(/claude\.ai/)).toBeInTheDocument()
  expect(screen.getByText(/this computer/i)).toBeInTheDocument()
  expect(screen.getAllByText(/Last used/)).toHaveLength(2)
})

test('revoke asks for confirmation, calls the API and fires the metric', async () => {
  vi.mocked(getConnectedApps).mockResolvedValue(apps)
  const user = userEvent.setup()
  renderPage()
  const revokeButtons = await screen.findAllByRole('button', { name: 'Revoke' })
  await user.click(revokeButtons[0])

  expect(await screen.findByText('Claude will lose access to your Econumo data.')).toBeInTheDocument()
  expect(revokeConnectedApp).not.toHaveBeenCalled()

  const confirm = await screen.findAllByRole('button', { name: 'Revoke' })
  await user.click(confirm[confirm.length - 1])

  await waitFor(() => expect(revokeConnectedApp).toHaveBeenCalledWith('g1'))
  await waitFor(() => expect(trackEvent).toHaveBeenCalledWith(METRICS.CONNECTED_APP_REVOKE, {}))
})

test('cancelling the confirmation revokes nothing', async () => {
  vi.mocked(getConnectedApps).mockResolvedValue(apps)
  const user = userEvent.setup()
  renderPage()
  await user.click((await screen.findAllByRole('button', { name: 'Revoke' }))[0])
  await user.click(await screen.findByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(screen.queryByText(/will lose access/)).not.toBeInTheDocument())
  expect(revokeConnectedApp).not.toHaveBeenCalled()
  expect(trackEvent).not.toHaveBeenCalled()
})

test('empty state shows the MCP address with a copy button', async () => {
  vi.mocked(getConnectedApps).mockResolvedValue([])
  const writeText = vi.fn().mockResolvedValue(undefined)
  const user = userEvent.setup()
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
  renderPage()
  expect(await screen.findByText(`${window.location.origin}/mcp`)).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: /copy/i }))
  expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/mcp`)
  expect(await screen.findByRole('button', { name: 'Copied' })).toBeInTheDocument()
})
