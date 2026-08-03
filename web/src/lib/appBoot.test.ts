import { afterEach, expect, it, vi } from 'vitest'
import { bootNativeApp, hideSplash } from './appBoot'

vi.mock('./appStorage', () => ({ restoreNativeStorage: vi.fn(async () => {}) }))
vi.mock('./appConfig', () => ({ fetchServerConfig: vi.fn(async () => {}) }))
import { restoreNativeStorage } from './appStorage'
import { fetchServerConfig } from './appConfig'

afterEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
  vi.clearAllMocks()
})

it('is a no-op on the web', async () => {
  await bootNativeApp()
  hideSplash()
  expect(restoreNativeStorage).not.toHaveBeenCalled()
  expect(fetchServerConfig).not.toHaveBeenCalled()
})

it('restores storage then fetches server config in app mode', async () => {
  const hide = vi.fn(async () => {})
  window.Capacitor = { isNativePlatform: () => true, Plugins: { SplashScreen: { hide } } }
  await bootNativeApp()
  expect(restoreNativeStorage).toHaveBeenCalledOnce()
  expect(fetchServerConfig).toHaveBeenCalledOnce()
  hideSplash()
  expect(hide).toHaveBeenCalledOnce()
})
