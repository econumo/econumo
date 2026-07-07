import { useEffect, useState } from 'react'

// The Vue shell switches to single-pane below Quasar's `lg` breakpoint (1024px).
const QUERY = '(max-width: 1023px)'

export function useIsCompact(): boolean {
  const [isCompact, setIsCompact] = useState(() => window.matchMedia(QUERY).matches)
  useEffect(() => {
    const mql = window.matchMedia(QUERY)
    const onChange = (e: MediaQueryListEvent) => setIsCompact(e.matches)
    mql.addEventListener('change', onChange)
    return () => mql.removeEventListener('change', onChange)
  }, [])
  return isCompact
}
