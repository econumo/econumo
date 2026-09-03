import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { toast } from 'sonner'
import { server } from '@/test/msw'
import { AppleWalletSetup, nav, setupDeepLink } from './AppleWalletSetup'

const mockIsIOS = vi.hoisted(() => ({ value: false }))
vi.mock('@/lib/platform', () => ({ isIOS: () => mockIsIOS.value, isNativeApp: () => false }))
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const source = { id: 's1', provider: 'apple-wallet' as const, name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00', cards: [] }

function renderSetup(src: typeof source | null) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <AppleWalletSetup source={src} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockIsIOS.value = false
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  vi.mocked(toast.error).mockClear()
})

it('setupDeepLink encodes the JSON input for the Setup shortcut', () => {
  expect(setupDeepLink('https://eco.example', 'eco_pat_x')).toBe(
    'shortcuts://run-shortcut?name=econumo-setup-v1&input=text&text=' + encodeURIComponent('{"url":"https://eco.example","token":"eco_pat_x"}'),
  )
})

it('connect posts create-source', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/import/create-source', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { item: source } })
  }))
  const user = userEvent.setup()
  renderSetup(null)
  await user.click(screen.getByRole('button', { name: 'Set up Apple Wallet' }))
  await waitFor(() => expect(body).toEqual({ provider: 'apple-wallet', name: 'iPhone' }))
})

it('desktop shows the iPhone hint instead of the configure button', () => {
  renderSetup(source)
  expect(screen.getByText('Open Settings → Import & export on your iPhone to configure the Shortcut there.')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Configure on this iPhone' })).not.toBeInTheDocument()
})

it('iOS configure mints an ingest token and opens the shortcuts deep link', async () => {
  mockIsIOS.value = true
  let body: unknown
  server.use(http.post('*/api/v1/user/create-personal-token', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { id: 'p1', name: 'Apple Wallet', token: 'eco_pat_new', createdAt: '2026-08-01 00:00:00', expiresAt: null } })
  }))
  const assigned: string[] = []
  const spy = vi.spyOn(nav, 'openDeepLink').mockImplementation((url) => assigned.push(url))
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(screen.getByRole('button', { name: 'Configure on this iPhone' }))
  await waitFor(() => expect(assigned).toHaveLength(1))
  expect(body).toEqual({ name: 'Apple Wallet', scope: 'ingest', expiresAt: '' })
  expect(assigned[0].startsWith('shortcuts://run-shortcut?name=econumo-setup-v1&input=text&text=')).toBe(true)
  expect(decodeURIComponent(assigned[0].split('text=')[1])).toContain('"token":"eco_pat_new"')
  spy.mockRestore()
})

it('manual recipe reveals the token once, with the server URL and the request body', async () => {
  server.use(http.post('*/api/v1/user/create-personal-token', () =>
    HttpResponse.json({ success: true, message: '', data: { id: 'p1', name: 'Apple Wallet', token: 'eco_pat_manual', createdAt: '2026-08-01 00:00:00', expiresAt: null } })))
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(screen.getByText('Configure manually'))
  expect(await screen.findByText('eco_pat_manual')).toBeInTheDocument()
  expect(screen.getByText(/ingest-apple-wallet-event/)).toBeInTheDocument()
  expect(screen.getByText(/"occurredAt"/)).toBeInTheDocument()
})

it('iOS configure toasts the server error and does not mark itself configured when minting fails', async () => {
  mockIsIOS.value = true
  server.use(http.post('*/api/v1/user/create-personal-token', () =>
    HttpResponse.json({ success: false, message: 'Too many attempts. Try again later.', code: 429, errors: {} }, { status: 429 })))
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(screen.getByRole('button', { name: 'Configure on this iPhone' }))
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Too many attempts. Try again later.'))
  expect(screen.queryByText('The Shortcut is configured. Your next Apple Pay payment will show up here.')).not.toBeInTheDocument()
})

it('manual recipe toasts the server error and leaves the panel closed when minting fails', async () => {
  server.use(http.post('*/api/v1/user/create-personal-token', () =>
    HttpResponse.json({ success: false, message: 'Too many attempts. Try again later.', code: 429, errors: {} }, { status: 429 })))
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(screen.getByText('Configure manually'))
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Too many attempts. Try again later.'))
  expect(screen.queryByText('Access token (shown once)')).not.toBeInTheDocument()
})
