import { createContext, useContext } from 'react'
import type { ReactNode } from 'react'
import { useDroppable } from '@dnd-kit/core'
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { GripVertical } from 'lucide-react'
import type { SortableHandleProps } from '@/components/SortableList'
import { lineControlClass, useLineControls } from './monthLayout'

/** A draggable row: the whole row (and anything unfolded under it) moves with the
 *  drag, the grip in the row's left indent is the only handle. */
export function DragRow({ id, children }: { id: string; children: ReactNode }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id })
  const controls = useLineControls()
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`group/drag relative ${isDragging ? 'z-10 opacity-60' : ''}`}
    >
      <button
        type="button"
        aria-label={`move ${id}`}
        className={`absolute top-3 left-0.5 z-10 cursor-grab touch-none text-muted-foreground ${isDragging ? '' : lineControlClass(controls, 'drag')}`}
        {...attributes}
        {...listeners}
      >
        <GripVertical className="size-4" />
      </button>
      {children}
    </div>
  )
}

// The folder section is a sortable item itself (folder reorder); its grip lives in
// the folder line, so the handle props travel via context.
const FolderHandleContext = createContext<SortableHandleProps | null>(null)

export function FolderGrip({ name }: { name: string }) {
  const handle = useContext(FolderHandleContext)
  const controls = useLineControls()
  if (!handle) {
    return null
  }
  return (
    <button
      type="button"
      aria-label={`move folder ${name}`}
      className={`absolute left-0.5 z-10 cursor-grab touch-none text-muted-foreground ${lineControlClass(controls, 'line')}`}
      {...handle.attributes}
      {...(handle.listeners ?? {})}
    >
      <GripVertical className="size-4" />
    </button>
  )
}

/** A folder's section: sortable among the folders (when it is a real folder) and a
 *  drop target for rows (`bfolder:<id|null>`). */
export function DragFolder({
  sortableId,
  dropId,
  rowIds,
  highlighted = false,
  folderDragging = false,
  children,
}: {
  /** null for a container that is not a folder (No folder) */
  sortableId: string | null
  dropId: string
  rowIds: string[]
  highlighted?: boolean
  /** a folder drag is in flight: row drop zones pause */
  folderDragging?: boolean
  children: ReactNode
}) {
  const sortable = useSortable({ id: sortableId ?? `__container__${dropId}`, disabled: sortableId === null })
  const { setNodeRef: setDroppableRef, isOver } = useDroppable({ id: dropId, disabled: folderDragging })
  return (
    <div
      ref={(el) => {
        sortable.setNodeRef(el)
        setDroppableRef(el)
      }}
      style={{ transform: CSS.Transform.toString(sortable.transform), transition: sortable.transition }}
      className={`${isOver || highlighted ? 'rounded-md ring-2 ring-ring' : ''} ${sortable.isDragging ? 'relative z-10 opacity-60' : ''}`}
    >
      <FolderHandleContext.Provider value={sortableId !== null ? { attributes: sortable.attributes, listeners: sortable.listeners } : null}>
        <SortableContext items={rowIds} strategy={verticalListSortingStrategy}>
          {children}
        </SortableContext>
      </FolderHandleContext.Provider>
    </div>
  )
}
