import { useEffect, useRef } from 'react'
import { Outlet, useLocation } from 'react-router'
import { METRICS, trackEvent, trackPage } from '@/lib/metrics'

// The ref dedupe absorbs StrictMode's double-mounted effect (dev sends real
// analytics events) and same-path renavigation.
export function TrackPageViews() {
  const { pathname } = useLocation()
  const lastTracked = useRef<string | null>(null)
  useEffect(() => {
    // A same-commit redirect (<Navigate>, e.g. RequireAuth -> /login) has
    // already moved the real URL past this location; skip the superseded
    // commit so a landing counts one page view, not two.
    if (pathname !== window.location.pathname) {
      return
    }
    if (lastTracked.current === pathname) {
      return
    }
    const entry = lastTracked.current === null
    lastTracked.current = pathname
    trackEvent(METRICS.PAGE_VIEW)
    trackPage(entry)
  }, [pathname])
  return <Outlet />
}
