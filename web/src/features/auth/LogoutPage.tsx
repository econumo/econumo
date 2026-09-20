import { useEffect, useRef } from 'react'
import { logout } from '@/api/user'
import { resetAnalyticsIdentity } from '@/lib/analytics'
import { METRICS, trackEvent } from '@/lib/metrics'
import { isNativeApp } from '@/lib/platform'
import { clearPersistedQueryCache } from '@/lib/queryPersist'
import { hasToken, removeToken } from '@/lib/storage'
import { RouterPage } from '@/app/router-pages'

export function LogoutPage() {
  // Latches the effect body to a single run so a re-render cannot slip in a
  // second logout call or an extra redirect.
  const started = useRef(false)

  useEffect(() => {
    if (started.current) {
      return
    }
    started.current = true
    const run = async () => {
      let logoutUrl = ''
      if (hasToken()) {
        const res = await logout().catch(() => undefined)
        if (res) {
          logoutUrl = res.logoutUrl
        }
        // the tail flush rides the visibilitychange beacon across the redirect
        trackEvent(METRICS.USER_LOGOUT)
        // must run after the logout event above, so it still carries the outgoing identity
        resetAnalyticsIdentity()
        removeToken()
        clearPersistedQueryCache()
        // The IdP redirect is web-only: an IdP logout page in the app's browser
        // sheet leaves the user with no clean way back. A provider that
        // publishes no end-session endpoint (Google, Apple) yields no url, so
        // the local logout above is the whole exit.
        if (logoutUrl && !isNativeApp()) {
          window.location.assign(logoutUrl)
          return
        }
      } else {
        removeToken()
        clearPersistedQueryCache()
      }
      window.location.assign(RouterPage.LOGIN)
    }
    void run()
  }, [])

  return null
}
