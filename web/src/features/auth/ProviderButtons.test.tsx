import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/msw'
import { ProviderButtons } from './ProviderButtons'

function renderButtons(intent: 'login' | 'link' = 'login') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(<QueryClientProvider client={qc}><ProviderButtons intent={intent} /></QueryClientProvider>)
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
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
