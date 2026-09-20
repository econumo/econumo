import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { useOAuthInFlight } from './oauthQueries'
import { ProviderButtons } from './ProviderButtons'

function renderButtons(intent: 'login' | 'link' = 'login') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={qc}><ProviderButtons intent={intent} /></QueryClientProvider>)
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  delete (window as { Capacitor?: unknown }).Capacitor
})

afterEach(() => {
  useOAuthInFlight.setState({ inFlight: false })
})

it('renders nothing when no provider is configured', async () => {
  server.use(http.get('*/api/v1/oauth/get-provider-list', () => HttpResponse.json({ success: true, message: '', data: [] })))
  const { container } = renderButtons()
  await new Promise((r) => setTimeout(r, 20))
  expect(container.querySelector('[data-testid="provider-buttons"]')).toBeNull()
})

it('renders one button per provider, in order, and starts the flow on click', async () => {
  const assign = vi.fn()
  Object.defineProperty(window, 'location', { value: { ...window.location, assign }, writable: true })
  server.use(
    http.get('*/api/v1/oauth/get-provider-list', () =>
      HttpResponse.json({ success: true, message: '', data: [{ id: 'google', name: 'Google' }, { id: 'apple', name: 'Apple' }, { id: 'oidc', name: 'Authentik' }] })),
    http.post('*/api/v1/oauth/start-login', async ({ request }) => {
      const { provider } = (await request.json()) as { provider: string }
      return HttpResponse.json({ success: true, message: '', data: { url: `https://idp/${provider}` } })
    }),
  )
  renderButtons()
  const buttons = await screen.findAllByRole('button')
  expect(buttons.map((b) => b.textContent)).toEqual(['Continue with Google', 'Continue with Apple', 'Continue with Authentik'])
  expect(screen.getByText('or continue with')).toBeInTheDocument()
  await userEvent.click(buttons[2])
  await vi.waitFor(() => expect(assign).toHaveBeenCalledWith('https://idp/oidc'))
})

it('keeps the button disabled after the start mutation settles, and re-enables it once the browser sheet finishes', async () => {
  const listeners: Record<string, () => void> = {}
  const browserOpen = vi.fn().mockResolvedValue(undefined)
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: {
    open: browserOpen,
    addListener: vi.fn((ev: string, cb: () => void) => { listeners[ev] = cb }),
  } } }
  server.use(
    http.get('*/api/v1/oauth/get-provider-list', () => HttpResponse.json({ success: true, message: '', data: [{ id: 'google', name: 'Google' }] })),
    http.post('*/api/v1/oauth/start-login', () => HttpResponse.json({ success: true, message: '', data: { url: 'https://idp/google', flow: 'f1' } })),
  )
  renderButtons()
  const button = await screen.findByRole('button')
  await userEvent.click(button)
  // Wait past openAuthorizationUrl (proof the mutation's async work ran), then
  // flush react-query's own state update so `start.isPending` itself settles
  // back to false — isolating that `inFlight`, not `isPending`, is what still
  // holds the button disabled here.
  await waitFor(() => expect(browserOpen).toHaveBeenCalled())
  await act(async () => {})
  expect(button).toBeDisabled()
  listeners.browserFinished()
  await waitFor(() => expect(button).not.toBeDisabled())
})

it('hides the buttons on the web when the SPA is served from a different origin than the backend', async () => {
  window.econumoConfig = { ALLOW_CUSTOM_API: true }
  localStorage.setItem('selfHosted', JSON.stringify(true))
  localStorage.setItem('backendHost', JSON.stringify('https://money.example.org'))
  server.use(http.get('*/api/v1/oauth/get-provider-list', () => HttpResponse.json({ success: true, message: '', data: [{ id: 'google', name: 'Google' }] })))
  const { container } = renderButtons()
  await new Promise((r) => setTimeout(r, 20))
  expect(container.querySelector('[data-testid="provider-buttons"]')).toBeNull()
})

it('shows the buttons when the stored backend host is same-origin but for a trailing slash', async () => {
  window.econumoConfig = { ALLOW_CUSTOM_API: true }
  localStorage.setItem('selfHosted', JSON.stringify(true))
  localStorage.setItem('backendHost', JSON.stringify(`${window.location.origin}/`))
  server.use(http.get('*/api/v1/oauth/get-provider-list', () => HttpResponse.json({ success: true, message: '', data: [{ id: 'google', name: 'Google' }] })))
  renderButtons()
  expect(await screen.findByTestId('provider-buttons')).toBeInTheDocument()
})
