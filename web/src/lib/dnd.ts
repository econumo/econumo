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

// Budget drags show a floating copy beside the cursor, so the drag re-anchors to
// the pointer vertically as snapRowToPointer does but keeps its sideways move.
export const centerRowOnPointer: Modifier = (args) => ({ ...snapRowToPointer(args), x: args.transform.x })

// The floating copy starts just right of the cursor, wherever the row was grabbed.
export const besidePointer: Modifier = ({ activatorEvent, activeNodeRect, transform }) => {
  if (!activeNodeRect || !activatorEvent || !('clientX' in activatorEvent)) {
    return transform
  }
  return { ...transform, x: transform.x + (activatorEvent as PointerEvent).clientX - activeNodeRect.left + 12 }
}
