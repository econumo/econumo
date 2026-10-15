import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { toast } from 'sonner'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import type { ImportSourceDto } from '@/api/dto/imports'
import { AppleWalletConnect, AppleWalletDisconnect, AppleWalletSetup, setupDeepLink } from './AppleWalletSetup'
import { useWalletSetup } from './walletSetup'

const mockIsIOS = vi.hoisted(() => ({ value: false }))
vi.mock('@/lib/platform', () => ({ isIOS: () => mockIsIOS.value, isNativeApp: () => false }))
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const card = { externalAccountId: 'Apple Card', externalName: 'Apple Card', externalCurrency: 'USD', state: 'unmapped' as const, accountId: '', queuedCount: 0, tapCount: 1, lastSeenAt: '2026-08-20 10:42:03' }
const source: ImportSourceDto = { id: 's1', provider: 'apple-wallet', name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00', lastSyncedAt: '', lastRunStatus: '', lastRunAt: '', lastRunError: '', lastRunErrorAccountId: '', credentialCiphertext: '', cards: [] }
const ingestToken = { id: 'p1', name: 'Apple Wallet', scope: 'ingest', createdAt: '2026-08-01 00:00:00', expiresAt: null, lastUsedAt: null }
const emptyQueue = { queued: [], skipped: [], failed: [] }
const noInputEvent = { eventId: 'e1', sourceId: 's1', receivedAt: '2026-08-21 08:00:00', error: 'account is required', payload: '{}' }
const minted = { id: 'p3', name: 'Apple Wallet', token: 'eco_pat_new', createdAt: '2026-08-01 00:00:00', expiresAt: null }

function Harness({ src }: { src: ImportSourceDto }) {
  const setup = useWalletSetup(src)
  return (
    <>
      <AppleWalletSetup setup={setup} collapsible={src.cards.length > 0} />
      <AppleWalletDisconnect source={src} />
    </>
  )
}

function renderSetup(src: ImportSourceDto | null, overrides: Record<string, unknown> = {}) {
  server.use(...coreHandlers({ importSources: src ? [src] : [], importQueue: emptyQueue, ...overrides }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      {src ? <Harness src={src} /> : <AppleWalletConnect />}
    </QueryClientProvider>,
  )
}

const step = (title: string) => screen.getByRole('button', { name: title }).closest('li') as HTMLElement
const stateOf = (title: string) => step(title).getAttribute('data-state')
const ticks = () => JSON.parse(localStorage.getItem('appleWalletSetup:s1') ?? '[]')

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
    'shortcuts://run-shortcut?name=econumo-apple-wallet&input=text&text=' + encodeURIComponent('{"url":"https://eco.example","token":"eco_pat_x"}'),
  )
})

it('before connecting it explains what is needed and previews the steps; connect posts create-source', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/import/create-source', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { item: source } })
  }))
  const user = userEvent.setup()
  renderSetup(null)
  expect(screen.getByText('An iPhone with Apple Pay')).toBeInTheDocument()
  expect(screen.getByText(/never card numbers/)).toBeInTheDocument()
  // no videos are configured yet, so the link stays hidden
  expect(screen.queryByText(/Watch the setup video/)).not.toBeInTheDocument()
  const preview = screen.getByRole('heading', { name: 'How it works' }).parentElement as HTMLElement
  expect(within(preview).getAllByRole('listitem')).toHaveLength(5)
  await user.click(screen.getByRole('button', { name: 'Set up Apple Wallet' }))
  await waitFor(() => expect(body).toEqual({ provider: 'apple-wallet', name: 'iPhone' }))
})

it('opens on the first step only; its download opens in a new window and Done moves on', async () => {
  const user = userEvent.setup()
  renderSetup(source)
  expect(stateOf('Install the Shortcut')).toBe('current')
  expect(screen.getByRole('button', { name: 'Install the Shortcut' })).toHaveAttribute('aria-expanded', 'true')
  expect(screen.getByRole('button', { name: 'Create the automation' })).toHaveAttribute('aria-expanded', 'false')
  const link = screen.getByRole('link', { name: 'econumo-apple-wallet' })
  expect(link).toHaveAttribute('href', '/shortcuts/econumo-apple-wallet.shortcut')
  expect(link).toHaveAttribute('target', '_blank')
  await user.click(screen.getByRole('button', { name: 'Done' }))
  expect(ticks()).toEqual(['install'])
  expect(stateOf('Install the Shortcut')).toBe('done')
  expect(stateOf('Connect the Shortcut to Econumo')).toBe('current')
  expect(screen.getByRole('button', { name: 'Connect the Shortcut to Econumo' })).toHaveAttribute('aria-expanded', 'true')
})

