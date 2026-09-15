import { handleAppUrl, installDeepLinkHandler } from './deepLinks'
import { useOAuthInFlight } from '@/features/auth/oauthQueries'
import * as routerRef from '@/app/routerRef'

beforeEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
})

afterEach(() => {
  // navigateTo is spied per test; vitest hands back the SAME spy when a module
  // method is already mocked, so without a restore the call counts accumulate.
  vi.restoreAllMocks()
  useOAuthInFlight.setState({ inFlight: false })
})

it('routes handoff, link-handoff, link-error and error urls and closes the browser sheet', () => {
  const close = vi.fn().mockResolvedValue(undefined)
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: { close } } }
  const nav = vi.spyOn(routerRef, 'navigateTo').mockImplementation(() => {})
  handleAppUrl('com.econumo.app://oauth#handoff=abc')
  expect(nav).toHaveBeenLastCalledWith('/oauth/callback#handoff=abc')
  handleAppUrl('com.econumo.app://oauth#linkHandoff=xyz')
  expect(nav).toHaveBeenLastCalledWith('/settings/profile/linked-accounts#linkHandoff=xyz')
  handleAppUrl('com.econumo.app://oauth?linkError=identity_taken')
  expect(nav).toHaveBeenLastCalledWith('/settings/profile/linked-accounts?oauthError=identity_taken')
  handleAppUrl('com.econumo.app://oauth?error=denied')
  expect(nav).toHaveBeenLastCalledWith('/login?oauthError=denied')
  handleAppUrl('https://example.com/other')
  expect(nav).toHaveBeenCalledTimes(4)
  expect(close).toHaveBeenCalledTimes(4)
})

it('routes the verified https app link on any host the same way as the private scheme', () => {
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: { close: vi.fn().mockResolvedValue(undefined) } } }
  const nav = vi.spyOn(routerRef, 'navigateTo').mockImplementation(() => {})
  handleAppUrl('https://app.econumo.com/oauth/app-return#handoff=abc')
  expect(nav).toHaveBeenLastCalledWith('/oauth/callback#handoff=abc')
  handleAppUrl('https://money.example.org/oauth/app-return#linkHandoff=xyz')
  expect(nav).toHaveBeenLastCalledWith('/settings/profile/linked-accounts#linkHandoff=xyz')
  handleAppUrl('https://money.example.org/oauth/app-return?error=denied')
  expect(nav).toHaveBeenLastCalledWith('/login?oauthError=denied')
})

it('ignores the retired econumo scheme', () => {
  const close = vi.fn().mockResolvedValue(undefined)
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: { close } } }
  const nav = vi.spyOn(routerRef, 'navigateTo').mockImplementation(() => {})
  handleAppUrl('econumo://oauth?handoff=abc')
  handleAppUrl('econumo://oauth#handoff=abc')
  expect(nav).not.toHaveBeenCalled()
  expect(close).not.toHaveBeenCalled()
})

it('clears the in-flight flag for every recognised deep link, including an error return', () => {
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: { close: vi.fn().mockResolvedValue(undefined) } } }
  vi.spyOn(routerRef, 'navigateTo').mockImplementation(() => {})
  useOAuthInFlight.getState().set(true)
  handleAppUrl('com.econumo.app://oauth?error=denied')
  expect(useOAuthInFlight.getState().inFlight).toBe(false)
  useOAuthInFlight.getState().set(true)
  handleAppUrl('https://app.econumo.com/oauth/app-return?error=denied')
  expect(useOAuthInFlight.getState().inFlight).toBe(false)
})

it('installs the appUrlOpen listener on the App plugin', () => {
  const addListener = vi.fn()
  window.Capacitor = { isNativePlatform: () => true, Plugins: { App: { addListener } } }
  installDeepLinkHandler()
  expect(addListener).toHaveBeenCalledWith('appUrlOpen', expect.any(Function))
})
