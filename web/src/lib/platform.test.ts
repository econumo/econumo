import { afterEach, expect, it } from 'vitest'
import { isNativeApp, nativePlugin } from './platform'

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
