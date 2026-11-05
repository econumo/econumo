import { createContext, useCallback, useContext, useMemo, useRef, useState } from 'react'
import type { HTMLAttributes, PointerEvent as ReactPointerEvent, ReactNode } from 'react'
import { DndContext, DragOverlay, KeyboardSensor, MouseSensor, PointerSensor, TouchSensor, closestCenter, useSensor, useSensors } from '@dnd-kit/core'
import type { DragEndEvent, DragOverEvent, DragStartEvent } from '@dnd-kit/core'
import { SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import type { DailyListEntry, ViewTransaction } from '@/features/transactions/useAccountTransactions'
import { snapRowToPointer } from '@/lib/dnd'
import { acceptsDrop, entryKey, resolveDrop } from './historyDrop'
import type { HistoryDrop } from './historyDrop'

// A press this long lifts a row: a finger on any screen, a mouse in the phone
// layout. Anything shorter stays a tap (preview) or a scroll.
const HOLD_MS = 500

// The grip's pointer sensor answers a mouse or pen only: a finger lifts rows by
// the long press, and a pointer sensor would turn its scroll swipes into drags.
class GripSensor extends PointerSensor {
  static activators = [
    {
      eventName: 'onPointerDown' as const,
      handler: ({ nativeEvent: event }: ReactPointerEvent) => event.pointerType !== 'touch' && event.isPrimary && event.button === 0,
    },
  ]
}

interface HistoryDragState {
  /** the day header to light up: the drop would move the row to that day */
  targetDay: string | null
  acceptsDrop: (key: string) => boolean
  /** true right after a drop, so the click a touch drag ends with opens nothing */
  justDragged: () => boolean
}

const HistoryDragContext = createContext<HistoryDragState>({ targetDay: null, acceptsDrop: () => true, justDragged: () => false })

function useHistoryDrag(): HistoryDragState {
  return useContext(HistoryDragContext)
}

function groupDayOf(entries: DailyListEntry[], index: number): string | null {
  for (let i = index; i >= 0; i--) {
    const entry = entries[i]
    if (entry.kind === 'separator') return entry.day
  }
  return null
}

export function HistoryDragProvider({ entries, onDrop, renderOverlay, children }: {
  entries: DailyListEntry[]
  onDrop: (tx: ViewTransaction, drop: HistoryDrop, dayChanged: boolean) => void
  renderOverlay: (tx: ViewTransaction) => ReactNode
  children: ReactNode
}) {
  // Which input lifts a row is decided by the listeners each row binds (see
  // SortableHistoryEntry), so every sensor is always on.
  const sensors = useSensors(
    useSensor(GripSensor, { activationConstraint: { distance: 4 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: HOLD_MS, tolerance: 8 } }),
    useSensor(MouseSensor, { activationConstraint: { delay: HOLD_MS, tolerance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )
  const keys = useMemo(() => entries.map(entryKey), [entries])
  const [active, setActive] = useState<ViewTransaction | null>(null)
  const [targetDay, setTargetDay] = useState<string | null>(null)
  const droppedAt = useRef(0)
  const justDragged = useCallback(() => Date.now() - droppedAt.current < 400, [])

  const dropFor = (activeId: string, overId: string | number | undefined) => {
    const overIndex = overId === undefined ? -1 : keys.indexOf(String(overId))
    return resolveDrop(entries, activeId, overIndex)
  }

  const onDragStart = ({ active: a }: DragStartEvent) => {
    const entry = entries[keys.indexOf(String(a.id))]
    if (entry?.kind === 'transaction') {
      setActive(entry.transaction)
      navigator.vibrate?.(10)
    }
  }

  const onDragOver = ({ active: a, over }: DragOverEvent) => {
    const drop = dropFor(String(a.id), over?.id)
    const from = groupDayOf(entries, keys.indexOf(String(a.id)))
    setTargetDay(drop && drop.date !== from ? drop.date : null)
  }

  const finish = () => {
    setActive(null)
    setTargetDay(null)
    droppedAt.current = Date.now()
  }

  const onDragEnd = ({ active: a, over }: DragEndEvent) => {
    const tx = active
    const drop = dropFor(String(a.id), over?.id)
    const from = groupDayOf(entries, keys.indexOf(String(a.id)))
    finish()
    if (tx && drop) {
      onDrop(tx, drop, drop.date !== from)
    }
  }

  const accepts = useCallback((key: string) => acceptsDrop(entries, key), [entries])
  const state = useMemo(() => ({ targetDay, acceptsDrop: accepts, justDragged }), [targetDay, accepts, justDragged])

  return (
    <HistoryDragContext.Provider value={state}>
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragStart={onDragStart}
        onDragOver={onDragOver}
        onDragEnd={onDragEnd}
        onDragCancel={finish}
      >
        <SortableContext items={keys} strategy={verticalListSortingStrategy}>
          {children}
        </SortableContext>
        <DragOverlay modifiers={[snapRowToPointer]}>
          {active ? <div className="rounded-md bg-background shadow-lg ring-1 ring-border">{renderOverlay(active)}</div> : null}
        </DragOverlay>
      </DndContext>
    </HistoryDragContext.Provider>
  )
}

/** One entry of the history list. A header only receives drops. A row lifts
 *  by a long press of a finger anywhere on it (phones and tablets, any
 *  layout); with a mouse it lifts from `handle`, the grip, in the desktop
 *  layout, and by a long press on the row in the phone layout, which has no
 *  grip. */
export function SortableHistoryEntry({ id, draggable, compact, children }: {
  id: string
  draggable: boolean
  compact: boolean
  children: (parts: {
    row: HTMLAttributes<HTMLElement>
    handle: HTMLAttributes<HTMLElement> | null
    isDragging: boolean
    targetDay: string | null
    justDragged: () => boolean
  }) => ReactNode
}) {
  const { targetDay, acceptsDrop: accepts, justDragged } = useHistoryDrag()
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({
    id,
    disabled: { draggable: !draggable, droppable: !accepts(id) },
  })
  let row: HTMLAttributes<HTMLElement> = {}
  let handle: HTMLAttributes<HTMLElement> | null = null
  if (draggable && listeners) {
    const { onTouchStart, onMouseDown, onPointerDown, onKeyDown } = listeners
    row = (compact ? { onTouchStart, onMouseDown } : { onTouchStart }) as unknown as HTMLAttributes<HTMLElement>
    if (!compact) {
      handle = { ...attributes, onPointerDown, onKeyDown, ref: setActivatorNodeRef } as unknown as HTMLAttributes<HTMLElement>
    }
  }
  return (
    <div ref={setNodeRef} style={{ transform: CSS.Transform.toString(transform), transition }} className={isDragging ? 'opacity-40' : undefined}>
      {children({ row, handle, isDragging, targetDay, justDragged })}
    </div>
  )
}
