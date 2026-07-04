import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { coreHandlers, fixtureConnections } from '@/test/fixtures'
import { ConnectionsPage } from './ConnectionsPage'

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter(
    [
      { path: '/settings/connections', element: <ConnectionsPage /> },
      { path: '/settings', element: <div>SETTINGS HUB</div> },
    ],
    { initialEntries: ['/settings/connections'] },
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
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})

it('lists connections; empty override shows the empty state', async () => {
  server.use(...coreHandlers({ connections: fixtureConnections }))
  renderPage()
  expect(await screen.findByText('Partner')).toBeInTheDocument()
  expect(screen.getByRole('img', { name: 'Partner' })).toBeInTheDocument()
})

it('shows the empty state without connections', async () => {
  server.use(...coreHandlers())
  renderPage()
  expect(await screen.findByText('List is empty')).toBeInTheDocument()
})

it('generate invite posts an empty body and shows the code dialog', async () => {
  let body: unknown
  server.use(
    ...coreHandlers(),
    http.post('*/api/v1/connection/generate-invite', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { item: { code: 'aB3f9', expiredAt: '2026-07-03 12:05:00' } } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Create an invitation' }))
  expect(await screen.findByTestId('invite-code')).toHaveTextContent('aB3f9')
  expect(screen.getByText(/The code is valid for 5 minutes\./)).toBeInTheDocument()
  expect(body).toEqual({})
})

it('accept invite posts the code and renders the refreshed list', async () => {
  let body: unknown
  server.use(
    ...coreHandlers(),
    http.post('*/api/v1/connection/accept-invite', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { items: fixtureConnections } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Accept an invitation' }))
  await user.type(await screen.findByLabelText('Enter the code'), 'aB3f9')
  await user.click(screen.getByRole('button', { name: 'Accept' }))
  expect(await screen.findByText('Partner')).toBeInTheDocument()
  expect(body).toEqual({ code: 'aB3f9' })
  await waitFor(() => expect(screen.queryByLabelText('Enter the code')).not.toBeInTheDocument())
})

it('accept invite failure keeps the dialog open with the server message', async () => {
  server.use(
    ...coreHandlers(),
    http.post('*/api/v1/connection/accept-invite', () =>
      HttpResponse.json({ success: false, message: 'ConnectionCode is incorrect', code: 400, errors: {} }, { status: 400 }),
    ),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Accept an invitation' }))
  await user.type(await screen.findByLabelText('Enter the code'), 'zzzzz')
  await user.click(screen.getByRole('button', { name: 'Accept' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('ConnectionCode is incorrect')
  expect(screen.getByLabelText('Enter the code')).toBeInTheDocument()
})

it('delete connection confirms with the name and posts the user id', async () => {
  let body: unknown
  server.use(
    ...coreHandlers({ connections: fixtureConnections }),
    http.post('*/api/v1/connection/delete-connection', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'connection actions Partner' }))
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText(/Are you sure you want to delete shared access from Partner\?/)).toBeInTheDocument()
  await user.click(within(dialog).getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(body).toEqual({ id: 'u2' }))
})
