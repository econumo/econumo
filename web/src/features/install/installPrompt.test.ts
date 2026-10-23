import { act, renderHook } from '@testing-library/react'
import { installMode, listenForInstallPrompt, promptInstall, resetInstallStateForTests, useInstallMode } from './installPrompt'

const ua = vi.spyOn(navigator, 'userAgent', 'get')

function firePrompt(outcome: 'accepted' | 'dismissed' = 'accepted') {
  const event = Object.assign(new Event('beforeinstallprompt', { cancelable: true }), {
    prompt: vi.fn().mockResolvedValue(undefined),
    userChoice: Promise.resolve({ outcome }),
  })
  window.dispatchEvent(event)
  return event
}

beforeAll(() => listenForInstallPrompt())

beforeEach(() => {
  resetInstallStateForTests()
  ua.mockReturnValue('Mozilla/5.0 (X11; Linux x86_64) Chrome/140.0')
  delete window.Capacitor
})

it('offers nothing on a browser that cannot install', () => {
  expect(installMode()).toBeNull()
})

it('offers the Share-menu instructions on iOS', () => {
  ua.mockReturnValue('Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) Safari/604.1')
  expect(installMode()).toBe('ios')
})

it.each([
  ['Firefox for Android', 'Mozilla/5.0 (Android 15; Mobile; rv:140.0) Gecko/140.0 Firefox/140.0', 'android_firefox'],
  ['Safari 17+ on macOS', 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15', 'macos_safari'],
  ['Safari 16 on macOS (no Add to Dock)', 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Safari/605.1.15', null],
  ['Chrome on macOS without its prompt yet', 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36', null],
  ['Edge on macOS', 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36 Edg/140.0', null],
  ['Firefox on macOS', 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:140.0) Gecko/20100101 Firefox/140.0', null],
  ['Firefox on Windows', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0', null],
])('%s', (_, agent, expected) => {
  ua.mockReturnValue(agent)
  expect(installMode()).toBe(expected)
})

it('prefers the browser prompt over the menu steps', () => {
  ua.mockReturnValue('Mozilla/5.0 (Android 15; Mobile; rv:140.0) Gecko/140.0 Firefox/140.0')
  firePrompt()
  expect(installMode()).toBe('prompt')
})

it('offers nothing once running as an installed app', () => {
  ua.mockReturnValue('Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) Safari/604.1')
  Object.defineProperty(navigator, 'standalone', { value: true, configurable: true })
  try {
    expect(installMode()).toBeNull()
  } finally {
    Object.defineProperty(navigator, 'standalone', { value: undefined, configurable: true })
  }
})

it('offers nothing inside the native app', () => {
  ua.mockReturnValue('Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)')
  window.Capacitor = { isNativePlatform: () => true }
  expect(installMode()).toBeNull()
})

it('keeps the browser prompt for a later click and uses it once', async () => {
  const { result } = renderHook(() => useInstallMode())
  let event!: ReturnType<typeof firePrompt>
  act(() => {
    event = firePrompt('accepted')
  })
  expect(event.defaultPrevented).toBe(true)
  expect(result.current).toBe('prompt')

  let accepted = false
  await act(async () => {
    accepted = await promptInstall()
  })
  expect(event.prompt).toHaveBeenCalledOnce()
  expect(accepted).toBe(true)
  expect(result.current).toBeNull()
})

it('hides the offer once the app is installed', () => {
  firePrompt()
  window.dispatchEvent(new Event('appinstalled'))
  expect(installMode()).toBeNull()
})
