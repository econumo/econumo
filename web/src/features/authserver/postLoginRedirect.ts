const KEY = 'econumo.postLoginRedirect'
const MAX_AGE_MS = 10 * 60 * 1000
const CONSENT_PREFIX = '/oauth/authorize?'

// Only the consent route may be remembered: the value ends up in
// window.location.assign, so anything else would be an open redirect.
export function rememberPostLoginRedirect(path: string): void {
  if (!path.startsWith(CONSENT_PREFIX)) {
    return
  }
  try {
    sessionStorage.setItem(KEY, JSON.stringify({ path, at: Date.now() }))
  } catch {
    // storage unavailable: the user lands on the home page after sign-in instead
  }
}

export function takePostLoginRedirect(): string {
  let raw: string | null = null
  try {
    raw = sessionStorage.getItem(KEY)
    sessionStorage.removeItem(KEY)
  } catch {
    return '/'
  }
  if (!raw) {
    return '/'
  }
  try {
    const { path, at } = JSON.parse(raw) as { path?: unknown; at?: unknown }
    if (typeof path === 'string' && typeof at === 'number' && path.startsWith(CONSENT_PREFIX) && Date.now() - at <= MAX_AGE_MS) {
      return path
    }
  } catch {
    // corrupt entry: treated as absent
  }
  return '/'
}

export function clearPostLoginRedirect(): void {
  try {
    sessionStorage.removeItem(KEY)
  } catch {
    // nothing to clear
  }
}
