import { StrictMode } from 'react'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider, useLocation } from 'react-router'
import { METRICS, trackEvent } from '@/lib/metrics'
import { queryKeys } from '@/app/queryKeys'
import { APP_LOCK_CONFIG_KEY } from './keys'
import { readAttempts, recordFailedAttempt, writeConfig } from './config'
import { resetAppLockForTests, useLockStore } from './lockStore'
import { hashSecret } from './secret'
import { verifyBiometric } from './biometric'
import { drawPattern, setSecureContext, signIn, testConfig } from './testUtils'
import { LockScreen } from './LockScreen'

vi.mock('@/lib/metrics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/metrics')>()
  return { ...actual, trackEvent: vi.fn() }
})

vi.mock('./biometric', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./biometric')>()
  return { ...actual, verifyBiometric: vi.fn() }
})

const reload = vi.fn()
const realLocation = window.location
const realMatchMedia = window.matchMedia

function LogoutProbe() {
  const { state } = useLocation()
  return <div data-testid="logout">{JSON.stringify(state)}</div>
}

function pointer(coarse: boolean) {
  window.matchMedia = ((query: string) => ({
    matches: coarse && query === '(pointer: coarse)',
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  })) as unknown as typeof window.matchMedia
}

async function configure(overrides: Parameters<typeof testConfig>[0] = {}) {
  signIn('u1')
  writeConfig(testConfig({ secret: await hashSecret('1234', 1000), ...overrides }))
  useLockStore.setState({ locked: true })
}

function renderLockScreen(queryClient = new QueryClient()) {
  const router = createMemoryRouter(
    [
      { path: '/', element: <LockScreen /> },
      { path: '/logout', element: <LogoutProbe /> },
    ],
    { initialEntries: ['/'] },
  )
  render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </StrictMode>,
  )
  return router
}

async function typePin(user: ReturnType<typeof userEvent.setup>, pin: string) {
  for (const d of pin) {
    await user.click(screen.getByRole('button', { name: d }))
  }
}

// Failures from long ago: counted, but their lockouts are over.
function seedFailures(count: number) {
  for (let i = 0; i < count; i++) {
    recordFailedAttempt(Date.now() - 3_600_000)
  }
}

beforeEach(() => {
  localStorage.clear()
  sessionStorage.clear()
  setSecureContext(true)
  resetAppLockForTests()
  vi.mocked(trackEvent).mockClear()
  vi.mocked(verifyBiometric).mockReset()
  reload.mockClear()
  pointer(false)
  Object.defineProperty(window, 'location', { value: { ...realLocation, reload }, writable: true, configurable: true })
})

afterEach(() => {
  Object.defineProperty(window, 'location', { value: realLocation, writable: true, configurable: true })
  window.matchMedia = realMatchMedia
  resetAppLockForTests()
  setSecureContext(false)
})

it('the right PIN unlocks', async () => {
  const user = userEvent.setup()
  await configure()
  seedFailures(2)
  renderLockScreen()
  expect(screen.getByRole('heading', { name: 'Enter PIN' })).toBeInTheDocument()
  await typePin(user, '1234')
  await waitFor(() => expect(useLockStore.getState().locked).toBe(false))
  expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_UNLOCKED, { method: 'pin' })
  expect(readAttempts().failedAttempts).toBe(0)
})

it('shows the cached user without fetching', async () => {
  await configure()
  const queryClient = new QueryClient()
  queryClient.setQueryData(queryKeys.user, { id: 'u1', name: 'Dmitry', avatar: 'face:blue' })
  renderLockScreen(queryClient)
  expect(screen.getByText('Dmitry')).toBeInTheDocument()
  expect(screen.getByTestId('user-avatar')).toHaveAttribute('data-avatar', 'face:blue')
})

it('shows the user once the persisted cache restores it', async () => {
  await configure()
  const queryClient = new QueryClient()
  renderLockScreen(queryClient)
  expect(screen.queryByText('Dmitry')).not.toBeInTheDocument()
  act(() => queryClient.setQueryData(queryKeys.user, { id: 'u1', name: 'Dmitry', avatar: 'face:blue' }))
  expect(await screen.findByText('Dmitry')).toBeInTheDocument()
})

it('a wrong PIN says so and stays locked', async () => {
  const user = userEvent.setup()
  await configure()
  renderLockScreen()
  await typePin(user, '9999')
  expect(await screen.findByText('Wrong PIN')).toBeInTheDocument()
  expect(useLockStore.getState().locked).toBe(true)
  expect(readAttempts().failedAttempts).toBe(1)
  expect(screen.queryByText(/attempts? left/)).not.toBeInTheDocument()
})

it('counts down the remaining attempts once fewer than 5 are left', async () => {
  const user = userEvent.setup()
  await configure()
  seedFailures(6)
  renderLockScreen()
  expect(screen.getByText('4 attempts left')).toBeInTheDocument()
  await typePin(user, '9999')
  expect(await screen.findByText('3 attempts left')).toBeInTheDocument()
})

