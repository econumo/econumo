import { createContext, useCallback, useContext, useMemo, useRef, useState } from 'react'
import type { HTMLAttributes, ReactNode } from 'react'
import { DndContext, DragOverlay, KeyboardSensor, MouseSensor, PointerSensor, TouchSensor, closestCenter, useSensor, useSensors } from '@dnd-kit/core'
import type { DragEndEvent, DragOverEvent, DragStartEvent } from '@dnd-kit/core'
import { SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import type { DailyListEntry, ViewTransaction } from '@/features/transactions/useAccountTransactions'
import { snapRowToPointer } from '@/lib/dnd'
import { entryKey, resolveDrop } from './historyDrop'
import type { HistoryDrop } from './historyDrop'

// A press this long lifts a row on a touch screen; anything shorter stays a
// tap (preview) or a scroll.
const TOUCH_HOLD_MS = 500

interface HistoryDragState {
  /** the day header to light up: the drop would move the row to that day */
  targetDay: string | null
  /** true right after a drop, so the click a touch drag ends with opens nothing */
  justDragged: () => boolean
}

const HistoryDragContext = createContext<HistoryDragState>({ targetDay: null, justDragged: () => false })

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

export function HistoryDragProvider({ entries, touch, onDrop, renderOverlay, children }: {
  entries: DailyListEntry[]
  /** phone layout: rows lift on a long press instead of from the grip */
  touch: boolean
  onDrop: (tx: ViewTransaction, drop: HistoryDrop, dayChanged: boolean) => void
  renderOverlay: (tx: ViewTransaction) => ReactNode
  children: ReactNode
}) {
  const pointer = useSensor(PointerSensor, { activationConstraint: { distance: 4 } })
  const keyboard = useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  const hold = useSensor(TouchSensor, { activationConstraint: { delay: TOUCH_HOLD_MS, tolerance: 8 } })
  const mouseHold = useSensor(MouseSensor, { activationConstraint: { delay: TOUCH_HOLD_MS, tolerance: 8 } })
  // The phone layout lifts the whole row by press-and-hold: a finger through
  // the touch sensor, a mouse (a narrow desktop window) through the mouse one.
  // A pointer sensor there would turn every scroll swipe into a drag. The
  // desktop grip has touch-action: none, so its pointer sensor also serves a
  // finger (an iPad in the desktop layout).
  const sensors = useSensors(touch ? hold : pointer, touch ? mouseHold : keyboard)
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
      if (touch) navigator.vibrate?.(10)
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

  const state = useMemo(() => ({ targetDay, justDragged }), [targetDay, justDragged])

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

/** One entry of the history list. A header only receives drops; a row lifts
 *  from `handle` (desktop grip) or, on touch, from the whole `row`. */
export function SortableHistoryEntry({ id, draggable, touch, children }: {
  id: string
  draggable: boolean
  touch: boolean
  children: (parts: {
    row: HTMLAttributes<HTMLElement>
    handle: HTMLAttributes<HTMLElement> | null
    isDragging: boolean
    targetDay: string | null
    justDragged: () => boolean
  }) => ReactNode
}) {
  const { targetDay, justDragged } = useHistoryDrag()
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({
    id,
    disabled: { draggable: !draggable, droppable: false },
  })
  const dragProps = { ...attributes, ...(listeners ?? {}) }
  const handle = draggable && !touch ? { ...dragProps, ref: setActivatorNodeRef } as HTMLAttributes<HTMLElement> : null
  const row = draggable && touch ? dragProps as HTMLAttributes<HTMLElement> : {}
  return (
    <div ref={setNodeRef} style={{ transform: CSS.Transform.toString(transform), transition }} className={isDragging ? 'opacity-40' : undefined}>
      {children({ row, handle, isDragging, targetDay, justDragged })}
    </div>
  )
}
