import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider, useLocation } from 'react-router'
import { toast } from 'sonner'
import { queryKeys } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { readAttempts, readConfig, recordFailedAttempt, writeConfig } from './config'
import { resetAppLockForTests, useLockStore } from './lockStore'
import { checkSecret, hashSecret, PIN_ITERATIONS_JS, verifySecret } from './secret'
import { drawPattern, signIn, testConfig } from './testUtils'
import { AppLockPage } from './AppLockPage'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

vi.mock('./secret', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./secret')>()
  return { ...actual, hashSecret: vi.fn(actual.hashSecret), checkSecret: vi.fn(actual.checkSecret) }
})

function LogoutProbe() {
  const { state } = useLocation()
  return <div data-testid="logout">{JSON.stringify(state)}</div>
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  queryClient.setQueryData(queryKeys.user, fixtureUser)
  const router = createMemoryRouter(
    [
      { path: '/settings/profile/app-lock', element: <AppLockPage /> },
      { path: '/logout', element: <LogoutProbe /> },
    ],
    { initialEntries: ['/settings/profile/app-lock'] },
  )
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

async function configure(overrides: Parameters<typeof testConfig>[0] = {}) {
  writeConfig(testConfig({ secret: await hashSecret('1234', 1000), ...overrides }))
}

async function typePin(user: ReturnType<typeof userEvent.setup>, dialog: HTMLElement, pin: string) {
  for (const d of pin) {
    await user.click(within(dialog).getByRole('button', { name: d }))
  }
}

const lockSwitch = () => screen.getByRole('switch', { name: 'App lock' })

beforeEach(() => {
  localStorage.clear()
  sessionStorage.clear()
  server.use(...coreHandlers())
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(), addListener: vi.fn(), removeListener: vi.fn(),
  }))
  signIn('u1')
  resetAppLockForTests()
  vi.mocked(trackEvent).mockClear()
  vi.mocked(toast.error).mockClear()
  vi.mocked(hashSecret).mockClear()
  vi.mocked(checkSecret).mockClear()
})

it('turns the lock on after the PIN is typed twice, starting over on a mismatch', async () => {
  const user = userEvent.setup()
  renderPage()
  expect(lockSwitch()).not.toBeChecked()
  expect(screen.queryByText('Change PIN')).not.toBeInTheDocument()

  await user.click(lockSwitch())
  let dialog = await screen.findByRole('dialog', { name: 'Choose a PIN' })
  await typePin(user, dialog, '1234')
  await user.click(within(dialog).getByRole('button', { name: 'Continue' }))
  dialog = await screen.findByRole('dialog', { name: 'Enter the PIN again' })
  await typePin(user, dialog, '1235')
  await user.click(within(dialog).getByRole('button', { name: 'Save' }))

  dialog = await screen.findByRole('dialog', { name: 'Choose a PIN' })
  expect(within(dialog).getByText("The PINs don't match. Try again.")).toBeInTheDocument()
  expect(readConfig()).toBeNull()

  await typePin(user, dialog, '1234')
  await user.click(within(dialog).getByRole('button', { name: 'Continue' }))
  dialog = await screen.findByRole('dialog', { name: 'Enter the PIN again' })
  await typePin(user, dialog, '1234')
  await user.click(within(dialog).getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  const config = readConfig()
  expect(config).toMatchObject({ userId: 'u1', lockAfterMs: 60_000 })
  expect(await verifySecret('1234', config!.secret)).toBe(true)
  expect(lockSwitch()).toBeChecked()
  // Turning it on does not lock the tab that just set it up.
  expect(useLockStore.getState().locked).toBe(false)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_ENABLED, { kind: 'pin' })
  expect(screen.getByText('Change PIN')).toBeInTheDocument()
})

it('cancelling the set-up leaves the lock off', async () => {
  const user = userEvent.setup()
  renderPage()
  await user.click(lockSwitch())
  await screen.findByRole('dialog', { name: 'Choose a PIN' })
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(lockSwitch()).not.toBeChecked()
  expect(readConfig()).toBeNull()
})

