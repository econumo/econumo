import { useEffect } from 'react'
import { logout } from '@/api/user'
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
      }
      removeToken()
      clearPersistedQueryCache()
      window.location.assign('/login')
    }
    void run()
  }, [])
  return null
}
