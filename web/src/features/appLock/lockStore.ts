import { create } from 'zustand'
import { setSignInListener } from '@/lib/storage'
import { activeConfig, readConfig, resetAttempts, type AppLockConfig } from './config'
import { ceremonyInFlight } from './biometric'
import { APP_LOCK_CONFIG_KEY, APP_LOCK_FRESH_KEY } from './keys'

interface AppLockState {
  locked: boolean
  obscured: boolean
}

export const useLockStore = create<AppLockState>()(() => ({ locked: false, obscured: false }))

const HEARTBEAT_MS = 15_000
const MIN_SLEEP_MS = 30_000
const FRESH_SIGN_IN_MS = 2 * 60_000
export const PRIVACY_COVER_ID = 'privacy-cover'

let initialized = false
let hiddenAt: number | null = null
let lastTick = 0
let lockSeen = false
let heartbeat: ReturnType<typeof setInterval> | null = null
let cover: HTMLDivElement | null = null
let reloadPage: () => void = () => window.location.reload()
const removers: Array<() => void> = []

function clearFreshMarker(): void {
  try {
    sessionStorage.removeItem(APP_LOCK_FRESH_KEY)
  } catch {
    // no sessionStorage: there is no marker either
  }
}

// The marker outlives a client-side sign-in in this tab, so it only counts
// right after it was set, and any lock spends it.
function consumeFreshMarker(now: number): boolean {
  let value: string | null = null
  try {
    value = sessionStorage.getItem(APP_LOCK_FRESH_KEY)
  } catch {
    return false
  }
  clearFreshMarker()
  const at = Number(value)
  return value !== null && Number.isFinite(at) && now - at >= 0 && now - at < FRESH_SIGN_IN_MS
}

export function lock(): void {
  clearFreshMarker()
  if (activeConfig() === null) {
    return
  }
  useLockStore.setState({ locked: true })
}

// Every unlock is a success, PIN or biometric, so the attempt counter starts over.
export function unlock(): void {
  resetAttempts()
  lastTick = Date.now()
  useLockStore.setState({ locked: false })
}

function setCover(visible: boolean): void {
  if (!cover) {
    return
  }
  cover.hidden = !visible
  cover.style.display = visible ? 'flex' : 'none'
}

function createCover(): void {
  const existing = document.getElementById(PRIVACY_COVER_ID)
  if (existing instanceof HTMLDivElement) {
    cover = existing
    return
  }
  cover = document.createElement('div')
  cover.id = PRIVACY_COVER_ID
  cover.setAttribute('aria-hidden', 'true')
  Object.assign(cover.style, {
    position: 'fixed',
    inset: '0',
    zIndex: '2147483647',
    background: 'var(--background, #fff)',
    alignItems: 'center',
    justifyContent: 'center',
  })
  const logo = document.createElement('img')
  logo.src = '/icons/android-chrome-192x192.png'
  logo.alt = ''
  logo.width = 96
  logo.height = 96
  cover.appendChild(logo)
  document.body.appendChild(cover)
  setCover(false)
}

// The cover is toggled here, outside React, so it is painted before the OS
// takes its app-switcher snapshot, as far as the platform allows.
function onHide(): void {
  if (hiddenAt !== null) {
    return
  }
  const config = activeConfig()
  // A passkey prompt backgrounds the page on Android; that is not leaving the app.
  if (config === null || ceremonyInFlight()) {
    return
  }
  lockSeen = true
  setCover(true)
  hiddenAt = Date.now()
  if (config.lockAfterMs === 0) {
    lock()
  }
  useLockStore.setState({ obscured: true })
}

function onShow(): void {
  const at = hiddenAt
  hiddenAt = null
  const now = Date.now()
  if (at !== null) {
    const config = activeConfig()
    const away = now - at
    // A clock set back reads as a negative time away: lock rather than trust it.
    if (config !== null && (away < 0 || away >= config.lockAfterMs)) {
      lock()
    }
  }
  lastTick = now
  useLockStore.setState({ obscured: false })
  if (!useLockStore.getState().locked) {
    setCover(false)
    return
  }
  // Locked: keep the cover until the lock screen has replaced the app.
  const raf = window.requestAnimationFrame?.bind(window) ?? ((cb: () => void) => setTimeout(cb, 16))
  raf(() => {
    if (hiddenAt === null) {
      setCover(false)
    }
  })
}

