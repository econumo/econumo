import { useEffect, useRef } from 'react'
import { suggestImportColumns } from '@/api/transaction'
import type { ColumnProfile } from './columns'
import type { Role } from './prepare'

/** Asks the AI once per loaded file which columns hold what. Pass null to ask
    nothing (no file, the dialog closed, an Econumo export); a reply for a file
    that is no longer loaded is dropped. Only headers and column types are sent. */
export function useColumnGuess(profiles: ColumnProfile[] | null, onGuess: (roles: Partial<Record<Role, string>>) => void) {
  const onGuessRef = useRef(onGuess)
  useEffect(() => {
    onGuessRef.current = onGuess
  })

  useEffect(() => {
    if (profiles === null || profiles.length === 0) return
    const controller = new AbortController()
    const columns = profiles.map(({ name, type, negatives }) => ({ name, type, negatives }))
    void suggestImportColumns(columns, controller.signal).then((roles) => {
      if (!controller.signal.aborted && Object.keys(roles).length > 0) onGuessRef.current(roles)
    })
    return () => controller.abort()
  }, [profiles])
}
