// The server owns this preference; this is a mirror. get-user-data resolves
// after the first page view fires, so without a synchronous local copy an
// opted-out user would still emit one event per page load.
//
// Deliberately NOT cleared on logout: it is a device-level fail-safe, so an
// opted-out user's login-page events do not resume the moment they sign out.
// The next user's data corrects it on arrival.

import { getItem, setItem } from './storage'

const KEY = 'analyticsOptOut'

export function analyticsAllowed(): boolean {
  return getItem(KEY) !== true
}

export function rememberAnalyticsPreference(enabled: boolean): void {
  setItem(KEY, !enabled)
}