it('desktop shows the iPhone hint and the manual guide in a new window, but no iOS-only actions', () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  renderSetup(source)
  expect(screen.getByText(/Open this page on your iPhone/)).toBeInTheDocument()
  const guide = screen.getByRole('link', { name: 'Configure manually' })
  expect(guide).toHaveAttribute('href', 'https://econumo.com/docs/user-guide/apple-wallet')
  expect(guide).toHaveAttribute('target', '_blank')
  expect(screen.queryByRole('button', { name: 'Create access token' })).not.toBeInTheDocument()
})

it('ticks from the first guide carry over: both install steps make the merged one done, the run-once step the connect one', () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install_wallet', 'install_setup', 'test', 'automate']))
  renderSetup(source)
  expect(stateOf('Install the Shortcut')).toBe('done')
  expect(stateOf('Connect the Shortcut to Econumo')).toBe('done')
  expect(stateOf('Create the automation')).toBe('done')
})

it('iOS connect mints an ingest token, then offers the shortcut\'s setup link in a new window', async () => {
  mockIsIOS.value = true
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  let body: unknown
  const user = userEvent.setup()
  renderSetup(source)
  // registered after the defaults so they win; the server holds the token
  // once this flow has minted it
  server.use(
    http.post('*/api/v1/user/create-personal-token', async ({ request }) => {
      body = await request.json()
      return HttpResponse.json({ success: true, message: '', data: minted })
    }),
    http.get('*/api/v1/user/get-personal-token-list', () => HttpResponse.json({ success: true, message: '', data: body ? [ingestToken] : [] })),
  )
  expect(screen.queryByText(/Open this page on your iPhone/)).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Create access token' }))
  const link = await screen.findByRole('link', { name: 'Open econumo-apple-wallet' })
  expect(body).toEqual({ name: 'Apple Wallet', scope: 'ingest', expiresAt: '' })
  expect(link).toHaveAttribute('target', '_blank')
  const href = link.getAttribute('href') ?? ''
  expect(href.startsWith('shortcuts://run-shortcut?name=econumo-apple-wallet&input=text&text=')).toBe(true)
  expect(decodeURIComponent(href.split('text=')[1])).toContain('"token":"eco_pat_new"')
  // a minted token proves nothing yet: the step waits for the shortcut's check-in
  link.addEventListener('click', (e) => e.preventDefault())
  await user.click(link)
  expect(stateOf('Connect the Shortcut to Econumo')).toBe('current')
  expect(screen.getByRole('status')).toHaveTextContent('Waiting for the Shortcut to reach Econumo')
})

it('iOS connect revokes the ingest tokens the server holds at tap time before minting, leaving full tokens alone', async () => {
  mockIsIOS.value = true
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  const fullToken = { ...ingestToken, id: 'p2', name: 'CLI', scope: 'full' }
  const revoked: string[] = []
  const calls: string[] = []
  server.use(http.post('*/api/v1/user/create-personal-token', () => {
    calls.push('create')
    return HttpResponse.json({ success: true, message: '', data: minted })
  }))
  server.use(http.post('*/api/v1/user/revoke-personal-token', async ({ request }) => {
    calls.push('revoke')
    revoked.push(((await request.json()) as { id: string }).id)
    return HttpResponse.json({ success: true, message: '', data: {} })
  }))
  const user = userEvent.setup()
  renderSetup(source)
  // The list was empty when the page rendered (so the button shows); by the
  // time the user taps, another device has minted an ingest token.
  let listCalls = 0
  server.use(http.get('*/api/v1/user/get-personal-token-list', () => {
    listCalls++
    return HttpResponse.json({ success: true, message: '', data: listCalls === 1 ? [fullToken] : [ingestToken, fullToken] })
  }))
  await user.click(screen.getByRole('button', { name: 'Create access token' }))
  await waitFor(() => expect(calls).toContain('create'))
  expect(revoked).toEqual(['p1'])
  expect(calls).toEqual(['revoke', 'create'])
})

