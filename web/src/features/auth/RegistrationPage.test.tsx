import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { useServerConfig } from '@/lib/appConfig'
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
  server.use(http.get('*/api/v1/oauth/get-provider-list', () => HttpResponse.json({ success: true, message: '', data: [] })))
})

it('registers, remembers the new email, and navigates to the login page', async () => {
  // A different account was remembered before; registering must replace it with
  // the address just created so the login form pre-fills the right one.
  localStorage.setItem('rememberedEmail', JSON.stringify('someone.else@example.test'))
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
  expect(localStorage.getItem('rememberedEmail')).toBe(JSON.stringify('ada@example.test'))
})

it('does not remember the email when registration fails', async () => {
  localStorage.setItem('rememberedEmail', JSON.stringify('someone.else@example.test'))
  server.use(
    http.post('*/api/v1/user/register-user', () =>
      HttpResponse.json({ success: false, message: 'nope', code: 400, errors: {} }, { status: 400 }),
    ),
  )
  const user = userEvent.setup()
  renderPage()
  await user.type(screen.getByLabelText('Name'), 'Ada')
  await user.type(screen.getByLabelText('Email'), 'ada@example.test')
  await user.type(screen.getByLabelText('Password'), 'secret12')
  await user.type(screen.getByLabelText('Confirm password'), 'secret12')
  await user.click(screen.getByRole('button', { name: /sign up/i }))
  expect(await screen.findByText(/registration failed/i)).toBeInTheDocument()
  // The prior value is untouched — a failed registration must not overwrite it.
  expect(localStorage.getItem('rememberedEmail')).toBe(JSON.stringify('someone.else@example.test'))
})

it('rejects mismatched password retry', async () => {
  const user = userEvent.setup()
  renderPage()
  await user.type(screen.getByLabelText('Password'), 'secret12')
  await user.type(screen.getByLabelText('Confirm password'), 'different')
  await user.click(screen.getByRole('button', { name: /sign up/i }))
  expect(await screen.findByText('Passwords do not match')).toBeInTheDocument()
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

it('submits with the custom-server section collapsed and custom API allowed', async () => {
  // Regression: register('host') runs even while the section is collapsed, so
  // its rules must not block the submit when selfHosted is off — this used to
  // silently swallow every registration on a fresh self-hosted browser.
  window.econumoConfig = { ALLOW_CUSTOM_API: 'true' }
  server.use(
    http.post('*/api/v1/user/register-user', () =>
      HttpResponse.json({ success: true, message: '', data: { user: { id: 'u1', name: 'Ada', avatar: '' } } }),
    ),
  )
  const user = userEvent.setup()
  renderPage()
  expect(screen.queryByLabelText('Server address')).not.toBeInTheDocument()
  await user.type(screen.getByLabelText('Name'), 'Ada')
  await user.type(screen.getByLabelText('Email'), 'ada@example.test')
  await user.type(screen.getByLabelText('Password'), 'secret12')
  await user.type(screen.getByLabelText('Confirm password'), 'secret12')
  await user.click(screen.getByRole('button', { name: /sign up/i }))
  expect(await screen.findByText('LOGIN PAGE')).toBeInTheDocument()
})

it("shows the server's reason when registration fails with one", async () => {
  server.use(
    http.post('*/api/v1/user/register-user', () =>
      HttpResponse.json({ success: false, message: 'User already exists', code: 400, errors: {} }, { status: 400 }),
    ),
  )
  const user = userEvent.setup()
  renderPage()
  await user.type(screen.getByLabelText('Name'), 'Ada')
  await user.type(screen.getByLabelText('Email'), 'taken@example.test')
  await user.type(screen.getByLabelText('Password'), 'secret12')
  await user.type(screen.getByLabelText('Confirm password'), 'secret12')
  await user.click(screen.getByRole('button', { name: /sign up/i }))
  expect(await screen.findByText('User already exists')).toBeInTheDocument()
})

it('redirects to login when registration is disabled', async () => {
  window.econumoConfig = { ALLOW_REGISTRATION: 'false' }
  renderPage()
  expect(await screen.findByText('LOGIN PAGE')).toBeInTheDocument()
})

it('signs up through the providers only when password sign-in is disabled', async () => {
  window.econumoConfig = { PASSWORD_LOGIN: false, ALLOW_REGISTRATION: true, ALLOW_CUSTOM_API: 'false' }
  server.use(
    http.get('*/api/v1/oauth/get-provider-list', () =>
      HttpResponse.json({ success: true, message: '', data: [{ id: 'oidc', name: 'Authentik' }] }),
    ),
  )
  renderPage()
  expect(await screen.findByRole('button', { name: 'Continue with Authentik' })).toBeInTheDocument()
  expect(screen.queryByLabelText('Email')).not.toBeInTheDocument()
  expect(screen.queryByLabelText('Password')).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: /sign up/i })).not.toBeInTheDocument()
})

it('leaves for the login page when the app learns registration is off', async () => {
  window.Capacitor = { isNativePlatform: () => true }
  window.econumoConfig = { ALLOW_REGISTRATION: true }
  useServerConfig.setState({ configHost: null })
  localStorage.setItem('selfHosted', 'true')
  localStorage.setItem('backendHost', JSON.stringify('https://closed.example.test'))
  server.use(
    http.get('https://closed.example.test/econumo-config.js', () =>
      new HttpResponse('window.econumoConfig = {"ALLOW_REGISTRATION":false};\n', { headers: { 'Content-Type': 'text/javascript' } }),
    ),
  )
  try {
    renderPage()
    expect(await screen.findByText('LOGIN PAGE')).toBeInTheDocument()
  } finally {
    delete (window as { Capacitor?: unknown }).Capacitor
  }
})
