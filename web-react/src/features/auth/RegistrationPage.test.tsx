import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { RegistrationPage } from './RegistrationPage'

function renderPage() {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  const router = createMemoryRouter(
    [
      { path: '/register', element: <RegistrationPage /> },
      { path: '/login', element: <div>LOGIN PAGE</div> },
    ],
    { initialEntries: ['/register'] },
  )
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { mutations: { retry: false } } })}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
})

it('registers and navigates to the login page', async () => {
  let body: unknown
  server.use(
    http.post('*/api/v1/user/register-user', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: { user: { id: 'u1', name: 'Ada', avatar: '' } } })
    }),
  )
  const user = userEvent.setup()
  renderPage()
  await user.type(screen.getByLabelText('Name'), 'Ada')
  await user.type(screen.getByLabelText('E-mail'), 'ada@example.test')
  await user.type(screen.getByLabelText('Password'), 'secret1')
  await user.type(screen.getByLabelText('Confirm password'), 'secret1')
  await user.click(screen.getByRole('button', { name: /sign up/i }))
  expect(await screen.findByText('LOGIN PAGE')).toBeInTheDocument()
  expect(body).toEqual({ email: 'ada@example.test', password: 'secret1', name: 'Ada' })
})

it('rejects mismatched password retry', async () => {
  const user = userEvent.setup()
  renderPage()
  await user.type(screen.getByLabelText('Password'), 'secret1')
  await user.type(screen.getByLabelText('Confirm password'), 'different')
  await user.click(screen.getByRole('button', { name: /sign up/i }))
  expect(await screen.findByText('Passwords do not match')).toBeInTheDocument()
})

it('shows the paywall instead of the form when enabled', () => {
  window.econumoConfig = { PAYWALL_ENABLED: 'true' }
  renderPage()
  expect(screen.queryByRole('button', { name: /sign up/i })).not.toBeInTheDocument()
  expect(screen.getByRole('link')).toHaveAttribute('href', 'https://pay.econumo.com/cloud/')
})
