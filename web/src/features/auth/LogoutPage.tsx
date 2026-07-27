import { useEffect } from 'react'
import { logout } from '@/api/user'
import { METRICS, trackEvent } from '@/lib/metrics'
import { clearPersistedQueryCache } from '@/lib/queryPersist'
import { hasToken, removeToken } from '@/lib/storage'

export function LogoutPage() {
  useEffect(() => {
    const run = async () => {
      if (hasToken()) {
        try {
          await logout()
        } catch {
          // best effort; the token is purged regardless
        }
        // the tail flush rides the visibilitychange beacon across the redirect
        trackEvent(METRICS.USER_LOGOUT)
      }
      removeToken()
      clearPersistedQueryCache()
      window.location.assign('/login')
    }
    void run()
  }, [])
  return null
}
