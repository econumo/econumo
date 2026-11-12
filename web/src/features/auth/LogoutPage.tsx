import { useEffect, useRef } from 'react'
import { useLocation } from 'react-router'
import { logout } from '@/api/user'
import { clearPendingWrites } from '@/features/pendingWrites/store'
import { resetAnalyticsIdentity } from '@/lib/analytics'
import { METRICS, trackEvent } from '@/lib/metrics'
import { isNativeApp } from '@/lib/platform'
import { clearPersistedQueryCache } from '@/lib/queryPersist'
import { hasToken, removeToken } from '@/lib/storage'
import { clearConfig } from '@/features/appLock/config'
import { RouterPage } from '@/app/router-pages'

export function LogoutPage() {
  // Latches the effect body to a single run so a re-render cannot slip in a
  // second logout call or an extra redirect.
  const started = useRef(false)
  // A password reset revokes this session and the same user signs back in; a
  // different user's rows are deleted when the queue loads for them.
  const { state } = useLocation()
  const signOutState = state as { keepPendingWrites?: boolean; resetAppLock?: boolean } | null
  const keepQueue = signOutState?.keepPendingWrites === true
  // The same user signing back in after "Set a password" keeps the app lock;
  // "Forgot PIN" and too many wrong PINs keep the queue but reset the lock.
  const keepAppLock = keepQueue && signOutState?.resetAppLock !== true

  useEffect(() => {
    if (started.current) {
      return
    }
    started.current = true
    const signOutLocally = () => {
      removeToken()
      if (!keepAppLock) {
        clearConfig()
      }
    }
    const clearLocalData = () => Promise.all([clearPersistedQueryCache(), keepQueue ? undefined : clearPendingWrites()])
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
        signOutLocally()
        await clearLocalData()
        // The IdP redirect is web-only: an IdP logout page in the app's browser
        // sheet leaves the user with no clean way back. A provider that
        // publishes no end-session endpoint (Google, Apple) yields no url, so
        // the local logout above is the whole exit.
        if (logoutUrl && !isNativeApp()) {
          window.location.assign(logoutUrl)
          return
        }
      } else {
        signOutLocally()
        await clearLocalData()
      }
      window.location.assign(RouterPage.LOGIN)
    }
    void run()
  }, [keepQueue, keepAppLock])

  return null
}
