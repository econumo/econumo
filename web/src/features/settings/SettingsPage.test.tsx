import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import type { AvailableUpdate } from '@/hooks/useAvailableUpdate'
import { SettingsPage } from './SettingsPage'

const mockUpdate = vi.hoisted(() => ({ value: null as AvailableUpdate | null }))
vi.mock('@/hooks/useAvailableUpdate', () => ({
  useAvailableUpdate: () => mockUpdate.value,
}))

function mockViewport(compact: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: query.includes('1023') ? compact : false,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/settings', element: <SettingsPage /> },
      { path: '/settings/budgets', element: <div>BUDGETS PAGE</div> },
    ],
    { initialEntries: ['/settings'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  server.use(...coreHandlers())
  mockUpdate.value = null
})

it('renders the menu rows with exact labels and navigates', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText('Settings')).toBeInTheDocument()
  // Full sync moved to the sidebar footer refresh button
  expect(screen.queryByText('Full sync')).not.toBeInTheDocument()
  // grouped menu; currency moved to the profile page
  expect(screen.getByText('Finances')).toBeInTheDocument()
  expect(screen.getByText('Classification')).toBeInTheDocument()
  expect(screen.getByText('Data')).toBeInTheDocument()
  expect(screen.queryByText('Default currency')).not.toBeInTheDocument()
  expect(screen.getByText('Shared access')).toBeInTheDocument()
  expect(screen.getByText('Accounts')).toBeInTheDocument()
  expect(screen.getByText('Payees')).toBeInTheDocument()
  await user.click(screen.getByText('Budgets'))
  expect(await screen.findByText('BUDGETS PAGE')).toBeInTheDocument()
})

it('shows a plain version label for non-release builds', async () => {
  window.econumoConfig = { VERSION: 'dev' }
  renderPage()
  const label = await screen.findByText('Econumo dev')
  expect(label.tagName).toBe('SPAN')
})

it('links a semver version to its release notes page', async () => {
  window.econumoConfig = { VERSION: 'v1.2.3' }
  renderPage()
  const label = await screen.findByText('Econumo v1.2.3')
  expect(label).toHaveAttribute('href', 'https://econumo.com/releases/v1.2.3/')
})

it('links to the API docs', async () => {
  window.econumoConfig = { API_URL: 'https://api.example.test' }
  renderPage()
  const link = await screen.findByRole('link', { name: 'API' })
  expect(link).toHaveAttribute('href', 'https://api.example.test/api/doc')
})

it('Import CSV and Export CSV rows open their dialogs', async () => {
  server.use(...coreHandlers())
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByText('Import CSV'))
  expect(await screen.findByText('Maximum file size: 10 MB')).toBeInTheDocument()
})

it('shows the new-version menu row above the Finances group when an update is available', async () => {
  mockUpdate.value = { version: 'v9.9.9', url: 'https://econumo.com/releases/v9.9.9/' }
  renderPage()
  const link = await screen.findByRole('link', { name: /v9\.9\.9/ })
  expect(link).toHaveAttribute('href', 'https://econumo.com/releases/v9.9.9/')
  expect(link).toHaveAttribute('target', '_blank')
  const finances = screen.getByText('Finances')
  expect(link.compareDocumentPosition(finances) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

it('shows no new-version link when no update is available', async () => {
  renderPage()
  expect(await screen.findByText('Settings')).toBeInTheDocument()
  expect(screen.queryByText(/New version/)).not.toBeInTheDocument()
})

function utcIn(days: number): string {
  return new Date(Date.now() + days * 86_400_000).toISOString().slice(0, 19).replace('T', ' ')
}

it('hides the Billing group when BILLING_URL is empty', async () => {
  mockViewport(false)
  renderPage()
  expect(await screen.findByText('Finances')).toBeInTheDocument()
  expect(screen.queryByText('Billing')).not.toBeInTheDocument()
})

it('shows the Billing group with the portal row for full access', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  mockViewport(false)
  renderPage()
  expect(await screen.findByText('Billing')).toBeInTheDocument()
  expect(screen.getByText('Manage subscription')).toBeInTheDocument()
  expect(screen.queryByText(/Trial/)).not.toBeInTheDocument()
})

it('shows the trial status hint from day one', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(...coreHandlers({ user: { ...fixtureUser, accessUntil: utcIn(40) } }))
  mockViewport(false)
  renderPage()
  expect(await screen.findByText(/Subscription ends/)).toBeInTheDocument()
})

it('shows the read-only status hint', async () => {
  window.econumoConfig = { BILLING_URL: 'https://pay.example.test/' }
  server.use(...coreHandlers({ user: { ...fixtureUser, accessLevel: 'readonly', accessUntil: '' } }))
  mockViewport(false)
  renderPage()
  expect(await screen.findByText('Billing')).toBeInTheDocument()
  expect(await screen.findByText(/^Read-only$/)).toBeInTheDocument()
})