it('the 5th failure locks the keypad with a countdown', async () => {
  const user = userEvent.setup()
  await configure()
  seedFailures(4)
  renderLockScreen()
  await typePin(user, '9999')
  expect(await screen.findByTestId('lockout')).toHaveTextContent('Try again in 0:30')
  expect(screen.getByRole('button', { name: '1' })).toBeDisabled()
  await waitFor(() => expect(screen.getByTestId('lockout')).toHaveTextContent('Try again in 0:29'), { timeout: 2500 })
})

it('a lockout survives a reload', async () => {
  await configure()
  seedFailures(4)
  recordFailedAttempt()
  renderLockScreen()
  expect(screen.getByTestId('lockout')).toHaveTextContent(/Try again in 0:(30|29)/)
  expect(screen.getByRole('button', { name: '1' })).toBeDisabled()
})

it('the 10th failure signs out, keeping the queue and resetting the lock', async () => {
  const user = userEvent.setup()
  await configure()
  seedFailures(9)
  const router = renderLockScreen()
  await typePin(user, '9999')
  await waitFor(() => expect(router.state.location.pathname).toBe('/logout'))
  expect(router.state.location.state).toEqual({ keepPendingWrites: true, resetAppLock: true })
  expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_ATTEMPTS_EXCEEDED)
})

it('Forgot PIN asks first, then signs out to reset the lock', async () => {
  const user = userEvent.setup()
  await configure()
  const router = renderLockScreen()
  await user.click(screen.getByRole('button', { name: 'Forgot PIN?' }))
  const dialog = await screen.findByRole('dialog')
  expect(within(dialog).getByText('Log out to reset the PIN?')).toBeInTheDocument()
  expect(router.state.location.pathname).toBe('/')
  await user.click(within(dialog).getByRole('button', { name: 'Log out' }))
  await waitFor(() => expect(router.state.location.pathname).toBe('/logout'))
  expect(router.state.location.state).toEqual({ keepPendingWrites: true, resetAppLock: true })
  expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_FORGOT_SECRET, { kind: 'pin' })
})

it('has no biometric button without a passkey', async () => {
  await configure()
  renderLockScreen()
  expect(screen.queryByRole('button', { name: 'Unlock with biometrics' })).not.toBeInTheDocument()
})

it('the biometric button unlocks, even during a lockout', async () => {
  const user = userEvent.setup()
  await configure({ biometric: { credentialId: 'AAAA' } })
  seedFailures(4)
  recordFailedAttempt()
  vi.mocked(verifyBiometric).mockResolvedValue('ok')
  renderLockScreen()
  await user.click(screen.getByRole('button', { name: 'Unlock with biometrics' }))
  expect(verifyBiometric).toHaveBeenCalledWith('AAAA')
  await waitFor(() => expect(useLockStore.getState().locked).toBe(false))
  expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_UNLOCKED, { method: 'biometric' })
  expect(readAttempts().failedAttempts).toBe(0)
})