it('changes the PIN after the current one, counting a wrong try', async () => {
  await configure()
  const user = userEvent.setup()
  renderPage()
  await user.click(screen.getByRole('button', { name: 'Change PIN' }))
  let dialog = await screen.findByRole('dialog', { name: 'Enter your current PIN' })
  await typePin(user, dialog, '9999')
  expect(await within(dialog).findByText('Wrong PIN')).toBeInTheDocument()
  expect(readAttempts().failedAttempts).toBe(1)

  await typePin(user, dialog, '1234')
  dialog = await screen.findByRole('dialog', { name: 'Choose a new PIN' })
  expect(readAttempts().failedAttempts).toBe(0)
  await typePin(user, dialog, '567890')
  await user.click(within(dialog).getByRole('button', { name: 'Continue' }))
  dialog = await screen.findByRole('dialog', { name: 'Enter the PIN again' })
  await typePin(user, dialog, '567890')
  await user.click(within(dialog).getByRole('button', { name: 'Save' }))

  await waitFor(() => expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_SECRET_CHANGED, { kind: 'pin' }))
  const config = readConfig()!
  expect(config.secret.length).toBe(6)
  expect(await verifySecret('567890', config!.secret)).toBe(true)
  expect(config.lockAfterMs).toBe(60_000)
})

it('turning it off needs the current PIN, and a wrong one counts', async () => {
  await configure()
  useLockStore.setState({ locked: false })
  const user = userEvent.setup()
  renderPage()
  await user.click(lockSwitch())
  const dialog = await screen.findByRole('dialog', { name: 'Enter your current PIN' })
  expect(screen.getByRole('switch', { name: 'App lock', hidden: true })).toBeChecked()
  await typePin(user, dialog, '0000')
  expect(await within(dialog).findByText('Wrong PIN')).toBeInTheDocument()
  expect(readAttempts().failedAttempts).toBe(1)
  expect(readConfig()).not.toBeNull()

  await typePin(user, dialog, '1234')
  await waitFor(() => expect(readConfig()).toBeNull())
  expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_DISABLED)
  await waitFor(() => expect(lockSwitch()).not.toBeChecked())
  expect(useLockStore.getState().locked).toBe(false)
})

it('the tenth wrong PIN in settings signs out, keeping the offline queue', async () => {
  await configure()
  for (let i = 0; i < 9; i++) {
    recordFailedAttempt(Date.now() - 3_600_000)
  }
  const user = userEvent.setup()
  renderPage()
  await user.click(screen.getByRole('button', { name: 'Change PIN' }))
  const dialog = await screen.findByRole('dialog', { name: 'Enter your current PIN' })
  expect(within(dialog).getByText('1 attempt left')).toBeInTheDocument()
  await typePin(user, dialog, '0000')
  const logout = await screen.findByTestId('logout')
  expect(JSON.parse(logout.textContent!)).toEqual({ keepPendingWrites: true, resetAppLock: true })
  expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_ATTEMPTS_EXCEEDED)
})

it('stores the lock-after choice', async () => {
  await configure()
  const user = userEvent.setup()
  renderPage()
  const select = screen.getByRole('combobox', { name: 'Lock after' })
  expect(select).toHaveTextContent('1 minute')
  await user.click(select)
  await user.click(await screen.findByRole('option', { name: '5 minutes' }))
  expect(readConfig()!.lockAfterMs).toBe(300_000)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_TIMEOUT_CHANGED, { lockAfter: 300 })
  expect(screen.getByRole('combobox', { name: 'Lock after' })).toHaveTextContent('5 minutes')
})

it('offers no biometrics', async () => {
  await configure()
  renderPage()
  await screen.findByText('Change PIN')
  expect(screen.getAllByRole('switch')).toHaveLength(1)
  expect(screen.queryByText(/biometric/i)).not.toBeInTheDocument()
})

