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
const openAppUrl = vi.hoisted(() => vi.fn())
vi.mock('@/lib/platform', () => ({ isIOS: () => mockIsIOS.value, isNativeApp: () => false, openAppUrl }))
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
      <AppleWalletSetup setup={setup} />
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

const stepToggle = (title: string) => screen.getAllByRole('button', { name: title }).find((button) => button.hasAttribute('aria-expanded')) as HTMLElement
const step = (title: string) => stepToggle(title).closest('li') as HTMLElement
const connectButton = () => screen.getAllByRole('button', { name: 'Connect the Shortcut' }).find((button) => !button.hasAttribute('aria-expanded')) as HTMLElement
const stateOf = (title: string) => step(title).getAttribute('data-state')
const ticks = () => JSON.parse(localStorage.getItem('appleWalletSetup:s1') ?? '[]')

beforeEach(() => {
  localStorage.clear()
  window.econumoConfig = {}
  mockIsIOS.value = false
  openAppUrl.mockClear()
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  vi.mocked(toast.error).mockClear()
})

it('setupDeepLink encodes the JSON input for the Setup shortcut', () => {
  expect(setupDeepLink('https://eco.example', 'eco_pat_x')).toBe(
    'shortcuts://run-shortcut?name=econumo&input=text&text=' + encodeURIComponent('{"url":"https://eco.example","token":"eco_pat_x"}'),
  )
})

it('before connecting it explains what is needed and how payments flow; connect posts create-source', async () => {
  let body: unknown
  server.use(http.post('*/api/v1/import/create-source', async ({ request }) => {
    body = await request.json()
    return HttpResponse.json({ success: true, message: '', data: { item: source } })
  }))
  const user = userEvent.setup()
  renderSetup(null)
  expect(screen.getByText('An iPhone with Apple Pay')).toBeInTheDocument()
  const limitations = screen.getByRole('heading', { name: 'Known limitations' }).parentElement as HTMLElement
  const limitationsItems = within(limitations).getAllByRole('listitem')
  expect(limitationsItems).toHaveLength(2)
  expect(limitationsItems[0]).toHaveTextContent('Works only when you tap your iPhone to pay')
  expect(limitationsItems[1]).toHaveTextContent('Apple Pay payments on websites aren’t supported')
  // no videos are configured yet, so the link stays hidden
  expect(screen.queryByText(/Watch the setup video/)).not.toBeInTheDocument()
  const preview = screen.getByRole('heading', { name: 'How it works' }).parentElement as HTMLElement
  const flow = within(preview).getAllByRole('listitem')
  expect(flow).toHaveLength(4)
  expect(flow[0]).toHaveTextContent('Pay with Apple Pay')
  expect(flow[1]).toHaveTextContent('Integration sends details directly to Econumo')
  expect(flow[2]).toHaveTextContent('Econumo receives the card name, merchant, amount and currency')
  expect(flow[3]).toHaveTextContent('Econumo saves the transaction')
  await user.click(screen.getByRole('button', { name: 'Set up Apple Wallet' }))
  await waitFor(() => expect(body).toEqual({ provider: 'apple-wallet', name: 'iPhone' }))
})

// Both platforms offer the published iCloud link and the downloadable file.
it.each([['desktop', false], ['iOS', true]] as const)('%s install opens the iCloud link, with the file as a fallback', (_platform, ios) => {
  mockIsIOS.value = ios
  renderSetup(source)
  const link = screen.getByRole('link', { name: 'econumo' })
  expect(link).toHaveAttribute('href', 'https://www.icloud.com/shortcuts/9a41df014a97408aaf4092a5bc69d228')
  expect(link).toHaveAttribute('target', '_blank')
  expect(link).not.toHaveAttribute('download')
  const file = screen.getByRole('link', { name: 'or download' })
  expect(file).toHaveAttribute('href', '/shortcuts/econumo.shortcut')
  expect(file).toHaveAttribute('target', '_blank')
})

it('opens on the first step only; its install link opens in a new window and Done moves on', async () => {
  const user = userEvent.setup()
  renderSetup(source)
  expect(stateOf('Install the Shortcut')).toBe('current')
  expect(screen.getByRole('button', { name: 'Install the Shortcut' })).toHaveAttribute('aria-expanded', 'true')
  expect(screen.getByRole('button', { name: 'Create the automation' })).toHaveAttribute('aria-expanded', 'false')
  const link = screen.getByRole('link', { name: 'econumo' })
  expect(link).toHaveAttribute('href', 'https://www.icloud.com/shortcuts/9a41df014a97408aaf4092a5bc69d228')
  expect(link).toHaveAttribute('target', '_blank')
  await user.click(screen.getByRole('button', { name: 'Done' }))
  expect(ticks()).toEqual(['install'])
  expect(stateOf('Install the Shortcut')).toBe('done')
  expect(stateOf('Connect the Shortcut')).toBe('current')
  expect(stepToggle('Connect the Shortcut')).toHaveAttribute('aria-expanded', 'true')
})

