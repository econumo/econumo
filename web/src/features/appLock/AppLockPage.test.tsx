import { render, screen, waitFor, within } from '@testing-library/react'
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
import { checkPin, hashPin, verifyPin } from './pin'
import { setSecureContext, signIn, testConfig } from './testUtils'
import { AppLockPage } from './AppLockPage'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

vi.mock('./pin', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./pin')>()
  return { ...actual, hashPin: vi.fn(actual.hashPin), checkPin: vi.fn(actual.checkPin) }
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
  writeConfig(testConfig({ pin: await hashPin('1234', 1000), ...overrides }))
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
  vi.mocked(hashPin).mockClear()
  vi.mocked(checkPin).mockClear()
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
  expect(config).toMatchObject({ userId: 'u1', lockAfterMs: 60_000, biometric: null })
  expect(await verifyPin('1234', config!.pin)).toBe(true)
  expect(lockSwitch()).toBeChecked()
  // Turning it on does not lock the tab that just set it up.
  expect(useLockStore.getState().locked).toBe(false)
  expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_ENABLED)
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

  await waitFor(() => expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_PIN_CHANGED))
  const config = readConfig()!
  expect(config.pin.length).toBe(6)
  expect(await verifyPin('567890', config.pin)).toBe(true)
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
  await waitFor(() => expect(prepareBiometricRegistration).toHaveBeenCalled())
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

it('is unavailable without HTTPS', () => {
  setSecureContext(false)
  renderPage()
  expect(lockSwitch()).toBeDisabled()
  expect(screen.getByText('Needs HTTPS')).toBeInTheDocument()
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
    const hash = deferred<Awaited<ReturnType<typeof hashPin>>>()
    vi.mocked(hashPin).mockImplementationOnce(() => hash.promise)
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(hashPin).toHaveBeenCalledWith('1234'))
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    hash.resolve(testConfig().pin)
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
    const check = deferred<Awaited<ReturnType<typeof checkPin>>>()
    vi.mocked(checkPin).mockImplementationOnce(() => check.promise)
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
    const hash = deferred<Awaited<ReturnType<typeof hashPin>>>()
    vi.mocked(hashPin).mockImplementationOnce(() => hash.promise)
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(hashPin).toHaveBeenCalledWith('5678'))
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    hash.resolve({ salt: 'bmV3', hash: 'bmV3', iterations: 1000, length: 4 })
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(await verifyPin('1234', readConfig()!.pin)).toBe(true)
    expect(trackEvent).not.toHaveBeenCalledWith(METRICS.APP_LOCK_PIN_CHANGED)
  })
})
