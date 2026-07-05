import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { queryKeys } from '@/app/queryKeys'
import { ProfilePage } from './ProfilePage'

function mockViewport(compact = false) {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: q.includes('1023') ? compact : false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderWithHistory(initialEntries: string[], initialIndex: number) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/account/:id', element: <div>ACCOUNT ROUTE</div> },
      { path: '/settings', element: <div>SETTINGS HUB ROUTE</div> },
      { path: '/settings/profile', element: <ProfilePage /> },
    ],
    { initialEntries, initialIndex },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/settings/profile', element: <ProfilePage /> },
      { path: '/logout', element: <div>LOGOUT ROUTE</div> },
      { path: '/settings/profile/change-password', element: <div>CHANGE PASSWORD ROUTE</div> },
    ],
    { initialEntries: ['/settings/profile'] },
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
  mockViewport()
})

it('mobile back returns to the previous url', async () => {
  mockViewport(true)
  const user = userEvent.setup()
  renderWithHistory(['/account/a1', '/settings/profile'], 1)
  await user.click(await screen.findByRole('button', { name: 'back' }))
  expect(await screen.findByText('ACCOUNT ROUTE')).toBeInTheDocument()
})

it('mobile back falls back to the settings hub on a deep link', async () => {
  mockViewport(true)
  const user = userEvent.setup()
  renderWithHistory(['/settings/profile'], 0)
  await user.click(await screen.findByRole('button', { name: 'back' }))
  expect(await screen.findByText('SETTINGS HUB ROUTE')).toBeInTheDocument()
})

it('saves the name on blur and updates the cache', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/update-name', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { user: { ...fixtureUser, name: 'Grace' } } })
    }),
  )
  const user = userEvent.setup()
  const queryClient = renderPage()
  const nameInput = await screen.findByLabelText('Name')
  await waitFor(() => expect(nameInput).toHaveValue('Ada'))
  await user.clear(nameInput)
  await user.type(nameInput, 'Grace')
  await user.tab()
  await waitFor(() => expect(body).toEqual({ name: 'Grace' }))
  await waitFor(() => expect(queryClient.getQueryData<{ name: string }>(queryKeys.user)!.name).toBe('Grace'))
})

it('surfaces server field errors under the name input', async () => {
  server.use(
    http.post('*/api/v1/user/update-name', () =>
      HttpResponse.json(
        { success: false, message: 'Form validation error', code: 400, errors: { name: ['This value is too long.'] } },
        { status: 400 },
      ),
    ),
  )
  const user = userEvent.setup()
  renderPage()
  const nameInput = await screen.findByLabelText('Name')
  await waitFor(() => expect(nameInput).toHaveValue('Ada'))
  await user.clear(nameInput)
  await user.type(nameInput, 'A very long name over twenty chars')
  await user.tab()
  expect(await screen.findByText('This value is too long.')).toBeInTheDocument()
})

it('email is readonly; logout confirm has the exact copy and navigates', async () => {
  const user = userEvent.setup()
  renderPage()
  expect(await screen.findByLabelText('E-mail')).toBeDisabled()
  await user.click(await screen.findByText('Logout'))
  expect(await screen.findByText('Are you sure you want to log out?')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Stay' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Logout' }))
  expect(await screen.findByText('LOGOUT ROUTE')).toBeInTheDocument()
})
