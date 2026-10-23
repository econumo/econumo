import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { InstallBanner } from './InstallBanner'

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

it('renders nothing when the app cannot be installed or already runs installed', () => {
  mockMode.value = null
  const { container } = render(<InstallBanner />)
  expect(container).toBeEmptyDOMElement()
})

it('opens the Share-menu instructions on iOS', async () => {
  const user = userEvent.setup()
  render(<InstallBanner />)
  expect(screen.getByText(/Add it to your Home Screen, or if you already have, open it from there/)).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'How to install' }))
  expect(await screen.findByText(/Add to Home Screen”/)).toBeInTheDocument()
  expect(screen.getByText(/Already added it\?/)).toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith('appInstallAppOpen', { source: 'banner', mode: 'ios' })
  expect(promptInstall).not.toHaveBeenCalled()
})

it('hands over to the browser prompt where there is one', async () => {
  mockMode.value = 'prompt'
  const user = userEvent.setup()
  render(<InstallBanner />)
  expect(screen.getByText(/Install it to open it from its own window/)).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Install' }))
  expect(promptInstall).toHaveBeenCalledOnce()
  await vi.waitFor(() => expect(trackEvent).toHaveBeenCalledWith('appInstallAppAccept', { source: 'banner' }))
})

it('stays hidden after a dismissal, on every later page', async () => {
  const user = userEvent.setup()
  const { unmount } = render(<InstallBanner />)
  await user.click(screen.getByRole('button', { name: 'Dismiss' }))
  expect(screen.queryByText(/works best as an app/)).not.toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith('appInstallNoticeDismiss')
  unmount()
  render(<InstallBanner />)
  expect(screen.queryByText(/works best as an app/)).not.toBeInTheDocument()
})
