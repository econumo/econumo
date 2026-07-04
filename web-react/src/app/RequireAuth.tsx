import { Navigate, Outlet } from 'react-router'
import { getToken, isTokenExpired, removeToken } from '@/lib/storage'

export function RequireAuth() {
  const token = getToken()
  if (!token) {
    return <Navigate to="/login" replace />
  }
  if (isTokenExpired(token)) {
    removeToken()
    return <Navigate to="/login?reason=expired" replace />
  }
  return <Outlet />
}
