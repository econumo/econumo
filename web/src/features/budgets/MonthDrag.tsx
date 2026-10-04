import { createContext, useContext } from 'react'
import type { ReactNode } from 'react'
import { useDraggable, useDroppable } from '@dnd-kit/core'
import { SortableContext, useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { GripVertical } from 'lucide-react'
import { EntityIcon } from '@/components/EntityIcon'
import type { SortableHandleProps } from '@/components/SortableList'
import { lineControlClass, useLineControls } from './monthLayout'
import { ENVELOPE_DROP, ENVELOPE_HEAD_DROP, noShift } from './elementMove'

/** The insertion line. Its indent tells the level: from where rows start for a
 *  place in a folder, from where an envelope's categories start for a drop into
 *  that envelope. */
export function DropLine({ level, edge }: { level: 'row' | 'child'; edge: 'before' | 'after' }) {
  return (
    <span
      aria-hidden="true"
      data-testid={`drop-line-${level}`}
      // a ring dot marks where the line starts, so its level reads at a glance
      className={`pointer-events-none absolute right-2 z-20 h-0.5 rounded-full bg-ring before:absolute before:-top-[3px] before:-left-2 before:size-2 before:rounded-full before:border-2 before:border-ring before:bg-background ${level === 'child' ? 'left-14 sm:left-16' : 'left-8'} ${edge === 'before' ? '-top-px' : '-bottom-px'}`}
    />
  )
}

/** what follows the pointer while a row or a category is dragged */
export function DragGhost({ icon, name }: { icon: string; name: string }) {
  return (
    // compact and centred on the cursor: the insertion line sits at a row's edge,
    // clear of it
    <div className="flex h-full items-center">
      <div className="flex min-h-7 items-center gap-1.5 rounded-md bg-background px-2.5 py-0.5 text-sm shadow-md ring-1 ring-border">
        <EntityIcon name={icon} className="text-base text-muted-foreground" />
        <span className="truncate">{name}</span>
      </div>
    </div>
  )
}

/** A draggable row: the grip in the row's left indent is the only handle; while it
 *  is dragged the row stays dimmed in place. `indicator`: the drop lands right
 *  before or after this row. */
export function DragRow({ id, indicator, children }: { id: string; indicator?: 'before' | 'after'; children: ReactNode }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id })
  const controls = useLineControls()
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={`group/drag relative ${isDragging ? 'opacity-40' : ''}`}
    >
      {indicator ? <DropLine level="row" edge={indicator} /> : null}
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
  indicator = false,
  folderDragging = false,
  children,
}: {
  /** null for a container that is not a folder (No folder) */
  sortableId: string | null
  dropId: string
  rowIds: string[]
  /** the drop lands in this folder, whose rows are not shown (empty or folded) */
  indicator?: boolean
  /** a folder drag is in flight: row drop zones pause */
  folderDragging?: boolean
  children: ReactNode
}) {
  // a container that is not a folder is neither dragged nor a drop target of its
  // own: drops land on its `bfolder:` droppable (a bare `disabled: true` still
  // leaves the sortable registered as a drop target)
  const sortable = useSortable({
    id: sortableId ?? `__container__${dropId}`,
    disabled: sortableId === null ? { draggable: true, droppable: true } : false,
  })
  const { setNodeRef: setDroppableRef } = useDroppable({ id: dropId, disabled: folderDragging })
  return (
    <div
      ref={(el) => {
        sortable.setNodeRef(el)
        setDroppableRef(el)
      }}
      style={{ transform: CSS.Transform.toString(sortable.transform), transition: sortable.transition }}
      className={`relative ${sortable.isDragging ? 'z-10 opacity-60' : ''}`}
    >
      {indicator ? <DropLine level="row" edge="after" /> : null}
      <FolderHandleContext.Provider value={sortableId !== null ? { attributes: sortable.attributes, listeners: sortable.listeners } : null}>
        <SortableContext items={rowIds} strategy={noShift}>
          {children}
        </SortableContext>
      </FolderHandleContext.Provider>
    </div>
  )
}

/** A category inside an unfolded envelope: dragged out to a row or folder, or into
 *  another envelope's list. It stays dimmed in place; a floating copy moves. */
export function DragChild({ id, children }: { id: string; children: ReactNode }) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({ id })
  const controls = useLineControls()
  return (
    <div ref={setNodeRef} data-drag-child="" className={`group/child relative ${isDragging ? 'opacity-40' : ''}`}>
      <button
        type="button"
        aria-label={`move ${id}`}
        className={`absolute top-2 left-8 z-10 cursor-grab touch-none text-muted-foreground ${isDragging ? '' : lineControlClass(controls, 'child')}`}
        {...attributes}
        {...listeners}
      >
        <GripVertical className="size-4" />
      </button>
      {children}
    </div>
  )
}

/** An unfolded envelope's category list as a drop target: a category dropped in
 *  it joins the envelope. */
export function EnvelopeDrop({ envelopeId, children }: { envelopeId: string; children: ReactNode }) {
  const { setNodeRef, isOver } = useDroppable({ id: `${ENVELOPE_DROP}${envelopeId}` })
  return (
    <div ref={setNodeRef} data-testid={`envelope-drop-${envelopeId}`} className="relative">
      {children}
      {isOver ? <DropLine level="child" edge="after" /> : null}
    </div>
  )
}

/** A folded envelope takes a category on the middle band of its row; the row's
 *  edges still place the category before or after the envelope. Goes inside the
 *  (relative) row. */
export function EnvelopeHeadDrop({ envelopeId }: { envelopeId: string }) {
  const { setNodeRef, isOver } = useDroppable({ id: `${ENVELOPE_HEAD_DROP}${envelopeId}` })
  return (
    <>
      <span ref={setNodeRef} aria-hidden="true" data-testid={`envelope-head-drop-${envelopeId}`} className="pointer-events-none absolute inset-x-0 top-1/4 bottom-1/4" />
      {isOver ? (
        <>
          <span aria-hidden="true" className="pointer-events-none absolute inset-0 rounded-md ring-2 ring-ring/40" />
          <DropLine level="child" edge="after" />
        </>
      ) : null}
    </>
  )
}
