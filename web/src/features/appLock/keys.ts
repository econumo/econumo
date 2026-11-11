// Kept import-free: lib/storage.ts needs these at sign-in, and the rest of the
// feature imports lib/storage.ts.
export const APP_LOCK_CONFIG_KEY = 'econumo.appLock'
export const APP_LOCK_ATTEMPTS_KEY = 'econumo.appLock.attempts'
// sessionStorage: set by every sign-in so the reload that follows a password
// login does not ask for the PIN that was just made unnecessary.
export const APP_LOCK_FRESH_KEY = 'econumo.appLock.fresh'
// A random id per browser, kept across sign-outs: it scopes the passkey's
// user handle to this device, so a synced passkey from another device never
// replaces this one's.
export const APP_LOCK_DEVICE_KEY = 'econumo.appLock.device'
