// Which sign-in methods the user HAS (auth_password / auth_google /
// auth_apple / auth_sso, 0 or 1 each) — a user-level fact, so it rides the
// batch context alongside $user_id rather than being stamped per event.

import { getItem, removeItem, setItem } from './storage'

const METHODS_KEY = 'authMethods'

// The custom OIDC slot is 'oidc' on the wire; the attribute is spelled 'sso'
// because that is what the deployment-configured provider is called in
// product terms (its display name varies per instance — "Authentik", etc.).
const PROVIDER_ATTRIBUTE: Record<string, string> = {
  google: 'auth_google',
  apple: 'auth_apple',
  oidc: 'auth_sso',
}

export const AUTH_METHOD_ATTRIBUTES = ['auth_password', 'auth_google', 'auth_apple', 'auth_sso'] as const

// Persisted rather than derived from the query cache: the identity list is a
// Settings-only query that never loads on boot, so a cache-derived value would
// be absent for every user who does not open Linked accounts. Written whenever
// the truth is actually known, read on each event.
//
// The two halves arrive from different endpoints at different times
// (get-user-data on boot; get-identity-list only once something asks for it),
// so each writer merges into the stored object instead of replacing it —
// otherwise whichever landed second would erase the other's answer.
function merge(patch: Record<string, 0 | 1>): void {
  const stored = getItem(METHODS_KEY)
  const base = stored && typeof stored === 'object' && !Array.isArray(stored) ? (stored as Record<string, unknown>) : {}
  setItem(METHODS_KEY, { ...base, ...patch })
}

export function rememberHasPassword(hasPassword: boolean): void {
  merge({ auth_password: hasPassword ? 1 : 0 })
}

export function rememberLinkedProviders(providers: readonly string[]): void {
  const linked = new Set(providers)
  const patch: Record<string, 0 | 1> = {}
  for (const [provider, attribute] of Object.entries(PROVIDER_ATTRIBUTE)) {
    patch[attribute] = linked.has(provider) ? 1 : 0
  }
  merge(patch)
}

export function forgetAuthMethods(): void {
  removeItem(METHODS_KEY)
}

// Only the keys actually written are reported: a flag stays absent until its
// source has answered, so an unanswered one never reads as a measured 0.
export function authMethods(): Record<string, 0 | 1> | null {
  const stored = getItem(METHODS_KEY)
  if (!stored || typeof stored !== 'object' || Array.isArray(stored)) {
    return null
  }
  const values = stored as Record<string, unknown>
  const out: Record<string, 0 | 1> = {}
  for (const key of AUTH_METHOD_ATTRIBUTES) {
    if (values[key] === 0 || values[key] === 1) {
      out[key] = values[key] as 0 | 1
    }
  }
  return Object.keys(out).length > 0 ? out : null
}