it('a double tap on Create access token mints once', async () => {
  mockIsIOS.value = true
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  let creates = 0
  server.use(http.post('*/api/v1/user/create-personal-token', () => {
    creates++
    return HttpResponse.json({ success: true, message: '', data: minted })
  }))
  const user = userEvent.setup()
  renderSetup(source)
  const btn = screen.getByRole('button', { name: 'Create access token' })
  const first = user.click(btn)
  await user.click(btn)
  await first
  await screen.findByRole('link', { name: 'Open econumo-apple-wallet' })
  expect(creates).toBe(1)
})

it('iOS connect toasts the server error and leaves the step open when minting fails', async () => {
  mockIsIOS.value = true
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  server.use(http.post('*/api/v1/user/create-personal-token', () =>
    HttpResponse.json({ success: false, message: 'Too many attempts. Try again later.', code: 429, errors: {} }, { status: 429 })))
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(screen.getByRole('button', { name: 'Create access token' }))
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Too many attempts. Try again later.'))
  expect(stateOf('Connect the Shortcut to Econumo')).toBe('current')
  expect(screen.queryByRole('link', { name: 'Open econumo-apple-wallet' })).not.toBeInTheDocument()
})

it('while the connect step is open the page watches for the shortcut: the "account is required" check-in is proof, discarded, and the step completes', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  let discarded: unknown
  server.use(http.post('*/api/v1/import/discard-event', async ({ request }) => {
    discarded = await request.json()
    return HttpResponse.json({ success: true, message: '', data: emptyQueue })
  }))
  renderSetup(source, { importQueue: { queued: [], skipped: [], failed: [noInputEvent] } })
  await waitFor(() => expect(stateOf('Connect the Shortcut to Econumo')).toBe('done'))
  expect(discarded).toEqual({ eventId: 'e1' })
  expect(ticks()).toEqual(['install', 'configure'])
  expect(stateOf('Create the automation')).toBe('current')
})

it('with nothing received the connect step keeps waiting and can be marked done by hand, then undone', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  const user = userEvent.setup()
  renderSetup(source)
  expect(await screen.findByRole('status')).toHaveTextContent('Waiting for the Shortcut to reach Econumo')
  await user.click(screen.getByRole('button', { name: 'Mark as done' }))
  expect(stateOf('Connect the Shortcut to Econumo')).toBe('done')
  await user.click(screen.getByRole('button', { name: 'Connect the Shortcut to Econumo' }))
  await user.click(screen.getByRole('button', { name: 'Mark as not done' }))
  expect(stateOf('Connect the Shortcut to Econumo')).toBe('current')
})

it('the first card completes every step up to the payment and folds the guide; Run Immediately stays a hand tick', async () => {
  const user = userEvent.setup()
  renderSetup({ ...source, cards: [card] })
  const toggle = screen.getByRole('button', { name: /Setup guide/ })
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
  expect(toggle).toHaveTextContent('4 of 5 done')
  await user.click(toggle)
  expect(stateOf('Make the first payment')).toBe('done')
  expect(stateOf('Switch the automation to Run Immediately')).toBe('current')
  // server-confirmed steps cannot be undone by hand
  await user.click(screen.getByRole('button', { name: 'Install the Shortcut' }))
  expect(screen.queryByRole('button', { name: 'Mark as not done' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Switch the automation to Run Immediately' }))
  await user.click(screen.getByRole('button', { name: 'Done' }))
  expect(toggle).toHaveTextContent('Setup complete')
})

it('disconnect drops the hand ticks', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  server.use(http.post('*/api/v1/import/delete-source', () => HttpResponse.json({ success: true, message: '', data: {} })))
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(screen.getByRole('button', { name: 'Disconnect Apple Wallet' }))
  await user.click(await screen.findByRole('button', { name: 'Disconnect' }))
  await waitFor(() => expect(localStorage.getItem('appleWalletSetup:s1')).toBeNull())
})
