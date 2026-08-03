import { afterEach, expect, it, vi } from 'vitest'
import { bootNativeApp, hideSplash, installBackHandler } from './appBoot'

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

it('resolves even when storage restore rejects, so the splash never hangs', async () => {
  window.Capacitor = { isNativePlatform: () => true, Plugins: {} }
  vi.mocked(restoreNativeStorage).mockRejectedValueOnce(new Error('boom'))
  await expect(bootNativeApp()).resolves.toBeUndefined()
  expect(fetchServerConfig).toHaveBeenCalledOnce()
})

it('maps hardware back to history-back, minimizing at the root', () => {
  let handler: (() => void) | undefined
  const minimizeApp = vi.fn(async () => {})
  const addListener = vi.fn((_ev: string, cb: () => void) => {
    handler = cb
  })
  window.Capacitor = {
    isNativePlatform: () => true,
    Plugins: { App: { addListener, minimizeApp } },
  }
  installBackHandler()
  expect(addListener).toHaveBeenCalledWith('backButton', expect.any(Function))

  const back = vi.spyOn(window.history, 'back').mockImplementation(() => {})
  window.history.replaceState(null, '', '/settings')
  handler!()
  expect(back).toHaveBeenCalledOnce()
  expect(minimizeApp).not.toHaveBeenCalled()

  window.history.replaceState(null, '', '/')
  handler!()
  expect(minimizeApp).toHaveBeenCalledOnce()
  back.mockRestore()
  window.history.replaceState(null, '', '/')
})
