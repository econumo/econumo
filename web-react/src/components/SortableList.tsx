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

// Single-container vertical sortable list; dragging starts from the handle
// (spread handleProps.attributes + handleProps.listeners onto the grip).
export function SortableList<T extends { id: string }>({ items, onReorder, renderItem, disabled }: SortableListProps<T>) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) {
      return
    }
    const oldIndex = items.findIndex((i) => i.id === active.id)
    const newIndex = items.findIndex((i) => i.id === over.id)
    if (oldIndex === -1 || newIndex === -1) {
      return
    }
    onReorder(arrayMove(items, oldIndex, newIndex).map((i) => i.id))
  }

  if (disabled) {
    return <ul>{items.map((item) => <li key={item.id}>{renderItem(item, { attributes: {}, listeners: undefined })}</li>)}</ul>
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext items={items.map((i) => i.id)} strategy={verticalListSortingStrategy}>
        <ul>
          {items.map((item) => (
            <SortableRow key={item.id} item={item} renderItem={renderItem} />
          ))}
        </ul>
      </SortableContext>
    </DndContext>
  )
}
