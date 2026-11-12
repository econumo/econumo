import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider, useLocation } from 'react-router'
import { toast } from 'sonner'
import { queryKeys } from '@/app/queryKeys'
import { METRICS, trackEvent } from '@/lib/metrics'
import { server } from '@/test/msw'
import { coreHandlers, fixtureUser } from '@/test/fixtures'
import { biometricAvailable, prepareBiometricRegistration, verifyBiometric } from './biometric'
import { readAttempts, readConfig, recordFailedAttempt, writeConfig } from './config'
import { resetAppLockForTests, useLockStore } from './lockStore'
import { checkSecret, hashSecret, PIN_ITERATIONS_JS, verifySecret } from './secret'
import { drawPattern, setSecureContext, signIn, testConfig } from './testUtils'
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

vi.mock('./biometric', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./biometric')>()
  return {
    ...actual,
    biometricAvailable: vi.fn(),
    prepareBiometricRegistration: vi.fn(),
    verifyBiometric: vi.fn(),
  }
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
  setSecureContext(true)
  signIn('u1')
  resetAppLockForTests()
  vi.mocked(trackEvent).mockClear()
  vi.mocked(toast.error).mockClear()
  vi.mocked(biometricAvailable).mockResolvedValue(false)
  vi.mocked(prepareBiometricRegistration).mockReset()
  vi.mocked(verifyBiometric).mockReset()
  vi.mocked(hashSecret).mockClear()
  vi.mocked(checkSecret).mockClear()
})

afterEach(() => {
  setSecureContext(false)
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
  expect(config).toMatchObject({ userId: 'u1', lockAfterMs: 0, biometric: null })
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

it('has no biometrics row where the device cannot do it', async () => {
  await configure()
  renderPage()
  await screen.findByText('Change PIN')
  await Promise.resolve()
  expect(screen.queryByRole('switch', { name: 'Use biometrics' })).not.toBeInTheDocument()
})

it('registers a passkey for biometrics and forgets it when turned off', async () => {
  await configure()
  vi.mocked(biometricAvailable).mockResolvedValue(true)
  const start = vi.fn().mockResolvedValue({ status: 'ok', credentialId: 'cred-1' })
  vi.mocked(prepareBiometricRegistration).mockResolvedValue(start)
  const user = userEvent.setup()
  renderPage()
  const toggle = await screen.findByRole('switch', { name: 'Use biometrics' })
  expect(screen.getByText(/may also accept its passcode/)).toBeInTheDocument()
  await waitFor(() =>
    expect(prepareBiometricRegistration).toHaveBeenCalledWith({ userId: 'u1', email: fixtureUser.email }, null),
  )
  await user.click(toggle)
  await waitFor(() => expect(readConfig()!.biometric).toEqual({ credentialId: 'cred-1' }))
  expect(start).toHaveBeenCalledTimes(1)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_BIOMETRIC_ENABLED)
  await waitFor(() => expect(toggle).toBeChecked())

  await user.click(toggle)
  expect(readConfig()!.biometric).toBeNull()
  expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_BIOMETRIC_DISABLED)
  expect(readConfig()).not.toBeNull()
})

it('a cancelled passkey prompt leaves biometrics off with a toast', async () => {
  await configure()
  vi.mocked(biometricAvailable).mockResolvedValue(true)
  vi.mocked(prepareBiometricRegistration).mockResolvedValue(vi.fn().mockResolvedValue({ status: 'cancelled' }))
  const user = userEvent.setup()
  renderPage()
  const toggle = await screen.findByRole('switch', { name: 'Use biometrics' })
  await waitFor(() => expect(toggle).toBeEnabled())
  await user.click(toggle)
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Couldn't turn on biometrics"))
  expect(toggle).not.toBeChecked()
  expect(readConfig()!.biometric).toBeNull()
  expect(trackEvent).not.toHaveBeenCalledWith(METRICS.APP_LOCK_BIOMETRIC_ENABLED)
})

it('turning the lock off accepts biometrics instead of the PIN', async () => {
  await configure({ biometric: { credentialId: 'cred-1' } })
  vi.mocked(verifyBiometric).mockResolvedValue('ok')
  const user = userEvent.setup()
  renderPage()
  await user.click(lockSwitch())
  const dialog = await screen.findByRole('dialog', { name: 'Enter your current PIN' })
  await user.click(within(dialog).getByRole('button', { name: 'Unlock with biometrics' }))
  await waitFor(() => expect(readConfig()).toBeNull())
  expect(verifyBiometric).toHaveBeenCalledWith('cred-1')
})

it('keeps the biometrics switch disabled until the passkey request is prepared', async () => {
  await configure()
  vi.mocked(biometricAvailable).mockResolvedValue(true)
  let ready: (start: () => Promise<{ status: 'ok'; credentialId: string }>) => void = () => {}
  vi.mocked(prepareBiometricRegistration).mockReturnValue(new Promise((resolve) => { ready = resolve }))
  renderPage()
  const toggle = await screen.findByRole('switch', { name: 'Use biometrics' })
  expect(toggle).toBeDisabled()
  ready(vi.fn().mockResolvedValue({ status: 'ok', credentialId: 'cred-1' }))
  await waitFor(() => expect(toggle).toBeEnabled())
})

it('a passkey answer arriving after the dialog closed changes nothing', async () => {
  await configure({ biometric: { credentialId: 'cred-1' } })
  localStorage.setItem('econumo.appLock.attempts', JSON.stringify({ failedAttempts: 3, lockedUntil: null }))
  let answer: (result: 'ok') => void = () => {}
  vi.mocked(verifyBiometric).mockReturnValue(new Promise((resolve) => { answer = resolve }))
  const user = userEvent.setup()
  renderPage()
  await user.click(lockSwitch())
  const dialog = await screen.findByRole('dialog', { name: 'Enter your current PIN' })
  await user.click(within(dialog).getByRole('button', { name: 'Unlock with biometrics' }))
  await user.keyboard('{Escape}')
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  answer('ok')
  await new Promise((resolve) => setTimeout(resolve, 0))
  expect(readConfig()).not.toBeNull()
  expect(JSON.parse(localStorage.getItem('econumo.appLock.attempts')!).failedAttempts).toBe(3)
})

it('works over plain http, showing biometrics as needing HTTPS', async () => {
  setSecureContext(false)
  const realCrypto = globalThis.crypto
  vi.stubGlobal('crypto', { getRandomValues: realCrypto.getRandomValues.bind(realCrypto), randomUUID: realCrypto.randomUUID.bind(realCrypto) })
  onTestFinished(() => {
    vi.unstubAllGlobals()
  })
  const user = userEvent.setup()
  renderPage()
  expect(screen.queryByText('Face ID / fingerprint needs HTTPS')).not.toBeInTheDocument()
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
  const note = await screen.findByText('Face ID / fingerprint needs HTTPS')
  expect(note.closest('[aria-disabled="true"]')).toHaveTextContent('Use biometrics')
  expect(screen.getByRole('switch', { name: 'Use biometrics' })).toBeDisabled()
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