it('desktop shows Connect the Shortcut alongside the iPhone hint', () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  renderSetup(source)
  expect(screen.getByText(/Open this page on your iPhone/)).toBeInTheDocument()
  expect(within(step('Connect the Shortcut')).queryByRole('link', { name: 'Configure manually' })).not.toBeInTheDocument()
  expect(connectButton()).toBeInTheDocument()
})

it('ticks from the first guide carry over: both install steps make the merged one done, the run-once step the connect one', () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install_wallet', 'install_setup', 'test', 'automate']))
  renderSetup(source)
  expect(stateOf('Install the Shortcut')).toBe('done')
  expect(stateOf('Connect the Shortcut')).toBe('done')
  expect(stateOf('Create the automation')).toBe('done')
})

it.each([['desktop', false], ['iOS', true]] as const)('%s connect mints an ingest token, then offers the shortcut\'s setup link in a new window', async (_platform, ios) => {
  mockIsIOS.value = ios
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
  expect(screen.queryByText(/Open this page on your iPhone/) !== null).toBe(!ios)
  await user.click(connectButton())
  const link = await screen.findByRole('link', { name: 'Link Econumo' })
  expect(body).toEqual({ name: 'Apple Wallet', scope: 'ingest', expiresAt: '' })
  expect(link).toHaveAttribute('target', '_blank')
  const href = link.getAttribute('href') ?? ''
  expect(href.startsWith('shortcuts://run-shortcut?name=econumo&input=text&text=')).toBe(true)
  expect(decodeURIComponent(href.split('text=')[1])).toContain('"token":"eco_pat_new"')
  // one tap: the same link goes to Shortcuts right away; the button is the fallback
  expect(openAppUrl).toHaveBeenCalledTimes(1)
  expect(openAppUrl).toHaveBeenCalledWith(href)
  // a minted token proves nothing yet: the step waits for the shortcut's check-in
  link.addEventListener('click', (e) => e.preventDefault())
  await user.click(link)
  expect(stateOf('Connect the Shortcut')).toBe('current')
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
  await user.click(connectButton())
  await waitFor(() => expect(calls).toContain('create'))
  expect(revoked).toEqual(['p1'])
  expect(calls).toEqual(['revoke', 'create'])
})

// Connecting again is how a new iPhone, a revoked token or a reinstalled
// Shortcut gets set up, so the button stays once the step is done.
it.each([['desktop', false], ['iOS', true]] as const)('%s connect step keeps its button after it is done, warning that a new token replaces the old one', async (_platform, ios) => {
  mockIsIOS.value = ios
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install', 'configure']))
  server.use(http.post('*/api/v1/user/create-personal-token', () => HttpResponse.json({ success: true, message: '', data: minted })))
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(stepToggle('Connect the Shortcut'))
  expect(stateOf('Connect the Shortcut')).toBe('done')
  expect(screen.getByText(/Connecting again replaces the token the Shortcut uses now/)).toBeInTheDocument()
  await user.click(connectButton())
  expect(await screen.findByRole('link', { name: 'Link Econumo' })).toBeInTheDocument()
  expect(screen.queryByText(/Connecting again replaces/)).not.toBeInTheDocument()
  expect(stateOf('Connect the Shortcut')).toBe('done')
})

it('a double tap on Connect the Shortcut mints once', async () => {
  mockIsIOS.value = true
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  let creates = 0
  server.use(http.post('*/api/v1/user/create-personal-token', () => {
    creates++
    return HttpResponse.json({ success: true, message: '', data: minted })
  }))
  const user = userEvent.setup()
  renderSetup(source)
  const btn = connectButton()
  const first = user.click(btn)
  await user.click(btn)
  await first
  await screen.findByRole('link', { name: 'Link Econumo' })
  expect(creates).toBe(1)
})

it('iOS connect toasts the server error and leaves the step open when minting fails', async () => {
  mockIsIOS.value = true
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  server.use(http.post('*/api/v1/user/create-personal-token', () =>
    HttpResponse.json({ success: false, message: 'Too many attempts. Try again later.', code: 429, errors: {} }, { status: 429 })))
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(connectButton())
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Too many attempts. Try again later.'))
  expect(stateOf('Connect the Shortcut')).toBe('current')
  expect(screen.queryByRole('link', { name: 'Link Econumo' })).not.toBeInTheDocument()
})

