import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { toast } from 'sonner'
import { server } from '@/test/msw'
import { coreHandlers } from '@/test/fixtures'
import { AppleWalletSetup, nav, setupDeepLink } from './AppleWalletSetup'

const mockIsIOS = vi.hoisted(() => ({ value: false }))
vi.mock('@/lib/platform', () => ({ isIOS: () => mockIsIOS.value, isNativeApp: () => false }))
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const card = { externalAccountId: 'Apple Card', externalName: 'Apple Card', externalCurrency: 'USD', state: 'unmapped', accountId: '', queuedCount: 0, tapCount: 1, lastSeenAt: '2026-08-20 10:42:03' }
const source = { id: 's1', provider: 'apple-wallet' as const, name: 'iPhone', status: 'active', createdAt: '2026-08-01 00:00:00', cards: [] as (typeof card)[] }
const ingestToken = { id: 'p1', name: 'Apple Wallet', scope: 'ingest', createdAt: '2026-08-01 00:00:00', expiresAt: null, lastUsedAt: null }
const emptyQueue = { queued: [], skipped: [], failed: [] }
const noInputEvent = { eventId: 'e1', sourceId: 's1', receivedAt: '2026-08-21 08:00:00', error: 'account is required', payload: '{}' }

const STEP_NAMES = [
  'Install econumo-wallet-v1', 'Install econumo-setup-v1', 'Configure the Shortcuts',
  'Run econumo-wallet-v1 once', 'Create the automation', 'Make the first payment with the iPhone unlocked',
]

function renderSetup(src: typeof source | null, overrides: Record<string, unknown> = {}) {
  server.use(...coreHandlers({ importSources: src ? [src] : [], importQueue: emptyQueue, ...overrides }))
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <AppleWalletSetup source={src} />
    </QueryClientProvider>,
  )
}

const checkbox = (name: string) => screen.getByRole('checkbox', { name })
// the test step's Check comes first in the list, the payment step's second
const checkButton = (step: 'test' | 'payment') => screen.getAllByRole('button', { name: 'Check' })[step === 'test' ? 0 : 1]

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

it('shows six unticked steps; the shortcut downloads open outside the app window', () => {
  renderSetup(source)
  for (const name of STEP_NAMES) {
    expect(checkbox(name)).not.toBeChecked()
  }
  for (const name of ['econumo-wallet-v1', 'econumo-setup-v1']) {
    const link = screen.getByRole('link', { name })
    expect(link).toHaveAttribute('href', `/shortcuts/${name}.shortcut`)
    expect(link).toHaveAttribute('target', '_blank')
  }
})

