import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors } from '@dnd-kit/core'
import type { DragEndEvent } from '@dnd-kit/core'
import { SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy, arrayMove } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import type { DraggableAttributes } from '@dnd-kit/core'
import type { SyntheticListenerMap } from '@dnd-kit/core/dist/hooks/utilities'

export interface SortableHandleProps {
  attributes: DraggableAttributes | Record<string, never>
  listeners: SyntheticListenerMap | undefined
}

interface SortableRowProps<T extends { id: string }> {
  item: T
  renderItem: (item: T, handleProps: SortableHandleProps) => ReactNode
}

function SortableRow<T extends { id: string }>({ item, renderItem }: SortableRowProps<T>) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: item.id })
  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={isDragging ? 'opacity-60' : undefined}
    >
      {renderItem(item, { attributes, listeners })}
    </li>
  )
}

interface SortableListProps<T extends { id: string }> {
  items: T[]
  onReorder: (orderedIds: string[]) => void
  renderItem: (item: T, handleProps: SortableHandleProps) => ReactNode
  disabled?: boolean
}

// Render order for an in-flight/awaiting-server preview: preview ids first
// (dropping ids that vanished), then any items the preview doesn't know about.
export function orderByPreview<T extends { id: string }>(items: T[], previewIds: string[] | null): T[] {
  if (!previewIds) {
    return items
  }
  const byId = new Map(items.map((i) => [i.id, i]))
  const ordered = previewIds.map((id) => byId.get(id)).filter((i): i is T => i !== undefined)
  const known = new Set(previewIds)
  return [...ordered, ...items.filter((i) => !known.has(i.id))]
}

// Single-container vertical sortable list; dragging starts from the handle
// (spread handleProps.attributes + handleProps.listeners onto the grip).
export function SortableList<T extends { id: string }>({ items, onReorder, renderItem, disabled }: SortableListProps<T>) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  // Keep the dropped order on screen until the parent hands us fresh items
  // (the order mutation echoes the list), so the row never snaps back.
  // Reset on the actual id order, NOT the array identity — parents rebuild
  // the array every render (filter/map), which used to clear the preview
  // immediately and bounce the row to its old slot until the server echoed.
  const [previewIds, setPreviewIds] = useState<string[] | null>(null)
  const idsKey = items.map((i) => i.id).join('|')
  useEffect(() => {
    setPreviewIds(null)
  }, [idsKey])

  const shown = orderByPreview(items, previewIds)

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) {
      return
    }
    const oldIndex = shown.findIndex((i) => i.id === active.id)
    const newIndex = shown.findIndex((i) => i.id === over.id)
    if (oldIndex === -1 || newIndex === -1) {
      return
    }
    const orderedIds = arrayMove(shown, oldIndex, newIndex).map((i) => i.id)
    setPreviewIds(orderedIds)
    onReorder(orderedIds)
  }

  if (disabled) {
    return <ul>{items.map((item) => <li key={item.id}>{renderItem(item, { attributes: {}, listeners: undefined })}</li>)}</ul>
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext items={shown.map((i) => i.id)} strategy={verticalListSortingStrategy}>
        <ul>
          {shown.map((item) => (
            <SortableRow key={item.id} item={item} renderItem={renderItem} />
          ))}
        </ul>
      </SortableContext>
    </DndContext>
  )
}
