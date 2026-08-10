import { afterEach, expect, it, vi } from 'vitest'
import { mirrorWrite, restoreNativeStorage } from './appStorage'

function installPrefs(store: Map<string, string>) {
  const prefs = {
    get: vi.fn(async ({ key }: { key: string }) => ({ value: store.get(key) ?? null })),
    set: vi.fn(async ({ key, value }: { key: string; value: string }) => {
      store.set(key, value)
    }),
    remove: vi.fn(async ({ key }: { key: string }) => {
      store.delete(key)
    }),
  }
  window.Capacitor = { isNativePlatform: () => true, Plugins: { Preferences: prefs } }
  return prefs
}

afterEach(() => {
  delete (window as { Capacitor?: unknown }).Capacitor
  localStorage.clear()
})

it('mirrors writes and removals of mirrored keys to Preferences', () => {
  const store = new Map<string, string>()
  const prefs = installPrefs(store)
  mirrorWrite('token', 'eco_ses_abc')
  mirrorWrite('token', null)
  expect(prefs.set).toHaveBeenCalledWith({ key: 'token', value: 'eco_ses_abc' })
  expect(prefs.remove).toHaveBeenCalledWith({ key: 'token' })
})

it('ignores non-mirrored keys and does nothing on the web', () => {
  const store = new Map<string, string>()
  const prefs = installPrefs(store)
  mirrorWrite('locale', 'en')
  expect(prefs.set).not.toHaveBeenCalled()
  delete (window as { Capacitor?: unknown }).Capacitor
  mirrorWrite('token', 't') // must not throw
})

it('restores lost keys from Preferences without clobbering live ones', async () => {
  const store = new Map([
    ['token', 'eco_ses_native'],
    ['backendHost', '"https://my.server.example"'],
  ])
  installPrefs(store)
  localStorage.setItem('token', 'eco_ses_live')
  await restoreNativeStorage()
  expect(localStorage.getItem('token')).toBe('eco_ses_live')
  expect(localStorage.getItem('backendHost')).toBe('"https://my.server.example"')
})