it('desktop shows the iPhone hint and the manual guide link, but no iOS-only buttons', () => {
  renderSetup(source)
  expect(screen.getByText(/Open this page on your iPhone/)).toBeInTheDocument()
  const guide = screen.getByRole('link', { name: 'Configure manually' })
  expect(guide).toHaveAttribute('href', 'https://econumo.com/docs/user-guide/apple-wallet')
  expect(guide).toHaveAttribute('target', '_blank')
  expect(screen.queryByRole('button', { name: 'Configure on this iPhone' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Run econumo-wallet-v1' })).not.toBeInTheDocument()
})

it('hand-ticked steps persist per source and are dropped on disconnect', async () => {
  server.use(http.post('*/api/v1/import/delete-source', () => HttpResponse.json({ success: true, message: '', data: {} })))
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(checkbox('Install econumo-wallet-v1'))
  expect(checkbox('Install econumo-wallet-v1')).toBeChecked()
  expect(JSON.parse(localStorage.getItem('appleWalletSetup:s1') ?? '[]')).toEqual(['install_wallet'])
  await user.click(checkbox('Install econumo-wallet-v1'))
  expect(checkbox('Install econumo-wallet-v1')).not.toBeChecked()
  await user.click(checkbox('Create the automation'))
  await user.click(screen.getByRole('button', { name: 'Disconnect' }))
  // the confirm dialog's own button carries the same label
  await user.click((await screen.findAllByRole('button', { name: 'Disconnect' })).at(-1) as HTMLElement)
  await waitFor(() => expect(localStorage.getItem('appleWalletSetup:s1')).toBeNull())
})

it('an existing ingest token ticks the configure step by itself', async () => {
  renderSetup(source, { personalTokens: [ingestToken] })
  await waitFor(() => expect(checkbox('Configure the Shortcuts')).toBeChecked())
})

it('iOS configure mints an ingest token, opens the shortcuts deep link and ticks the step', async () => {
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
  expect(screen.queryByText(/Open this page on your iPhone/)).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Configure on this iPhone' }))
  await waitFor(() => expect(assigned).toHaveLength(1))
  expect(body).toEqual({ name: 'Apple Wallet', scope: 'ingest', expiresAt: '' })
  expect(assigned[0].startsWith('shortcuts://run-shortcut?name=econumo-setup-v1&input=text&text=')).toBe(true)
  expect(decodeURIComponent(assigned[0].split('text=')[1])).toContain('"token":"eco_pat_new"')
  expect(checkbox('Configure the Shortcuts')).toBeChecked()
  expect(screen.getByText(/^Configured\./)).toBeInTheDocument()
  spy.mockRestore()
})

it('iOS configure toasts the server error and leaves the step unticked when minting fails', async () => {
  mockIsIOS.value = true
  server.use(http.post('*/api/v1/user/create-personal-token', () =>
    HttpResponse.json({ success: false, message: 'Too many attempts. Try again later.', code: 429, errors: {} }, { status: 429 })))
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(screen.getByRole('button', { name: 'Configure on this iPhone' }))
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Too many attempts. Try again later.'))
  expect(checkbox('Configure the Shortcuts')).not.toBeChecked()
})

it('iOS "Run econumo-wallet-v1" opens the run-shortcut deep link', async () => {
  mockIsIOS.value = true
  const assigned: string[] = []
  const spy = vi.spyOn(nav, 'openDeepLink').mockImplementation((url) => assigned.push(url))
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(screen.getByRole('button', { name: 'Run econumo-wallet-v1' }))
  expect(assigned).toEqual(['shortcuts://run-shortcut?name=econumo-wallet-v1'])
  spy.mockRestore()
})

it('Check after a manual run treats the "account is required" event as proof, discards it and ticks the step', async () => {
  let discarded: unknown
  server.use(http.post('*/api/v1/import/discard-event', async ({ request }) => {
    discarded = await request.json()
    return HttpResponse.json({ success: true, message: '', data: emptyQueue })
  }))
  const user = userEvent.setup()
  renderSetup(source, { importQueue: { queued: [], skipped: [], failed: [noInputEvent] } })
  await user.click(checkButton('test'))
  expect(await screen.findByText('The Shortcut reached Econumo.')).toBeInTheDocument()
  expect(discarded).toEqual({ eventId: 'e1' })
  expect(checkbox('Run econumo-wallet-v1 once')).toBeChecked()
  expect(JSON.parse(localStorage.getItem('appleWalletSetup:s1') ?? '[]')).toEqual(['test'])
})

it('Check with an empty queue reports nothing received and leaves the step unticked', async () => {
  const user = userEvent.setup()
  renderSetup(source)
  await user.click(checkButton('test'))
  expect(await screen.findByText(/Nothing received yet/)).toBeInTheDocument()
  expect(checkbox('Run econumo-wallet-v1 once')).not.toBeChecked()
})

it('a tapped card ticks the payment step; its Check names the card', async () => {
  const tapped = { ...source, cards: [card] }
  const user = userEvent.setup()
  renderSetup(tapped)
  expect(checkbox('Make the first payment with the iPhone unlocked')).toBeChecked()
  await user.click(checkButton('payment'))
  expect(await screen.findByText('Apple Card received — map it to an account below.')).toBeInTheDocument()
})

it('collapses to "Setup complete" once every step is done, with a toggle to show the steps again', async () => {
  localStorage.setItem('appleWalletSetup:s1', JSON.stringify(['install_wallet', 'install_setup', 'test', 'automate']))
  const user = userEvent.setup()
  renderSetup({ ...source, cards: [card] }, { personalTokens: [ingestToken] })
  expect(await screen.findByText('Setup complete')).toBeInTheDocument()
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Show steps' }))
  expect(screen.getAllByRole('checkbox')).toHaveLength(6)
  await user.click(screen.getByRole('button', { name: 'Hide steps' }))
  expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
})