it('works over plain http', async () => {
  const realCrypto = globalThis.crypto
  vi.stubGlobal('crypto', { getRandomValues: realCrypto.getRandomValues.bind(realCrypto), randomUUID: realCrypto.randomUUID.bind(realCrypto) })
  onTestFinished(() => {
    vi.unstubAllGlobals()
  })
  const user = userEvent.setup()
  renderPage()
  await user.click(lockSwitch())
  let dialog = await screen.findByRole('dialog', { name: 'Choose a PIN' })
  await typePin(user, dialog, '2580')
  await user.click(within(dialog).getByRole('button', { name: 'Continue' }))
  dialog = await screen.findByRole('dialog', { name: 'Enter the PIN again' })
  await typePin(user, dialog, '2580')
  await user.click(within(dialog).getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(readConfig()).not.toBeNull())
  expect(readConfig()!.secret.iterations).toBe(PIN_ITERATIONS_JS)
  expect(lockSwitch()).toBeChecked()
})

it('turning the lock on spends the sign-in marker, so a reload locks', async () => {
  sessionStorage.setItem('econumo.appLock.fresh', String(Date.now()))
  const user = userEvent.setup()
  renderPage()
  await user.click(lockSwitch())
  let dialog = await screen.findByRole('dialog', { name: 'Choose a PIN' })
  await typePin(user, dialog, '1234')
  await user.click(within(dialog).getByRole('button', { name: 'Continue' }))
  dialog = await screen.findByRole('dialog', { name: 'Enter the PIN again' })
  await typePin(user, dialog, '1234')
  await user.click(within(dialog).getByRole('button', { name: 'Save' }))
  await waitFor(() => expect(readConfig()).not.toBeNull())
  expect(sessionStorage.getItem('econumo.appLock.fresh')).toBeNull()
})

describe('closing the dialog while it is working', () => {
  function deferred<T>() {
    let resolve: (value: T) => void = () => {}
    const promise = new Promise<T>((r) => (resolve = r))
    return { promise, resolve }
  }

  it('does not turn the lock on', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(lockSwitch())
    let dialog = await screen.findByRole('dialog', { name: 'Choose a PIN' })
    await typePin(user, dialog, '1234')
    await user.click(within(dialog).getByRole('button', { name: 'Continue' }))
    dialog = await screen.findByRole('dialog', { name: 'Enter the PIN again' })
    await typePin(user, dialog, '1234')
    const hash = deferred<Awaited<ReturnType<typeof hashSecret>>>()
    vi.mocked(hashSecret).mockImplementationOnce(() => hash.promise)
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(hashSecret).toHaveBeenCalledWith('1234'))
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    hash.resolve(testConfig().secret)
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(readConfig()).toBeNull()
    expect(trackEvent).not.toHaveBeenCalledWith(METRICS.APP_LOCK_ENABLED)
    expect(lockSwitch()).not.toBeChecked()
  })

  it('does not turn the lock off', async () => {
    await configure()
    const user = userEvent.setup()
    renderPage()
    await user.click(lockSwitch())
    const dialog = await screen.findByRole('dialog', { name: 'Enter your current PIN' })
    const check = deferred<Awaited<ReturnType<typeof checkSecret>>>()
    vi.mocked(checkSecret).mockImplementationOnce(() => check.promise)
    await typePin(user, dialog, '1234')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    check.resolve({ status: 'ok' })
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(readConfig()).not.toBeNull()
    expect(trackEvent).not.toHaveBeenCalledWith(METRICS.APP_LOCK_DISABLED)
    expect(lockSwitch()).toBeChecked()
  })

  it('does not change the PIN', async () => {
    await configure()
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: 'Change PIN' }))
    let dialog = await screen.findByRole('dialog', { name: 'Enter your current PIN' })
    await typePin(user, dialog, '1234')
    dialog = await screen.findByRole('dialog', { name: 'Choose a new PIN' })
    await typePin(user, dialog, '5678')
    await user.click(within(dialog).getByRole('button', { name: 'Continue' }))
    dialog = await screen.findByRole('dialog', { name: 'Enter the PIN again' })
    await typePin(user, dialog, '5678')
    const hash = deferred<Awaited<ReturnType<typeof hashSecret>>>()
    vi.mocked(hashSecret).mockImplementationOnce(() => hash.promise)
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(hashSecret).toHaveBeenCalledWith('5678'))
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    hash.resolve({ kind: 'pin', salt: 'bmV3', hash: 'bmV3', iterations: 1000, length: 4 })
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(await verifySecret('1234', readConfig()!.secret)).toBe(true)
    expect(trackEvent).not.toHaveBeenCalledWith(METRICS.APP_LOCK_SECRET_CHANGED)
  })

  it('does not sign out when the tenth failure lands after the dialog closed', async () => {
    await configure()
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: 'Change PIN' }))
    const dialog = await screen.findByRole('dialog', { name: 'Enter your current PIN' })
    const check = deferred<Awaited<ReturnType<typeof checkSecret>>>()
    vi.mocked(checkSecret).mockImplementationOnce(() => check.promise)
    await typePin(user, dialog, '0000')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    check.resolve({ status: 'exceeded' })
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(screen.queryByTestId('logout')).not.toBeInTheDocument()
    expect(trackEvent).not.toHaveBeenCalledWith(METRICS.APP_LOCK_ATTEMPTS_EXCEEDED)
  })
})