function onVisibilityChange(): void {
  if (document.visibilityState === 'hidden') {
    onHide()
  } else {
    onShow()
  }
}

// A device asleep keeps the page "visible" on some platforms; a long gap
// between ticks is the only sign of it.
function tick(): void {
  const now = Date.now()
  const gap = now - lastTick
  lastTick = now
  if (useLockStore.getState().locked || hiddenAt !== null || ceremonyInFlight(now)) {
    return
  }
  const config = activeConfig()
  if (config === null) {
    return
  }
  lockSeen = true
  if (gap < 0 || gap >= Math.max(config.lockAfterMs, MIN_SLEEP_MS)) {
    lock()
  }
}

function ownerOf(raw: string | null): string | null {
  if (raw === null) {
    return null
  }
  try {
    const owner: unknown = JSON.parse(raw)?.userId
    return typeof owner === 'string' ? owner : null
  } catch {
    return null
  }
}

// Another tab never unlocks this one. When it removes the lock or changes who
// is signed in, a tab that is locked or showing data reloads and boots into
// whatever the new state is (unlocked, locked, or the login page).
function onStorage(event: StorageEvent): void {
  if (event.storageArea !== null && event.storageArea !== localStorage) {
    return
  }
  const locked = useLockStore.getState().locked
  if (event.key === null) {
    if (locked || lockSeen) {
      reloadPage()
    }
    return
  }
  if (event.key === APP_LOCK_CONFIG_KEY) {
    const before = ownerOf(event.oldValue)
    const after = ownerOf(event.newValue)
    if (before !== null || after !== null) {
      lockSeen = true
    }
    if (locked && before !== after) {
      reloadPage()
    }
    return
  }
  const sessionChanged =
    event.key === 'token' || (event.key === 'sessionUserId' && event.oldValue !== null && event.oldValue !== event.newValue)
  if (sessionChanged && (locked || lockSeen || readConfig() !== null)) {
    reloadPage()
  }
}

function listen(target: EventTarget, type: string, handler: (event: Event) => void): void {
  target.addEventListener(type, handler)
  removers.push(() => target.removeEventListener(type, handler))
}

// Runs once, before the first render (main.tsx), so the first frame is
// already the lock screen; nothing reads storage at module load.
export function initAppLock(): void {
  if (initialized) {
    return
  }
  initialized = true
  const now = Date.now()
  const fresh = consumeFreshMarker(now)
  if (fresh) {
    resetAttempts()
  }
  lockSeen = readConfig() !== null
  useLockStore.setState({ locked: !fresh && activeConfig() !== null, obscured: false })
  createCover()
  listen(document, 'visibilitychange', onVisibilityChange)
  listen(window, 'pagehide', onHide)
  listen(window, 'pageshow', (event) => {
    if ((event as PageTransitionEvent).persisted) {
      onShow()
    }
  })
  listen(window, 'storage', (event) => onStorage(event as StorageEvent))
  setSignInListener(unlock)
  lastTick = now
  heartbeat = setInterval(tick, HEARTBEAT_MS)
}

export function useAppLock(): { locked: boolean; obscured: boolean; config: AppLockConfig | null } {
  const locked = useLockStore((state) => state.locked)
  const obscured = useLockStore((state) => state.obscured)
  return { locked, obscured, config: activeConfig() }
}

export function resetAppLockForTests(options: { reload?: () => void } = {}): void {
  for (const remove of removers.splice(0)) {
    remove()
  }
  if (heartbeat !== null) {
    clearInterval(heartbeat)
    heartbeat = null
  }
  cover?.remove()
  cover = null
  setSignInListener(null)
  initialized = false
  hiddenAt = null
  lastTick = 0
  lockSeen = false
  reloadPage = options.reload ?? (() => window.location.reload())
  useLockStore.setState({ locked: false, obscured: false })
}
