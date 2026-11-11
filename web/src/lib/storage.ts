import { mirrorWrite } from './appStorage'
import { APP_LOCK_ATTEMPTS_KEY, APP_LOCK_CONFIG_KEY, APP_LOCK_FRESH_KEY } from '@/features/appLock/keys'

const TOKEN_KEY = 'token'
// The user the stored token was issued to. The transaction store's owner is
// checked against it: the clear at sign-in may not have reached a stalled
// IndexedDB, so the store cannot vouch for the session by itself.
const SESSION_USER_KEY = 'sessionUserId'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function hasToken(): boolean {
  return getToken() !== null
}

let signInListener: (() => void) | null = null

// The app lock registers here rather than being imported: it reads this module.
export function setSignInListener(listener: (() => void) | null): void {
  signInListener = listener
}

function lockOwner(): string | null {
  try {
    const owner: unknown = JSON.parse(localStorage.getItem(APP_LOCK_CONFIG_KEY) ?? 'null')?.userId
    return typeof owner === 'string' ? owner : null
  } catch {
    return null
  }
}

// Every sign-in passes here, so this is the one place that knows both who the
// lock belongs to and who just signed in. Signing in proves more than the PIN
// does: the attempt counter starts over and the tab is not locked.
function startAppLockSession(userId: string | undefined): void {
  const owner = lockOwner()
  if (userId && owner !== null && owner !== userId) {
    localStorage.removeItem(APP_LOCK_CONFIG_KEY)
    mirrorWrite(APP_LOCK_CONFIG_KEY, null)
  }
  localStorage.removeItem(APP_LOCK_ATTEMPTS_KEY)
  mirrorWrite(APP_LOCK_ATTEMPTS_KEY, null)
  try {
    sessionStorage.setItem(APP_LOCK_FRESH_KEY, String(Date.now()))
  } catch {
    // Without the marker the reload after a password login asks for the PIN once.
  }
  signInListener?.()
}

export function setToken(token: string, userId?: string): void {
  startAppLockSession(userId)
  localStorage.setItem(TOKEN_KEY, token)
  mirrorWrite(TOKEN_KEY, token)
  if (userId) {
    localStorage.setItem(SESSION_USER_KEY, userId)
  } else {
    localStorage.removeItem(SESSION_USER_KEY)
  }
  mirrorWrite(SESSION_USER_KEY, userId || null)
}

export function removeToken(): void {
  localStorage.removeItem(TOKEN_KEY)
  mirrorWrite(TOKEN_KEY, null)
  localStorage.removeItem(SESSION_USER_KEY)
  mirrorWrite(SESSION_USER_KEY, null)
}

export function getSessionUserId(): string | null {
  return localStorage.getItem(SESSION_USER_KEY)
}

// For a session signed in before the user was kept. Never replaces one: a
// late response from a previous session must not relabel the current one.
export function rememberSessionUser(userId: string): void {
  if (!hasToken() || getSessionUserId() !== null) {
    return
  }
  localStorage.setItem(SESSION_USER_KEY, userId)
  mirrorWrite(SESSION_USER_KEY, userId)
}

export function getItem(key: string): unknown {
  const value = localStorage.getItem(key)
  if (value === null) {
    return null
  }
  try {
    return JSON.parse(value)
  } catch {
    return null
  }
}

export function setItem(key: string, value: unknown): void {
  const encoded = JSON.stringify(value)
  localStorage.setItem(key, encoded)
  mirrorWrite(key, encoded)
}

export function removeItem(key: string): void {
  localStorage.removeItem(key)
  mirrorWrite(key, null)
}
