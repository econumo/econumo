import type { Modifier } from '@dnd-kit/core'

// Grabbing a row can collapse other rows (folders hide accounts, budget
// elements hide nested categories), so the grabbed node's measured rect jumps
// and no longer sits under the cursor. Re-anchor the drag to the pointer (row
// centered on the cursor) so both the overlay and the collision rect follow
// the mouse, not the stale rect; x stays pinned — rows only move vertically.
export const snapRowToPointer: Modifier = ({ activatorEvent, draggingNodeRect, transform }) => {
  if (!draggingNodeRect || !activatorEvent || !('clientY' in activatorEvent)) {
    return transform
  }
  const activator = activatorEvent as PointerEvent
  return {
    ...transform,
    x: 0,
    y: transform.y + activator.clientY - draggingNodeRect.top - draggingNodeRect.height / 2,
  }
}
