import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { formatDate } from '@/lib/datetime'
import { expiresAtFrom, PersonalTokensPage } from './PersonalTokensPage'

const pat = {
  id: '01890000-0000-7000-8000-00000000d001',
  name: 'Home Assistant',
  createdAt: '2026-07-01 10:00:00',
  lastUsedAt: '2026-07-10 09:00:00',
  expiresAt: null,
}

function mockViewport() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const router = createMemoryRouter([{ path: '/settings/profile/tokens', element: <PersonalTokensPage /> }], {
    initialEntries: ['/settings/profile/tokens'],
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockViewport()
  server.use(
    http.get('*/api/v1/user/get-personal-token-list', () =>
      HttpResponse.json({ success: true, message: '', data: [pat] }),
    ),
  )
})

it('lists tokens with name and expiry', async () => {
  renderPage()
  expect(await screen.findByText('Home Assistant')).toBeInTheDocument()
  expect(screen.getByText(/Never expires/)).toBeInTheDocument()
})

it('creates a token and shows it exactly once with a copy button', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/create-personal-token', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({
        success: true,
        message: '',
        data: { id: 'new-id', name: 'CI', token: 'eco_pat_shown-once-value', createdAt: '2026-07-10 12:00:00', expiresAt: null },
      })
    }),
  )
  const user = userEvent.setup()
  // userEvent installs its own clipboard stub at setup(); override after it.
  const writeText = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
  renderPage()
  await user.click(await screen.findByRole('button', { name: /Create token/ }))
  await user.type(await screen.findByLabelText('Name'), 'CI')
  // default expiry choice is "Never"
  await user.click(screen.getByRole('button', { name: 'Create token' }))

  expect(await screen.findByTestId('created-token')).toHaveTextContent('eco_pat_shown-once-value')
  expect(screen.getByText(/won't be able to see it again/)).toBeInTheDocument()
  expect(body).toEqual({ name: 'CI', expiresAt: '' })

  await user.click(screen.getByRole('button', { name: 'Copy' }))
  expect(writeText).toHaveBeenCalledWith('eco_pat_shown-once-value')
  expect(await screen.findByRole('button', { name: 'Copied' })).toBeInTheDocument()

  // Closing the dialog discards the token; it is not cached anywhere.
  await user.click(screen.getByRole('button', { name: 'Done' }))
  await waitFor(() => expect(screen.queryByTestId('created-token')).not.toBeInTheDocument())
})

it('cancels the create dialog without creating anything', async () => {
  let called = false
  server.use(
    http.post('*/api/v1/user/create-personal-token', () => {
      called = true
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: /Create token/ }))
  await user.type(await screen.findByLabelText('Name'), 'CI')
  await user.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() => expect(screen.queryByLabelText('Name')).not.toBeInTheDocument())
  expect(called).toBe(false)
})

it('picks a custom expiry from the calendar and shows it on the chip', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/create-personal-token', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({
        success: true,
        message: '',
        data: { id: 'new-id', name: 'CI', token: 'eco_pat_x', createdAt: '2026-07-10 12:00:00', expiresAt: null },
      })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: /Create token/ }))
  await user.type(await screen.findByLabelText('Name'), 'CI')

  await user.click(screen.getByRole('button', { name: 'Custom date' }))
  const grid = await screen.findByRole('grid')

  const tomorrow = new Date()
  tomorrow.setDate(tomorrow.getDate() + 1)
  const dayButton = within(grid)
    .getAllByText(String(tomorrow.getDate()))
    .map((el) => el.closest('button'))
    .find((b): b is HTMLButtonElement => b !== null && !b.disabled)
  await user.click(dayButton!)

  // The calendar closes and the picked date replaces the chip label.
  await waitFor(() => expect(screen.queryByRole('grid')).not.toBeInTheDocument())
  expect(screen.getByRole('button', { name: formatDate(tomorrow) })).toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Create token' }))
  await waitFor(() => expect(body).toEqual({ name: 'CI', expiresAt: `${formatDate(tomorrow)} 23:59:59` }))
})

it('requires a name', async () => {
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: /Create token/ }))
  await user.click(await screen.findByRole('button', { name: 'Create token' }))
  expect(await screen.findByText('Enter a token name')).toBeInTheDocument()
})

it('revokes a token after confirmation', async () => {
  let revokedId: unknown
  server.use(
    http.post('*/api/v1/user/revoke-personal-token', async ({ request }) => {
      revokedId = ((await request.json()) as { id: string }).id
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.click(await screen.findByRole('button', { name: 'Revoke' }))
  const buttons = await screen.findAllByRole('button', { name: 'Revoke' })
  await user.click(buttons[buttons.length - 1])
  await waitFor(() => expect(revokedId).toBe(pat.id))
})

it('expiresAtFrom computes UTC datetimes from choices', () => {
  const now = new Date(Date.UTC(2026, 6, 10, 12, 0, 0))
  expect(expiresAtFrom('d30', '', now)).toBe('2026-08-09 12:00:00')
  expect(expiresAtFrom('d90', '', now)).toBe('2026-10-08 12:00:00')
  expect(expiresAtFrom('d365', '', now)).toBe('2027-07-10 12:00:00')
  expect(expiresAtFrom('custom', '2030-01-01', now)).toBe('2030-01-01 23:59:59')
  expect(expiresAtFrom('never', '', now)).toBeNull()
})
