import { Navigate, Outlet, useLocation } from 'react-router'
import { rememberPostLoginRedirect } from '@/features/authserver/postLoginRedirect'
import { getToken } from '@/lib/storage'

// Opaque access tokens carry no client-readable expiry: presence gates the
// route, and server-side expiry surfaces as a 401 that the api client
// interceptor turns into the /login?reason=expired redirect.
export function RequireAuth() {
  const location = useLocation()
  if (!getToken()) {
    // An app's authorization request must survive the sign-in; the helper
    // accepts only the consent route and ignores every other path.
    rememberPostLoginRedirect(location.pathname + location.search)
    return <Navigate to="/login" replace />
  }
  return <Outlet />
}
