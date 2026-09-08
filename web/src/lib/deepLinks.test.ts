import { handleAppUrl, installDeepLinkHandler } from './deepLinks'
import * as routerRef from '@/app/routerRef'

beforeEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
})

it('routes handoff, linked and error urls and closes the browser sheet', () => {
  const close = vi.fn().mockResolvedValue(undefined)
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Browser: { close } } }
  const nav = vi.spyOn(routerRef, 'navigateTo').mockImplementation(() => {})
  handleAppUrl('econumo://oauth?handoff=abc')
  expect(nav).toHaveBeenLastCalledWith('/oauth/callback#handoff=abc')
  handleAppUrl('econumo://oauth?linked=google')
  expect(nav).toHaveBeenLastCalledWith('/settings/profile/linked-accounts?linked=google')
  handleAppUrl('econumo://oauth?error=denied')
  expect(nav).toHaveBeenLastCalledWith('/login?oauthError=denied')
  handleAppUrl('https://example.com/other')
  expect(nav).toHaveBeenCalledTimes(3)
  expect(close).toHaveBeenCalledTimes(3)
})

it('installs the appUrlOpen listener on the App plugin', () => {
  const addListener = vi.fn()
  window.Capacitor = { isNativePlatform: () => true, Plugins: { App: { addListener } } }
  installDeepLinkHandler()
  expect(addListener).toHaveBeenCalledWith('appUrlOpen', expect.any(Function))
})