it('while the connect step is open the page watches for the shortcut: the "account is required" check-in is proof, discarded, and the step completes', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  let discarded: unknown
  server.use(http.post('*/api/v1/import/discard-event', async ({ request }) => {
    discarded = await request.json()
    return HttpResponse.json({ success: true, message: '', data: emptyQueue })
  }))
  renderSetup(source, { importQueue: { queued: [], skipped: [], failed: [noInputEvent] } })
  await waitFor(() => expect(stateOf('Connect the Shortcut')).toBe('done'))
  expect(discarded).toEqual({ eventId: 'e1' })
  expect(ticks()).toEqual(['install', 'configure'])
  expect(stateOf('Create the automation')).toBe('current')
})

it('Configure again clears manual progress and restarts the whole checklist', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  const user = userEvent.setup()
  renderSetup(source)
  expect(await screen.findByRole('status')).toHaveTextContent('Waiting for the Shortcut to reach Econumo')
  await user.click(screen.getByRole('button', { name: 'Done' }))
  expect(stateOf('Connect the Shortcut')).toBe('done')
  await user.click(screen.getByRole('button', { name: 'Configure again' }))
  await waitFor(() => expect(stateOf('Install the Shortcut')).toBe('current'))
  expect(stepToggle('Install the Shortcut')).toHaveAttribute('aria-expanded', 'true')
  expect(stateOf('Connect the Shortcut')).toBe('todo')
  expect(ticks()).toEqual([])
  expect(screen.queryByRole('button', { name: 'Mark as not done' })).not.toBeInTheDocument()
})

it('Configure again clears the temporary setup link without minting or revoking tokens', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  let creates = 0
  let revokes = 0
  server.use(
    http.post('*/api/v1/user/create-personal-token', () => {
      creates++
      return HttpResponse.json({ success: true, message: '', data: minted })
    }),
    http.post('*/api/v1/user/revoke-personal-token', () => {
      revokes++
      return HttpResponse.json({ success: true, message: '', data: {} })
    }),
  )
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(connectButton())
  expect(await screen.findByRole('link', { name: 'Link Econumo' })).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Configure again' }))
  await waitFor(() => expect(stateOf('Install the Shortcut')).toBe('current'))
  await user.click(stepToggle('Connect the Shortcut'))
  expect(screen.queryByRole('link', { name: 'Link Econumo' })).not.toBeInTheDocument()
  expect(connectButton()).toBeInTheDocument()
  expect(creates).toBe(1)
  expect(revokes).toBe(0)
})

it('Configure again preserves progress if refreshing the connection fails', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install', 'configure']))
  const user = userEvent.setup()
  renderSetup(source)
  server.use(http.get('*/api/v1/import/get-source-list', () => HttpResponse.json({ success: false, message: 'Connection unavailable', errors: {} }, { status: 500 })))
  await user.click(screen.getByRole('button', { name: 'Configure again' }))
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Connection unavailable'))
  expect(stateOf('Install the Shortcut')).toBe('done')
  expect(stateOf('Connect the Shortcut')).toBe('done')
  expect(stateOf('Create the automation')).toBe('current')
  expect(ticks()).toEqual(['install', 'configure'])
})

it('ignores an old manual payment tick and keeps waiting when only failed events have arrived', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install', 'configure', 'automate', 'payment']))
  renderSetup(source, { importQueue: { ...emptyQueue, failed: [noInputEvent, { ...noInputEvent, eventId: 'e2', error: 'amount must be a positive number' }] } })
  expect(stateOf('Make the first payment')).toBe('current')
  expect(await screen.findByRole('status')).toHaveTextContent('Waiting for the first payment')
  const payment = within(step('Make the first payment'))
  expect(payment.queryByRole('button', { name: 'Done' })).not.toBeInTheDocument()
  expect(payment.queryByRole('button', { name: 'Mark as not done' })).not.toBeInTheDocument()
})

it('a mapped card without a received payment does not complete step 4', () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install', 'configure', 'automate']))
  renderSetup({ ...source, cards: [{ ...card, state: 'mapped', accountId: 'a1', tapCount: 0, lastSeenAt: '' }] })
  expect(stateOf('Make the first payment')).toBe('current')
  expect(screen.getByRole('status')).toHaveTextContent('Waiting for the first payment')
  expect(screen.queryByRole('button', { name: 'Done' })).not.toBeInTheDocument()
})

