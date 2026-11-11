import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { act } from '@testing-library/react'
import * as installPrompt from '@/features/install/installPrompt'
import { InstallStep } from './InstallStep'

const IPHONE = 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1'

const DESKTOP = 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36'

function offerInstall() {
  act(() => {
    window.dispatchEvent(new Event('beforeinstallprompt'))
  })
}

beforeAll(() => installPrompt.listenForInstallPrompt())

beforeEach(() => {
  installPrompt.resetInstallStateForTests()
  window.econumoConfig = {}
  window.matchMedia = vi.fn().mockImplementation((q: string) => ({
    matches: false, media: q, addEventListener: vi.fn(), removeEventListener: vi.fn(),
  }))
})
afterEach(() => vi.restoreAllMocks())

it('shows the device steps inline in instruction mode and Done marks the step', async () => {
  vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue(IPHONE)
  const onDone = vi.fn().mockResolvedValue(undefined)
  render(<InstallStep onDone={onDone} onSkip={vi.fn()} />)
  expect(screen.getByText(/Add to Home Screen/)).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: 'Install' })).not.toBeInTheDocument()
  await userEvent.setup().click(screen.getByRole('button', { name: 'Done' }))
  expect(onDone).toHaveBeenCalledTimes(1)
})

it('prompt mode installs through the browser dialog and finishes the step once accepted', async () => {
  vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue(DESKTOP)
  offerInstall()
  const prompt = vi.spyOn(installPrompt, 'promptInstall').mockResolvedValue(true)
  const onDone = vi.fn().mockResolvedValue(undefined)
  render(<InstallStep onDone={onDone} onSkip={vi.fn()} />)
  await userEvent.setup().click(screen.getByRole('button', { name: 'Install' }))
  expect(prompt).toHaveBeenCalledTimes(1)
  expect(onDone).toHaveBeenCalledTimes(1)
})

it('a dismissed install dialog leaves the step open', async () => {
  vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue(DESKTOP)
  offerInstall()
  vi.spyOn(installPrompt, 'promptInstall').mockResolvedValue(false)
  const onDone = vi.fn()
  render(<InstallStep onDone={onDone} onSkip={vi.fn()} />)
  await userEvent.setup().click(screen.getByRole('button', { name: 'Install' }))
  expect(onDone).not.toHaveBeenCalled()
})
