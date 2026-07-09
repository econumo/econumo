// Port of the Vue getChangedPositions helper: the new position is the
// 0-based index in the reordered id list; only items whose position
// actually changed are reported (empty array = nothing to persist).
export function getChangedPositions(
  current: { id: string; position: number }[],
  orderedIds: string[],
): { id: string; position: number }[] {
  const changes: { id: string; position: number }[] = []
  orderedIds.forEach((id, index) => {
    const item = current.find((c) => c.id === id)
    if (item && item.position !== index) {
      changes.push({ id, position: index })
    }
  })
  return changes
}
