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
  expect(screen.getByText('Full sync')).toBeInTheDocument()
  expect(screen.getByText('Shared access')).toBeInTheDocument()
  expect(screen.getByText('Accounts and Folders')).toBeInTheDocument()
  expect(screen.getByText('Payees (senders, recipients)')).toBeInTheDocument()
  expect(screen.getByText('Default currency')).toBeInTheDocument()
  // language group hidden with a single locale
  expect(screen.queryByText('User Interface')).not.toBeInTheDocument()
  expect(screen.queryByText('Language')).not.toBeInTheDocument()

  await user.click(screen.getByText('Budgets'))
  expect(await screen.findByText('BUDGETS PAGE')).toBeInTheDocument()
})

it('changing the default currency posts the code and updates the cache', async () => {
  mockViewport(false)
  let body: unknown
  server.use(
    http.post('*/api/v1/user/update-currency', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({
        success: true, message: '',
        data: { user: { ...fixtureUser, options: fixtureUser.options.map((o) => (o.name === 'currency_id' ? { ...o, value: 'cur-eur' } : o)) } },
      })
    }),
  )
  const user = userEvent.setup()
  const queryClient = renderPage()
  await screen.findByText('Default currency')
  await waitFor(() => expect(screen.getByRole('combobox', { name: 'Default currency' })).toHaveTextContent('USD'))
  await user.click(screen.getByRole('combobox', { name: 'Default currency' }))
  await user.click(await screen.findByText('EUR, €, Euro'))
  await waitFor(() => expect(body).toEqual({ currency: 'EUR' }))
  await waitFor(() => {
    const cached = queryClient.getQueryData<typeof fixtureUser>(queryKeys.user)!
    expect(cached.options.find((o) => o.name === 'currency_id')!.value).toBe('cur-eur')
  })
})