describe('patterns', () => {
  const draw = (dialog: HTMLElement, nodes: number[]) => drawPattern(fireEvent, within(dialog).getByTestId('pattern-grid'), nodes)

  it('sets up a pattern instead of a PIN, starting over on a mismatch', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(lockSwitch())
    let dialog = await screen.findByRole('dialog', { name: 'Choose a PIN' })
    await typePin(user, dialog, '12')
    await user.click(within(dialog).getByRole('button', { name: 'Use a pattern instead' }))
    dialog = await screen.findByRole('dialog', { name: 'Draw a pattern' })
    expect(within(dialog).getByText('Connect at least 4 dots')).toBeInTheDocument()
    draw(dialog, [0, 2, 8])
    dialog = await screen.findByRole('dialog', { name: 'Draw the pattern again' })
    draw(dialog, [0, 4, 8, 5])
    dialog = await screen.findByRole('dialog', { name: 'Draw a pattern' })
    expect(within(dialog).getByText("The patterns don't match. Try again.")).toBeInTheDocument()
    expect(readConfig()).toBeNull()

    draw(dialog, [0, 2, 8])
    dialog = await screen.findByRole('dialog', { name: 'Draw the pattern again' })
    draw(dialog, [0, 2, 8])
    await waitFor(() => expect(readConfig()).not.toBeNull())
    const config = readConfig()!
    expect(config.secret).toMatchObject({ kind: 'pattern', length: 5 })
    expect(await verifySecret('p:0-1-2-5-8', config.secret)).toBe(true)
    expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_ENABLED, { kind: 'pattern' })
    expect(await screen.findByRole('button', { name: 'Change pattern' })).toBeInTheDocument()
  })

  it('switches back to a PIN during set-up', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(lockSwitch())
    let dialog = await screen.findByRole('dialog', { name: 'Choose a PIN' })
    await user.click(within(dialog).getByRole('button', { name: 'Use a pattern instead' }))
    dialog = await screen.findByRole('dialog', { name: 'Draw a pattern' })
    await user.click(within(dialog).getByRole('button', { name: 'Use a PIN instead' }))
    dialog = await screen.findByRole('dialog', { name: 'Choose a PIN' })
    expect(within(dialog).getByTestId('pin-dots')).toBeInTheDocument()
  })

  it('changes a PIN into a pattern after the current PIN', async () => {
    await configure()
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: 'Change PIN' }))
    let dialog = await screen.findByRole('dialog', { name: 'Enter your current PIN' })
    expect(within(dialog).queryByRole('button', { name: 'Use a pattern instead' })).not.toBeInTheDocument()
    await typePin(user, dialog, '1234')
    dialog = await screen.findByRole('dialog', { name: 'Choose a new PIN' })
    await user.click(within(dialog).getByRole('button', { name: 'Use a pattern instead' }))
    dialog = await screen.findByRole('dialog', { name: 'Draw a new pattern' })
    draw(dialog, [6, 7, 8, 5])
    dialog = await screen.findByRole('dialog', { name: 'Draw the pattern again' })
    draw(dialog, [6, 7, 8, 5])
    await waitFor(() => expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_SECRET_CHANGED, { kind: 'pattern' }))
    expect(toast.success).toHaveBeenCalledWith('Pattern changed')
    expect(readConfig()!.secret.kind).toBe('pattern')
    expect(await screen.findByRole('button', { name: 'Change pattern' })).toBeInTheDocument()
  })

  it('asks for the current pattern to change it, offering a PIN as the new secret', async () => {
    writeConfig(testConfig({ secret: await hashSecret('p:0-1-2-5-8', 1000) }))
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: 'Change pattern' }))
    let dialog = await screen.findByRole('dialog', { name: 'Draw your current pattern' })
    draw(dialog, [6, 7, 8, 5])
    expect(await within(dialog).findByText('Wrong pattern')).toBeInTheDocument()
    expect(readAttempts().failedAttempts).toBe(1)
    draw(dialog, [0, 2, 8])
    dialog = await screen.findByRole('dialog', { name: 'Draw a new pattern' })
    await user.click(within(dialog).getByRole('button', { name: 'Use a PIN instead' }))
    dialog = await screen.findByRole('dialog', { name: 'Choose a new PIN' })
    await typePin(user, dialog, '2580')
    await user.click(within(dialog).getByRole('button', { name: 'Continue' }))
    dialog = await screen.findByRole('dialog', { name: 'Enter the PIN again' })
    await typePin(user, dialog, '2580')
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_SECRET_CHANGED, { kind: 'pin' }))
    expect(await verifySecret('2580', readConfig()!.secret)).toBe(true)
  })

  it('turning it off needs the current pattern', async () => {
    writeConfig(testConfig({ secret: await hashSecret('p:0-1-2-5-8', 1000) }))
    const user = userEvent.setup()
    renderPage()
    await user.click(lockSwitch())
    const dialog = await screen.findByRole('dialog', { name: 'Draw your current pattern' })
    draw(dialog, [0, 2, 8])
    await waitFor(() => expect(readConfig()).toBeNull())
    expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_DISABLED)
  })
})

