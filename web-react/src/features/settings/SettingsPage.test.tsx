import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { SettingsPage } from './SettingsPage'

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
})

it('renders the menu rows with exact labels and navigates', async () => {
  mockViewport(false)
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByText('Service settings')).toBeInTheDocument()
  // Full sync moved to the sidebar footer refresh button
  expect(screen.queryByText('Full sync')).not.toBeInTheDocument()
  // grouped menu; currency moved to the profile page, so with a single locale
  // there is no Preferences group at all
  expect(screen.getByText('Service')).toBeInTheDocument()
  expect(screen.getByText('Classification')).toBeInTheDocument()
  expect(screen.getByText('Data')).toBeInTheDocument()
  expect(screen.queryByText('Preferences')).not.toBeInTheDocument()
  expect(screen.queryByText('Default currency')).not.toBeInTheDocument()
  expect(screen.getByText('Shared access')).toBeInTheDocument()
  expect(screen.getByText('Accounts and Folders')).toBeInTheDocument()
  expect(screen.getByText('Payees (senders, recipients)')).toBeInTheDocument()
  // language group hidden with a single locale
  expect(screen.queryByText('User Interface')).not.toBeInTheDocument()
  expect(screen.queryByText('Language')).not.toBeInTheDocument()

  await user.click(screen.getByText('Budgets'))
  expect(await screen.findByText('BUDGETS PAGE')).toBeInTheDocument()
})


it('Import CSV and Export CSV rows open their dialogs', async () => {
  server.use(...coreHandlers())
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByText('Import CSV'))
  expect(await screen.findByText('Maximum file size: 10 MB')).toBeInTheDocument()
})
