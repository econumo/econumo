// Which sign-in methods the user HAS (auth_password / auth_google /
// auth_apple / auth_sso, "on" or "off" each) — a user-level fact, so it rides
// the batch context alongside $user_id rather than being stamped per event.
//
// Values are the strings "on"/"off", not 0/1: the collector groups attr_value
// as a categorical label (there is no numeric aggregation over it), and every
// other attribute this project sends is a word — full_access, cloud, desktop.

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

export type AuthMethodFlag = 'on' | 'off'

function flag(present: boolean): AuthMethodFlag {
  return present ? 'on' : 'off'
}

// Persisted rather than derived from the query cache: the identity list is a
// Settings-only query that never loads on boot, so a cache-derived value would
// be absent for every user who does not open Linked accounts. Written whenever
// the truth is actually known, read on each event.
//
// The two halves arrive from different endpoints at different times
// (get-user-data on boot; get-identity-list only once something asks for it),
// so each writer merges into the stored object instead of replacing it —
// otherwise whichever landed second would erase the other's answer.
function merge(patch: Record<string, AuthMethodFlag>): void {
  const stored = getItem(METHODS_KEY)
  const base = stored && typeof stored === 'object' && !Array.isArray(stored) ? (stored as Record<string, unknown>) : {}
  setItem(METHODS_KEY, { ...base, ...patch })
}

export function rememberHasPassword(hasPassword: boolean): void {
  merge({ auth_password: flag(hasPassword) })
}

export function rememberLinkedProviders(providers: readonly string[]): void {
  const linked = new Set(providers)
  const patch: Record<string, AuthMethodFlag> = {}
  for (const [provider, attribute] of Object.entries(PROVIDER_ATTRIBUTE)) {
    patch[attribute] = flag(linked.has(provider))
  }
  merge(patch)
}

export function forgetAuthMethods(): void {
  removeItem(METHODS_KEY)
}

// Only the keys actually written are reported: a flag stays absent until its
// source has answered, so an unanswered one never reads as a measured "off".
export function authMethods(): Record<string, AuthMethodFlag> | null {
  const stored = getItem(METHODS_KEY)
  if (!stored || typeof stored !== 'object' || Array.isArray(stored)) {
    return null
  }
  const values = stored as Record<string, unknown>
  const out: Record<string, AuthMethodFlag> = {}
  for (const key of AUTH_METHOD_ATTRIBUTES) {
    if (values[key] === 'on' || values[key] === 'off') {
      out[key] = values[key]
    }
  }
  return Object.keys(out).length > 0 ? out : null
}
