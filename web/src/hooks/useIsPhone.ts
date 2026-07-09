import { useEffect, useState } from 'react'

// Dialogs render as full-screen / bottom-sheet only below Tailwind's `sm`
// breakpoint (640px); at sm and up they are centred desktop dialogs. This is a
// deliberately narrower cutoff than useIsMobile (767px, which also gates the
// sidebar and the in-dialog poppers tied to Tailwind `md`) so a 640-767px
// viewport gets desktop dialogs without disturbing the md-based layout.
const QUERY = '(max-width: 639px)'

export function useIsPhone(): boolean {
  const [isPhone, setIsPhone] = useState(() => window.matchMedia(QUERY).matches)
  useEffect(() => {
    const mql = window.matchMedia(QUERY)
    const onChange = (e: MediaQueryListEvent) => setIsPhone(e.matches)
    mql.addEventListener('change', onChange)
    return () => mql.removeEventListener('change', onChange)
  }, [])
  return isPhone
}