it('a cancelled biometric check stays locked without an error', async () => {
  const user = userEvent.setup()
  await configure({ biometric: { credentialId: 'AAAA' } })
  vi.mocked(verifyBiometric).mockResolvedValue('cancelled')
  renderLockScreen()
  await user.click(screen.getByRole('button', { name: 'Unlock with biometrics' }))
  await waitFor(() => expect(verifyBiometric).toHaveBeenCalled())
  expect(useLockStore.getState().locked).toBe(true)
  expect(screen.queryByText(/Couldn't verify/)).not.toBeInTheDocument()
  expect(trackEvent).not.toHaveBeenCalled()
})

it('a failed biometric check asks for the PIN', async () => {
  const user = userEvent.setup()
  await configure({ biometric: { credentialId: 'AAAA' } })
  vi.mocked(verifyBiometric).mockResolvedValue('failed')
  renderLockScreen()
  await user.click(screen.getByRole('button', { name: 'Unlock with biometrics' }))
  expect(await screen.findByText("Couldn't verify it's you. Enter your PIN.")).toBeInTheDocument()
  expect(useLockStore.getState().locked).toBe(true)
})

describe('the automatic biometric attempt', () => {
  it('runs once on a touch device', async () => {
    pointer(true)
    vi.spyOn(document, 'hasFocus').mockReturnValue(true)
    await configure({ biometric: { credentialId: 'AAAA' } })
    vi.mocked(verifyBiometric).mockResolvedValue('ok')
    renderLockScreen()
    await waitFor(() => expect(useLockStore.getState().locked).toBe(false))
    expect(verifyBiometric).toHaveBeenCalledTimes(1)
    expect(vi.mocked(verifyBiometric).mock.calls[0][1]).toBeInstanceOf(AbortSignal)
    vi.mocked(document.hasFocus).mockRestore()
  })

  it('is silent when rejected', async () => {
    pointer(true)
    vi.spyOn(document, 'hasFocus').mockReturnValue(true)
    await configure({ biometric: { credentialId: 'AAAA' } })
    vi.mocked(verifyBiometric).mockResolvedValue('failed')
    renderLockScreen()
    await waitFor(() => expect(verifyBiometric).toHaveBeenCalledTimes(1))
    expect(screen.queryByText(/Couldn't verify/)).not.toBeInTheDocument()
    expect(useLockStore.getState().locked).toBe(true)
    vi.mocked(document.hasFocus).mockRestore()
  })

  it('runs once the page it appeared on in the background comes forward', async () => {
    pointer(true)
    const focus = vi.spyOn(document, 'hasFocus').mockReturnValue(false)
    await configure({ biometric: { credentialId: 'AAAA' } })
    vi.mocked(verifyBiometric).mockResolvedValue('cancelled')
    renderLockScreen()
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(verifyBiometric).not.toHaveBeenCalled()
    focus.mockReturnValue(true)
    act(() => {
      window.dispatchEvent(new Event('focus'))
    })
    await waitFor(() => expect(verifyBiometric).toHaveBeenCalledTimes(1))
    act(() => {
      window.dispatchEvent(new Event('focus'))
      document.dispatchEvent(new Event('visibilitychange'))
    })
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(verifyBiometric).toHaveBeenCalledTimes(1)
    focus.mockRestore()
  })

  it('does not run with a mouse', async () => {
    pointer(false)
    await configure({ biometric: { credentialId: 'AAAA' } })
    renderLockScreen()
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(verifyBiometric).not.toHaveBeenCalled()
  })
})

it('reloads when the lock configuration is gone', async () => {
  await configure()
  localStorage.removeItem(APP_LOCK_CONFIG_KEY)
  renderLockScreen()
  await waitFor(() => expect(reload).toHaveBeenCalled())
})

it('reloads when the configuration vanishes before the PIN is checked', async () => {
  const user = userEvent.setup()
  await configure()
  renderLockScreen()
  await typePin(user, '12')
  localStorage.removeItem(APP_LOCK_CONFIG_KEY)
  await typePin(user, '34')
  await waitFor(() => expect(reload).toHaveBeenCalled())
})

describe('with a pattern', () => {
  async function configurePattern(overrides: Parameters<typeof testConfig>[0] = {}) {
    await configure({ secret: await hashSecret('p:0-1-2-5-8', 1000), ...overrides })
  }

  it('asks for the pattern and unlocks with it', async () => {
    await configurePattern()
    renderLockScreen()
    expect(screen.getByRole('heading', { name: 'Draw your pattern' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '1' })).not.toBeInTheDocument()
    drawPattern(fireEvent, screen.getByTestId('pattern-grid'), [0, 2, 8])
    await waitFor(() => expect(useLockStore.getState().locked).toBe(false))
    expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_UNLOCKED, { method: 'pattern' })
  })

  it('a wrong pattern says so, counts, and stays locked', async () => {
    await configurePattern()
    renderLockScreen()
    drawPattern(fireEvent, screen.getByTestId('pattern-grid'), [6, 7, 8, 5])
    expect(await screen.findByText('Wrong pattern')).toBeInTheDocument()
    expect(readAttempts().failedAttempts).toBe(1)
    expect(useLockStore.getState().locked).toBe(true)
  })

  it('a too-short pattern is not an attempt', async () => {
    await configurePattern()
    renderLockScreen()
    drawPattern(fireEvent, screen.getByTestId('pattern-grid'), [0, 1, 4])
    expect(await screen.findByText('Connect at least 4 dots')).toBeInTheDocument()
    expect(readAttempts().failedAttempts).toBe(0)
  })

  it('locks the grid during a lockout, with the countdown and the attempts left', async () => {
    await configurePattern()
    seedFailures(5)
    recordFailedAttempt()
    renderLockScreen()
    expect(screen.getByTestId('lockout')).toHaveTextContent(/Try again in (1:00|0:59)/)
    expect(screen.getByText('4 attempts left')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Dot 1' })).toBeDisabled()
  })

  it('keeps the biometric button', async () => {
    const user = userEvent.setup()
    await configurePattern({ biometric: { credentialId: 'AAAA' } })
    vi.mocked(verifyBiometric).mockResolvedValue('failed')
    renderLockScreen()
    await user.click(screen.getByRole('button', { name: 'Unlock with biometrics' }))
    expect(await screen.findByText("Couldn't verify it's you. Draw your pattern.")).toBeInTheDocument()
  })

  it('Forgot pattern signs out to reset the lock', async () => {
    const user = userEvent.setup()
    await configurePattern()
    const router = renderLockScreen()
    await user.click(screen.getByRole('button', { name: 'Forgot pattern?' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Log out to reset the pattern?')).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Log out' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/logout'))
    expect(router.state.location.state).toEqual({ keepPendingWrites: true, resetAppLock: true })
    expect(trackEvent).toHaveBeenCalledWith(METRICS.APP_LOCK_FORGOT_SECRET, { kind: 'pattern' })
  })
})
