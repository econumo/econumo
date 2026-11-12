import { act, renderHook } from '@testing-library/react'
import { setToken } from '@/lib/storage'
import { readAttempts, recordFailedAttempt, writeConfig } from './config'
import { initAppLock, lock, PRIVACY_COVER_ID, resetAppLockForTests, unlock, useAppLock, useLockStore } from './lockStore'
import { signIn, testConfig } from './testUtils'

let visibility: DocumentVisibilityState = 'visible'
const reload = vi.fn()

function setVisibility(state: DocumentVisibilityState): void {
  visibility = state
  document.dispatchEvent(new Event('visibilitychange'))
}

function locked(): boolean {
  return useLockStore.getState().locked
}

function cover(): HTMLElement {
  return document.getElementById(PRIVACY_COVER_ID)!
}

function storageEvent(key: string | null, oldValue: string | null, newValue: string | null): void {
  window.dispatchEvent(new StorageEvent('storage', { key, oldValue, newValue, storageArea: localStorage }))
}

beforeAll(() => {
  Object.defineProperty(document, 'visibilityState', { get: () => visibility, configurable: true })
})

beforeEach(() => {
  vi.useFakeTimers({ now: 1_000_000 })
  localStorage.clear()
  sessionStorage.clear()
  visibility = 'visible'
  reload.mockReset()
  resetAppLockForTests({ reload })
})

afterEach(() => {
  resetAppLockForTests()
  vi.useRealTimers()
})

function configure(lockAfterMs = 60_000): void {
  signIn('u1')
  writeConfig(testConfig({ lockAfterMs }))
}

describe('cold start', () => {
  it('starts locked with a config for the signed-in user', () => {
    configure()
    initAppLock()
    expect(locked()).toBe(true)
  })

  it('starts unlocked without one, or with another user\'s', () => {
    initAppLock()
    expect(locked()).toBe(false)
    resetAppLockForTests({ reload })
    writeConfig(testConfig({ userId: 'u2' }))
    signIn('u1')
    initAppLock()
    expect(locked()).toBe(false)
  })

  it('starts unlocked right after a sign-in, once', () => {
    configure()
    recordFailedAttempt()
    setToken('tok', 'u1')
    initAppLock()
    expect(locked()).toBe(false)
    expect(readAttempts().failedAttempts).toBe(0)
    resetAppLockForTests({ reload })
    initAppLock()
    expect(locked()).toBe(true)
  })

  it('ignores a stale sign-in marker', () => {
    configure()
    sessionStorage.setItem('econumo.appLock.fresh', String(Date.now() - 31_000))
    initAppLock()
    expect(locked()).toBe(true)
  })

  it('the page unloading after a password login keeps the marker for the reload', () => {
    configure(0)
    initAppLock()
    setToken('tok2', 'u1')
    window.dispatchEvent(new Event('pagehide'))
    setVisibility('hidden')
    resetAppLockForTests({ reload })
    initAppLock()
    expect(locked()).toBe(false)
  })

  it('is idempotent', () => {
    configure()
    initAppLock()
    act(() => unlock())
    initAppLock()
    expect(locked()).toBe(false)
    expect(document.querySelectorAll(`#${PRIVACY_COVER_ID}`)).toHaveLength(1)
  })
})

describe('background', () => {
  it('locks after the configured time away', () => {
    configure(60_000)
    initAppLock()
    unlock()
    setVisibility('hidden')
    vi.advanceTimersByTime(59_000)
    setVisibility('visible')
    expect(locked()).toBe(false)
    setVisibility('hidden')
    vi.advanceTimersByTime(60_000)
    setVisibility('visible')
    expect(locked()).toBe(true)
  })

  it('locks on hide when set to immediately', () => {
    configure(0)
    initAppLock()
    unlock()
    setVisibility('hidden')
    expect(locked()).toBe(true)
  })

  it('locks when the clock went back while away', () => {
    configure(60_000)
    initAppLock()
    unlock()
    setVisibility('hidden')
    vi.setSystemTime(Date.now() - 10_000)
    setVisibility('visible')
    expect(locked()).toBe(true)
  })

  it('covers the page while hidden and uncovers it after', () => {
    configure(60_000)
    initAppLock()
    unlock()
    expect(cover().hidden).toBe(true)
    setVisibility('hidden')
    expect(cover().hidden).toBe(false)
    expect(useLockStore.getState().obscured).toBe(true)
    setVisibility('visible')
    expect(cover().hidden).toBe(true)
    expect(useLockStore.getState().obscured).toBe(false)
  })

  it('keeps the cover until the frame after a lock', () => {
    configure(0)
    initAppLock()
    unlock()
    setVisibility('hidden')
    setVisibility('visible')
    expect(cover().hidden).toBe(false)
    vi.advanceTimersByTime(50)
    expect(cover().hidden).toBe(true)
  })

  it('does nothing without a config', () => {
    initAppLock()
    setVisibility('hidden')
    expect(cover().hidden).toBe(true)
    expect(useLockStore.getState().obscured).toBe(false)
  })

  it('handles a bfcache page like a hidden one', () => {
    configure(0)
    initAppLock()
    unlock()
    window.dispatchEvent(new Event('pagehide'))
    expect(locked()).toBe(true)
  })

})

