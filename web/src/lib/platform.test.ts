import { afterEach, expect, it } from 'vitest'
import { isIOS, isNativeApp, nativePlugin } from './platform'

afterEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
})

it('is not native without the Capacitor global', () => {
  expect(isNativeApp()).toBe(false)
  expect(nativePlugin('Preferences')).toBeNull()
})

it('is native when Capacitor reports a native platform', () => {
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Preferences: { get: () => {} } } }
  expect(isNativeApp()).toBe(true)
  expect(nativePlugin('Preferences')).not.toBeNull()
  expect(nativePlugin('Missing')).toBeNull()
})

it('is not native for the Capacitor web runtime', () => {
  window.Capacitor = { isNativePlatform: () => false, Plugins: {} }
  expect(isNativeApp()).toBe(false)
  expect(nativePlugin('Preferences')).toBeNull()
})

function stubNavigator(overrides: { userAgent?: string; platform?: string; maxTouchPoints?: number }) {
  Object.defineProperty(window.navigator, 'userAgent', { value: overrides.userAgent ?? 'Mozilla/5.0 (X11; Linux x86_64)', configurable: true })
  Object.defineProperty(window.navigator, 'platform', { value: overrides.platform ?? 'Linux x86_64', configurable: true })
  Object.defineProperty(window.navigator, 'maxTouchPoints', { value: overrides.maxTouchPoints ?? 0, configurable: true })
}

it('isIOS matches an iPhone user agent', () => {
  stubNavigator({ userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15' })
  expect(isIOS()).toBe(true)
})

it('isIOS treats a touch-capable "MacIntel" as an iPad', () => {
  stubNavigator({ userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15', platform: 'MacIntel', maxTouchPoints: 5 })
  expect(isIOS()).toBe(true)
})

it('isIOS is false on a desktop Mac and on Linux', () => {
  stubNavigator({ userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15', platform: 'MacIntel', maxTouchPoints: 0 })
  expect(isIOS()).toBe(false)
  stubNavigator({})
  expect(isIOS()).toBe(false)
})
