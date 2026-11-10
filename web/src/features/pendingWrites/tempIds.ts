import type { Id } from '@/api/types'

// resolveTempId rewrites only what is queued; a dialog or a confirmation opened
// on the row before its add landed still holds the temporary id. In memory on
// purpose: such a dialog does not outlive the page.
const realIds = new Map<Id, Id>()

export function rememberRealId(tempId: Id, realId: Id): void {
  realIds.set(tempId, realId)
}

/** The id to send for a row an open dialog holds, which may be a temporary one that has landed. */
export function realIdOf(id: Id): Id {
  return realIds.get(id) ?? id
}
