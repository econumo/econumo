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
  await user.type(screen.getByLabelText('Email'), 'ada@example.test')
  await user.type(screen.getByLabelText('Password'), 'secret12')
  await user.type(screen.getByLabelText('Confirm password'), 'secret12')
  await user.click(screen.getByRole('button', { name: /sign up/i }))
  expect(await screen.findByText('LOGIN PAGE')).toBeInTheDocument()
  expect(body).toEqual({ email: 'ada@example.test', password: 'secret12', name: 'Ada' })
})

it('rejects mismatched password retry', async () => {
  const user = userEvent.setup()
  renderPage()
  await user.type(screen.getByLabelText('Password'), 'secret12')
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

it('persists the collapse and clears the server address', async () => {
  window.econumoConfig = { ALLOW_CUSTOM_API: 'true' }
  localStorage.setItem('selfHosted', 'true')
  localStorage.setItem('backendHost', JSON.stringify('https://old.example.test'))
  const user = userEvent.setup()
  renderPage()
  expect(screen.getByLabelText('Server address')).toHaveValue('https://old.example.test')
  await user.click(screen.getByRole('button', { name: /custom server/i }))
  expect(localStorage.getItem('selfHosted')).toBe('false')
  expect(localStorage.getItem('backendHost')).toBeNull()
})

it('stores the server address as it is typed', async () => {
  window.econumoConfig = { ALLOW_CUSTOM_API: 'true' }
  const user = userEvent.setup()
  renderPage()
  await user.click(screen.getByRole('button', { name: /custom server/i }))
  const host = screen.getByLabelText('Server address')
  await user.clear(host)
  await user.type(host, 'https://my.box.test')
  expect(localStorage.getItem('backendHost')).toBe(JSON.stringify('https://my.box.test'))
})
