import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { InstallNotice } from './InstallNotice'

const mockMode = vi.hoisted(() => ({ value: 'ios' as 'ios' | 'prompt' | null }))
const promptInstall = vi.hoisted(() => vi.fn())
vi.mock('./installPrompt', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./installPrompt')>()),
  useInstallMode: () => mockMode.value,
  promptInstall,
}))
const trackEvent = vi.hoisted(() => vi.fn())
vi.mock('@/lib/metrics', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/metrics')>()),
  trackEvent,
}))

beforeEach(() => {
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
  localStorage.removeItem('econumo.install-notice-dismissed')
  mockMode.value = 'ios'
  promptInstall.mockReset().mockResolvedValue(true)
  trackEvent.mockReset()
})

it('renders nothing when the app cannot be installed', () => {
  mockMode.value = null
  render(<InstallNotice />)
  expect(screen.queryByRole('button')).not.toBeInTheDocument()
})

it('opens the Share-menu instructions on iOS', async () => {
  const user = userEvent.setup()
  render(<InstallNotice />)
  await user.click(screen.getByRole('button', { name: 'Install Econumo as an app' }))
  expect(await screen.findByText('Install Econumo')).toBeInTheDocument()
  expect(screen.getByText(/Add to Home Screen/)).toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith('appInstallAppOpen', { source: 'sidebar', mode: 'ios' })
  expect(promptInstall).not.toHaveBeenCalled()
})

it('hands over to the browser prompt where there is one', async () => {
  mockMode.value = 'prompt'
  const user = userEvent.setup()
  render(<InstallNotice />)
  await user.click(screen.getByRole('button', { name: 'Install Econumo as an app' }))
  expect(promptInstall).toHaveBeenCalledOnce()
  await vi.waitFor(() => expect(trackEvent).toHaveBeenCalledWith('appInstallAppAccept', { source: 'sidebar' }))
})

it('stays hidden after a dismissal', async () => {
  const user = userEvent.setup()
  const { unmount } = render(<InstallNotice />)
  await user.click(screen.getByRole('button', { name: 'Dismiss' }))
  expect(screen.queryByRole('button', { name: 'Install Econumo as an app' })).not.toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith('appInstallNoticeDismiss')
  unmount()
  render(<InstallNotice />)
  expect(screen.queryByRole('button', { name: 'Install Econumo as an app' })).not.toBeInTheDocument()
})
