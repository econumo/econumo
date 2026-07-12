import { useEffect, useState } from 'react'

const LOGOUT_ESCAPE_DELAY_MS = 3000

// An unreachable backend keeps blocking loaders up forever (queries retry
// endlessly) — once a loader has been visible for a few seconds, surface a
// logout escape hatch so the user isn't trapped behind it.
export function useLogoutEscape(loading: boolean): boolean {
  const [escape, setEscape] = useState(false)
  useEffect(() => {
    if (!loading) {
      setEscape(false)
      return
    }
    const id = setTimeout(() => setEscape(true), LOGOUT_ESCAPE_DELAY_MS)
    return () => clearTimeout(id)
  }, [loading])
  return escape
}