it.each([false, true])('step 5 requires the first payment (previously marked done: %s)', async (previouslyDone) => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install', 'configure', 'automate', ...(previouslyDone ? ['immediate'] : [])]))
  const user = userEvent.setup()
  renderSetup(source, { importQueue: { ...emptyQueue, failed: [noInputEvent] } })
  expect(stateOf('Make the first payment')).toBe('current')
  expect(stateOf('Switch the automation to Run Immediately')).toBe('todo')
  await user.click(stepToggle('Switch the automation to Run Immediately'))
  const done = within(step('Switch the automation to Run Immediately')).getByRole('button', { name: 'Done' })
  expect(done).toBeDisabled()
  await user.click(done)
  expect(stateOf('Switch the automation to Run Immediately')).toBe('todo')
  expect(screen.queryByRole('button', { name: 'Mark as not done' })).not.toBeInTheDocument()
  expect(ticks()).not.toContain('immediate')
})

it.each(['mapped', 'unmapped', 'ignored'] as const)('the first payment on a %s card completes steps 1–4; Run Immediately stays a hand tick', async (state) => {
  const user = userEvent.setup()
  renderSetup({ ...source, cards: [{ ...card, state, accountId: state === 'mapped' ? 'a1' : '', queuedCount: state === 'unmapped' ? 1 : 0 }] })
  expect(stateOf('Make the first payment')).toBe('done')
  expect(stateOf('Switch the automation to Run Immediately')).toBe('current')
  await user.click(stepToggle('Make the first payment'))
  expect(within(step('Make the first payment')).queryByRole('button', { name: 'Mark as not done' })).not.toBeInTheDocument()
  // server-confirmed steps cannot be undone by hand
  await user.click(screen.getByRole('button', { name: 'Install the Shortcut' }))
  expect(screen.queryByRole('button', { name: 'Mark as not done' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Switch the automation to Run Immediately' }))
  await user.click(screen.getByRole('button', { name: 'Done' }))
  expect(stateOf('Switch the automation to Run Immediately')).toBe('done')
  expect(screen.getByText(/Open this page on your iPhone/)).toBeVisible()
  expect(screen.getByText(/Open this page on your iPhone/).nextElementSibling).toContainElement(screen.getByRole('link', { name: 'Setup instructions' }))
  expect(screen.getByRole('button', { name: 'Configure again' })).toBeInTheDocument()
})

it.each([false, true])('completed setup starts collapsed and can be reopened (iOS: %s)', async (ios) => {
  mockIsIOS.value = ios
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['immediate']))
  const user = userEvent.setup()
  renderSetup({ ...source, cards: [card] })
  const toggle = screen.getByRole('button', { name: 'Setup guide' })
  expect(screen.getByRole('heading', { name: 'Setup guide', level: 2 })).toContainElement(toggle)
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByRole('link', { name: 'Setup instructions' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Install the Shortcut' })).not.toBeInTheDocument()

  await user.click(toggle)
  expect(toggle).toHaveAttribute('aria-expanded', 'true')
  expect(screen.getByRole('link', { name: 'Setup instructions' })).toBeVisible()
  expect(stateOf('Switch the automation to Run Immediately')).toBe('done')
  expect(screen.queryByText(/Open this page on your iPhone/) !== null).toBe(!ios)

  await user.click(toggle)
  expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await user.click(toggle)
  await user.click(screen.getByRole('button', { name: 'Configure again' }))
  await waitFor(() => expect(stateOf('Install the Shortcut')).toBe('current'))
  expect(stepToggle('Install the Shortcut')).toHaveAttribute('aria-expanded', 'true')
  expect(screen.queryByRole('button', { name: 'Setup guide' })).not.toBeInTheDocument()
  expect(screen.getByRole('heading', { name: 'Setup guide', level: 2 })).toBeVisible()
})

it('disconnect drops the hand ticks', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install']))
  localStorage.setItem('appleWalletSetupReset:s1', JSON.stringify({ tapCount: 0, failedEventIds: [], queuedLinkIds: [] }))
  server.use(http.post('*/api/v1/import/delete-source', () => HttpResponse.json({ success: true, message: '', data: {} })))
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(screen.getByRole('button', { name: 'Disconnect Apple Wallet' }))
  await user.click(await screen.findByRole('button', { name: 'Disconnect' }))
  await waitFor(() => expect(localStorage.getItem('appleWalletSetup:s1')).toBeNull())
  expect(localStorage.getItem('appleWalletSetupReset:s1')).toBeNull()
})