describe('heartbeat', () => {
  it('locks after the device slept', () => {
    configure(60_000)
    initAppLock()
    unlock()
    vi.advanceTimersByTime(15_000 * 8)
    expect(locked()).toBe(false)
    vi.setSystemTime(Date.now() + 60_000)
    vi.advanceTimersByTime(15_000)
    expect(locked()).toBe(true)
  })

  it('needs at least 30 s for an immediate lock', () => {
    configure(0)
    initAppLock()
    unlock()
    vi.setSystemTime(Date.now() + 10_000)
    vi.advanceTimersByTime(15_000)
    expect(locked()).toBe(false)
    vi.setSystemTime(Date.now() + 20_000)
    vi.advanceTimersByTime(15_000)
    expect(locked()).toBe(true)
  })
})

describe('lock and unlock', () => {
  it('lock() is a no-op without a config, unlock() resets attempts', () => {
    initAppLock()
    lock()
    expect(locked()).toBe(false)
    configure()
    lock()
    expect(locked()).toBe(true)
    recordFailedAttempt()
    unlock()
    expect(locked()).toBe(false)
    expect(readAttempts().failedAttempts).toBe(0)
  })

  it('lock() spends a sign-in marker', () => {
    configure()
    initAppLock()
    sessionStorage.setItem('econumo.appLock.fresh', String(Date.now()))
    lock()
    expect(sessionStorage.getItem('econumo.appLock.fresh')).toBeNull()
  })

  it('a sign-in in this tab unlocks it', () => {
    configure()
    initAppLock()
    setToken('tok2', 'u1')
    expect(locked()).toBe(false)
  })

  it('useAppLock exposes the state and the active config', () => {
    configure()
    initAppLock()
    const { result } = renderHook(() => useAppLock())
    expect(result.current.locked).toBe(true)
    expect(result.current.config?.userId).toBe('u1')
    act(() => unlock())
    expect(result.current.locked).toBe(false)
  })
})

describe('other tabs', () => {
  it('a locked tab reloads when the lock is removed elsewhere, never unlocks', () => {
    configure()
    initAppLock()
    const old = localStorage.getItem('econumo.appLock')
    localStorage.removeItem('econumo.appLock')
    storageEvent('econumo.appLock', old, null)
    expect(reload).toHaveBeenCalledOnce()
    expect(locked()).toBe(true)
  })

  it('an unlocked tab takes settings changes as they are', () => {
    configure()
    initAppLock()
    unlock()
    const old = localStorage.getItem('econumo.appLock')
    writeConfig(testConfig({ lockAfterMs: 0 }))
    storageEvent('econumo.appLock', old, localStorage.getItem('econumo.appLock'))
    localStorage.removeItem('econumo.appLock')
    storageEvent('econumo.appLock', old, null)
    expect(reload).not.toHaveBeenCalled()
  })

  it('a locked tab reloads when another tab changes the lock in any way', () => {
    configure()
    initAppLock()
    const old = localStorage.getItem('econumo.appLock')
    writeConfig(testConfig({ secret: { kind: 'pin', salt: 'c2FsdA==', hash: 'bmV3', iterations: 1000, length: 6 } }))
    storageEvent('econumo.appLock', old, localStorage.getItem('econumo.appLock'))
    expect(reload).toHaveBeenCalledOnce()
    expect(locked()).toBe(true)
  })

  it('a tab showing data reloads when the session changes elsewhere', () => {
    configure()
    initAppLock()
    unlock()
    storageEvent('token', 'tok', null)
    expect(reload).toHaveBeenCalledOnce()
  })

  it('without any lock the session events change nothing', () => {
    signIn('u1')
    initAppLock()
    storageEvent('token', 'tok', 'tok2')
    storageEvent('sessionUserId', 'u1', 'u2')
    expect(reload).not.toHaveBeenCalled()
  })

  it('recording the session user is not a change', () => {
    configure()
    initAppLock()
    storageEvent('sessionUserId', null, 'u1')
    expect(reload).not.toHaveBeenCalled()
  })
})