it('a config from before patterns (no kind) still works for the change flow', async () => {
  const { secret, ...rest } = testConfig({ secret: await hashSecret('1234', 1000) })
  const { kind: _kind, ...pin } = secret
  localStorage.setItem('econumo.appLock', JSON.stringify({ ...rest, pin }))
  const user = userEvent.setup()
  renderPage()
  await user.click(screen.getByRole('button', { name: 'Change PIN' }))
  const dialog = await screen.findByRole('dialog', { name: 'Enter your current PIN' })
  await typePin(user, dialog, '1234')
  expect(await screen.findByRole('dialog', { name: 'Choose a new PIN' })).toBeInTheDocument()
})

describe('attempts left in the current-secret prompt', () => {
  it('stays visible after a wrong PIN and through the lockout', async () => {
    await configure()
    for (let i = 0; i < 6; i++) {
      recordFailedAttempt(Date.now() - 3_600_000)
    }
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: 'Change PIN' }))
    const dialog = await screen.findByRole('dialog', { name: 'Enter your current PIN' })
    expect(within(dialog).getByText('4 attempts left')).toBeInTheDocument()
    await typePin(user, dialog, '0000')
    expect(await within(dialog).findByText('3 attempts left')).toBeInTheDocument()
    expect(within(dialog).getByText(/Try again in/)).toBeInTheDocument()
  })

  it('stays visible after a wrong pattern', async () => {
    writeConfig(testConfig({ secret: await hashSecret('p:0-1-2-5-8', 1000) }))
    localStorage.setItem('econumo.appLock.attempts', JSON.stringify({ failedAttempts: 6, lockedUntil: null }))
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: 'Change pattern' }))
    const dialog = await screen.findByRole('dialog', { name: 'Draw your current pattern' })
    drawPattern(fireEvent, within(dialog).getByTestId('pattern-grid'), [6, 7, 8, 5])
    expect(await within(dialog).findByText('3 attempts left')).toBeInTheDocument()
  })
})
