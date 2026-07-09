import { useEffect } from 'react'
import { Navigate, Outlet } from 'react-router'
import { getToken, isTokenExpired, removeToken } from '@/lib/storage'

export function RequireAuth() {
  const token = getToken()
  const expired = token !== null && isTokenExpired(token)

  // Purge in an effect, not during render: StrictMode renders twice, and a
  // render-phase removal makes the second pass take the no-token branch,
  // losing the ?reason=expired redirect.
  useEffect(() => {
    if (expired) {
      removeToken()
    }
  }, [expired])

  if (!token) {
    return <Navigate to="/login" replace />
  }
  if (expired) {
    return <Navigate to="/login?reason=expired" replace />
  }
  return <Outlet />
}
