import { useEffect, useState } from 'react'
import { suggestImportColumns } from '@/api/transaction'
import { headerLooksLikeData, type ColumnProfile } from './columns'
import type { Role } from './prepare'

/** As long as the server gives the AI: past this the wizard stops waiting and the steps start unanswered. */
export const COLUMN_GUESS_WAIT_MS = 8000

type Roles = Partial<Record<Role, string>>

/** whether a file's columns may go to the AI at all: not when its header row reads like a transaction */
function canAskColumns(profiles: ColumnProfile[]): boolean {
  return profiles.length > 0 && !headerLooksLikeData(profiles)
}

/** Asks the AI once per loaded file which columns hold what. Answers the roles once the reply
    is in ({} when it failed, came too late or had nothing), and null while waiting or when
    `profiles` is null (nothing to ask: no file, AI off, an Econumo export). Only headers and
    column types are sent, and nothing at all when the header row reads like a transaction. */
export function useColumnGuess(profiles: ColumnProfile[] | null): Roles | null {
  const [reply, setReply] = useState<{ profiles: ColumnProfile[]; roles: Roles } | null>(null)

  useEffect(() => {
    if (profiles === null || !canAskColumns(profiles)) return
    const controller = new AbortController()
    let cancelled = false
    const timer = setTimeout(() => controller.abort(), COLUMN_GUESS_WAIT_MS)
    const columns = profiles.map(({ name, type, negatives }) => ({ name, type, negatives }))
    void suggestImportColumns(columns, controller.signal).then((roles) => {
      clearTimeout(timer)
      if (!cancelled) setReply({ profiles, roles: controller.signal.aborted ? {} : roles })
    })
    return () => {
      cancelled = true
      clearTimeout(timer)
      controller.abort()
    }
  }, [profiles])

  if (profiles === null) return null
  if (!canAskColumns(profiles)) return {}
  return reply?.profiles === profiles ? reply.roles : null
}
