// Which provider opened the CURRENT session: a password login, or one of the
// OAuth providers. A user-level fact, so it rides the batch context alongside
// $user_id rather than being stamped per event.
//
// Persisted rather than held in memory: only login-user and exchange-handoff
// name the provider, and neither runs on a reload. get-user-data (the boot
// path) carries no provider field, and get-session-list — which does — is a
// Settings-only query that never loads on boot, so without a local copy every
// reloaded session would report its provider as unknown.

import { getItem, removeItem, setItem } from './storage'

const KEY = 'authProvider'

// The wire sends '' for a password session (see SessionDto.provider); the
// analytics value is spelled out instead, so the attribute never carries an
// empty string that reads as "missing" rather than "password".
export const PASSWORD_PROVIDER = 'password'

export function rememberAuthProvider(provider: string): void {
  setItem(KEY, provider || PASSWORD_PROVIDER)
}

// Unlike the analytics opt-out (a device-level fail-safe that deliberately
// survives), this describes one session and must not outlive it: leaving it
// set would attribute the next person on this device to the previous one's
// provider.
export function forgetAuthProvider(): void {
  removeItem(KEY)
}

export function authProvider(): string | null {
  const value = getItem(KEY)
  return typeof value === 'string' && value !== '' ? value : null
}
