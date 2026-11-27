import { useEffect, useRef } from 'react'
import { useLocation } from 'react-router'
import { logout } from '@/api/user'
import { METRICS, trackEvent } from '@/lib/metrics'
import { hasToken } from '@/lib/storage'
import { RouterPage } from '@/app/router-pages'
import { signOutLocally } from './localSignOut'

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
    const opts = { keepPendingWrites: keepQueue, keepAppLock }
    const run = async () => {
      let logoutUrl = ''
      if (hasToken()) {
        const res = await logout().catch(() => undefined)
        if (res) {
          logoutUrl = res.logoutUrl
        }
        // the tail flush rides the visibilitychange beacon across the redirect
        trackEvent(METRICS.USER_LOGOUT)
        // must run after the logout event above, so the event still carries the outgoing identity
        await signOutLocally(opts)
        // A provider that publishes no end-session endpoint (Google, Apple)
        // yields no url, so the local logout above is the whole exit.
        if (logoutUrl) {
          window.location.assign(logoutUrl)
          return
        }
      } else {
        await signOutLocally(opts)
      }
      window.location.assign(RouterPage.LOGIN)
    }
    void run()
  }, [keepQueue, keepAppLock])

  return null
}
