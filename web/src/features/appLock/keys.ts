// Kept import-free: lib/storage.ts needs these at sign-in, and the rest of the
// feature imports lib/storage.ts.
export const APP_LOCK_CONFIG_KEY = 'econumo.appLock'
export const APP_LOCK_ATTEMPTS_KEY = 'econumo.appLock.attempts'
// sessionStorage: set by every sign-in so the reload that follows a password
// login does not ask for the PIN that was just made unnecessary.
export const APP_LOCK_FRESH_KEY = 'econumo.appLock.fresh'
// sessionStorage: when this tab was last unlocked and on screen, so a reload
// within "Lock after" does not ask again. Per tab: a new tab or a fresh
// launch has none and starts locked.
export const APP_LOCK_SEEN_KEY = 'econumo.appLock.seen'
