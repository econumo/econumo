import { Navigate, Outlet } from 'react-router'
import { getToken } from '@/lib/storage'

// Opaque access tokens carry no client-readable expiry: presence gates the
// route, and server-side expiry surfaces as a 401 that the api client
// interceptor turns into the /login?reason=expired redirect.
export function RequireAuth() {
  if (!getToken()) {
    return <Navigate to="/login" replace />
  }
  return <Outlet />
}
