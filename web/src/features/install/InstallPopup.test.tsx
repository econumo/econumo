import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { InstallPopup } from './InstallPopup'

const mockMode = vi.hoisted(() => ({ value: 'ios' as 'ios' | 'prompt' | null }))
vi.mock('./installPrompt', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./installPrompt')>()),
  useInstallMode: () => mockMode.value,
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
  localStorage.removeItem('econumo.install-popup-shown')
  localStorage.removeItem('econumo.install-notice-dismissed')
  mockMode.value = 'ios'
  trackEvent.mockReset()
})

it('opens the install steps once on iOS Safari', async () => {
  const user = userEvent.setup()
  const { unmount } = render(<InstallPopup />)
  expect(await screen.findByText('Install Econumo')).toBeInTheDocument()
  expect(trackEvent).toHaveBeenCalledWith('appInstallAppOpen', { source: 'popup', mode: 'ios' })
  await user.click(screen.getByRole('button', { name: 'Got it' }))
  expect(screen.queryByText('Install Econumo')).not.toBeInTheDocument()
  unmount()
  render(<InstallPopup />)
  expect(screen.queryByText('Install Econumo')).not.toBeInTheDocument()
})

it.each([
  ['the browser has its own install dialog', 'prompt' as const],
  ['the app already runs installed', null],
])('stays closed when %s', (_, mode) => {
  mockMode.value = mode
  render(<InstallPopup />)
  expect(screen.queryByText('Install Econumo')).not.toBeInTheDocument()
  expect(localStorage.getItem('econumo.install-popup-shown')).toBeNull()
})

it('stays closed once the banner was dismissed', () => {
  localStorage.setItem('econumo.install-notice-dismissed', '1')
  render(<InstallPopup />)
  expect(screen.queryByText('Install Econumo')).not.toBeInTheDocument()
})
