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
