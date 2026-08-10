// The anchor a relative move needs: the id the dragged item now sits after, or
// null when it landed first. The server derives the sort key from this, so the
// client no longer computes or diffs positions.
export function afterIdFromDrop(orderedIds: string[], movedId: string): string | null {
  const i = orderedIds.indexOf(movedId)
  return i > 0 ? orderedIds[i - 1] : null
}

// Apply a relative move to a local list and re-stamp dense positions, so an
// optimistic update matches what the server will send back. Items are assumed to
// be in position order; an afterId that is not in the list appends, mirroring the
// server's degradation rule.
export function applyMove<T extends { id: string; position: number }>(
  items: T[],
  id: string,
  afterId: string | null,
): T[] {
  const moved = items.find((it) => it.id === id)
  if (!moved) {
    return items
  }
  const rest = items.filter((it) => it.id !== id)
  let at = rest.length
  if (afterId === null) {
    at = 0
  } else {
    const anchor = rest.findIndex((it) => it.id === afterId)
    at = anchor >= 0 ? anchor + 1 : rest.length
  }
  const next = [...rest.slice(0, at), moved, ...rest.slice(at)]
  return next.map((it, position) => ({ ...it, position }))
}

// The anchor for a list that mixes kinds holding INDEPENDENT sort-key sequences
// (tags vs labels). Ids outside the moved row's scope are dropped before the
// anchor is read, because the receiving endpoint does not own them: it would
// treat such an anchor as unknown and silently append instead of honouring the
// drop. A dropped-to-front row therefore reports null even when rows of another
// kind sit above it.
export function afterIdInScope(
  orderedIds: string[],
  movedId: string,
  scopeOf: (id: string) => string | undefined,
): string | null {
  const scope = scopeOf(movedId)
  return afterIdFromDrop(
    orderedIds.filter((id) => scopeOf(id) === scope),
    movedId,
  )
}
