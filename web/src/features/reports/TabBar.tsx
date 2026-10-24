import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Plus, X } from 'lucide-react'
import { DndContext, PointerSensor, closestCenter, useSensor, useSensors } from '@dnd-kit/core'
import type { DragEndEvent, Modifier } from '@dnd-kit/core'
import { SortableContext, horizontalListSortingStrategy, useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { useIsMobile } from '@/hooks/useIsMobile'
import { useLongPress } from '@/hooks/useLongPress'
import { MAX_REPORT_TABS } from './reportTabs'

const restrictToHorizontalAxis: Modifier = ({ transform }) => ({ ...transform, y: 0 })

export interface TabBarItem {
  id: string
  title: string
}

export interface TabBarProps {
  tabs: TabBarItem[]
  activeId: string
  canAdd: boolean
  onActivate: (id: string) => void
  onNew: () => void
  onClose: (id: string) => void
  onRename: (id: string, title: string) => void
  onDuplicate: (id: string) => void
  onMove: (id: string, toIndex: number) => void
}

function RenameInput({ initial, onSave, onCancel, className, sizerClassName }: {
  initial: string
  onSave: (title: string) => void
  onCancel: () => void
  className: string
  /** the input's box without its colours: an invisible copy of the text sizes the field, so renaming never changes the tab's width */
  sizerClassName: string
}) {
  const { t } = useTranslation()
  const [value, setValue] = useState(initial)
  // Enter/Escape unmount the input, and a blur may follow: only the first outcome counts
  const done = useRef(false)
  const finish = (save: boolean) => {
    if (done.current) return
    done.current = true
    if (save) {
      onSave(value)
    } else {
      onCancel()
    }
  }
  return (
    <span className="inline-grid min-w-0">
      <span aria-hidden="true" className={`invisible col-start-1 row-start-1 whitespace-pre ${sizerClassName}`}>{value || ' '}</span>
      <input
      aria-label={t('reports.tabs.name_label')}
      className={`col-start-1 row-start-1 w-full min-w-0 ${className}`}
      value={value}
      maxLength={64}
      autoFocus
      onFocus={(e) => e.currentTarget.select()}
      onChange={(e) => setValue(e.target.value)}
      onBlur={() => finish(true)}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          e.preventDefault()
          finish(true)
        } else if (e.key === 'Escape') {
          e.preventDefault()
          e.stopPropagation()
          finish(false)
        }
      }}
    />
    </span>
  )
}

function DesktopTab({ tab, active, editing, onActivate, onStartRename, onRename, onCancelRename, onClose }: {
  tab: TabBarItem
  active: boolean
  editing: boolean
  onActivate: () => void
  onStartRename: () => void
  onRename: (title: string) => void
  onCancelRename: () => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: tab.id, disabled: editing })
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Translate.toString(transform), transition }}
      {...attributes}
      {...listeners}
      role="presentation"
      tabIndex={undefined}
      // underlined like the Budget / Plan switch: the selected tab carries the rule
      className={`group -mb-px flex max-w-56 shrink-0 items-center gap-1 border-b-2 ${
        active ? 'border-foreground text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'
      } ${isDragging ? 'z-10 opacity-80' : ''}`}
    >
      {editing ? (
        <RenameInput
          initial={tab.title}
          onSave={onRename}
          onCancel={onCancelRename}
          className="bg-transparent py-2.5 text-sm outline-none"
          sizerClassName="py-2.5 text-sm"
        />
      ) : (
        <button
          type="button"
          role="tab"
          aria-selected={active}
          title={tab.title}
          className="min-w-0 truncate py-2.5 text-sm outline-none focus-visible:underline"
          onClick={onActivate}
          onDoubleClick={onStartRename}
        >
          {tab.title}
        </button>
      )}
      <button
        type="button"
        aria-label={t('reports.tabs.close', { name: tab.title })}
        title={t('reports.tabs.close_menu')}
        className="-mr-1 grid size-5 shrink-0 place-items-center rounded-full text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100 hover:text-foreground focus-visible:opacity-100"
        onPointerDown={(e) => e.stopPropagation()}
        onClick={onClose}
      >
        <X className="size-3" />
      </button>
    </div>
  )
}

function DesktopStrip(props: TabBarProps) {
  const { t } = useTranslation()
  const [editingId, setEditingId] = useState<string | null>(null)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))
  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return
    const to = props.tabs.findIndex((tab) => tab.id === over.id)
    if (to !== -1) props.onMove(String(active.id), to)
  }
  return (
    <div className="flex items-center gap-4 overflow-x-auto border-b scrollbar-none">
      <DndContext sensors={sensors} collisionDetection={closestCenter} modifiers={[restrictToHorizontalAxis]} onDragEnd={onDragEnd}>
        <SortableContext items={props.tabs.map((tab) => tab.id)} strategy={horizontalListSortingStrategy}>
          <div role="tablist" aria-label={t('reports.page.title')} className="flex items-end gap-5">
            {props.tabs.map((tab) => (
              <DesktopTab
                key={tab.id}
                tab={tab}
                active={tab.id === props.activeId}
                editing={editingId === tab.id}
                onActivate={() => props.onActivate(tab.id)}
                onStartRename={() => setEditingId(tab.id)}
                onRename={(title) => {
                  setEditingId(null)
                  if (title.trim() !== tab.title) props.onRename(tab.id, title)
                }}
                onCancelRename={() => setEditingId(null)}
                onClose={() => props.onClose(tab.id)}
              />
            ))}
          </div>
        </SortableContext>
      </DndContext>
      <NewTabButton canAdd={props.canAdd} onNew={props.onNew}
        className="grid size-7 shrink-0 place-items-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-40 disabled:hover:bg-transparent" />
    </div>
  )
}

function NewTabButton({ canAdd, onNew, className }: { canAdd: boolean; onNew: () => void; className: string }) {
  const { t } = useTranslation()
  return (
    <button
      type="button"
      aria-label={t('reports.tabs.new')}
      title={canAdd ? t('reports.tabs.new') : t('reports.tabs.limit', { count: MAX_REPORT_TABS })}
      disabled={!canAdd}
      className={className}
      onClick={onNew}
    >
      <Plus className="size-4" />
    </button>
  )
}

function Pill({ tab, active, editing, menuOpen, onActivate, onMenu, onMenuOpenChange, onStartRename, onRename, onCancelRename, onDuplicate, onClose }: {
  tab: TabBarItem
  active: boolean
  editing: boolean
  menuOpen: boolean
  onActivate: () => void
  onMenu: () => void
  onMenuOpenChange: (open: boolean) => void
  onStartRename: () => void
  onRename: (title: string) => void
  onCancelRename: () => void
  onDuplicate: () => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const press = useLongPress({ onClick: onActivate, onLongPress: onMenu })
  const ref = useRef<HTMLSpanElement>(null)
  useEffect(() => {
    if (active) ref.current?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
  }, [active])
  const base = 'shrink-0 rounded-full border px-3 py-1 text-sm'
  return (
    <span ref={ref} className="relative shrink-0">
      {editing ? (
        <RenameInput
          initial={tab.title}
          onSave={onRename}
          onCancel={onCancelRename}
          className={`${base} border-primary bg-transparent outline-none`}
          sizerClassName={base}
        />
      ) : (
        <button
          type="button"
          role="tab"
          aria-selected={active}
          className={`${base} block max-w-48 truncate select-none [-webkit-touch-callout:none] ${
            active ? 'border-primary bg-primary text-primary-foreground' : 'text-muted-foreground'
          }`}
          {...press}
        >
          {tab.title}
        </button>
      )}
      {/* the long-press menu anchors to the pill without taking over its tap */}
      <DropdownMenu open={menuOpen} onOpenChange={onMenuOpenChange}>
        <DropdownMenuTrigger asChild>
          <span aria-hidden="true" tabIndex={-1} className="pointer-events-none absolute inset-0" />
        </DropdownMenuTrigger>
        <DropdownMenuContent onCloseAutoFocus={(e) => e.preventDefault()}>
          <DropdownMenuItem onSelect={onStartRename}>{t('reports.tabs.rename')}</DropdownMenuItem>
          <DropdownMenuItem onSelect={onDuplicate}>{t('reports.tabs.duplicate')}</DropdownMenuItem>
          <DropdownMenuItem onSelect={onClose}>{t('reports.tabs.close_menu')}</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </span>
  )
}

function PhonePills(props: TabBarProps) {
  const { t } = useTranslation()
  const [editingId, setEditingId] = useState<string | null>(null)
  const [menuId, setMenuId] = useState<string | null>(null)
  return (
    <div className="-mx-2.5 flex items-center gap-1.5 overflow-x-auto px-2.5 py-0.5 scrollbar-none">
      <div role="tablist" aria-label={t('reports.page.title')} className="flex items-center gap-1.5">
        {props.tabs.map((tab) => (
          <Pill
            key={tab.id}
            tab={tab}
            active={tab.id === props.activeId}
            editing={editingId === tab.id}
            menuOpen={menuId === tab.id}
            onActivate={() => props.onActivate(tab.id)}
            onMenu={() => setMenuId(tab.id)}
            onMenuOpenChange={(open) => setMenuId(open ? tab.id : null)}
            onStartRename={() => setEditingId(tab.id)}
            onRename={(title) => {
              setEditingId(null)
              if (title.trim() !== tab.title) props.onRename(tab.id, title)
            }}
            onCancelRename={() => setEditingId(null)}
            onDuplicate={() => props.onDuplicate(tab.id)}
            onClose={() => props.onClose(tab.id)}
          />
        ))}
      </div>
      <NewTabButton canAdd={props.canAdd} onNew={props.onNew}
        className="grid h-8 w-10 shrink-0 place-items-center rounded-full border text-muted-foreground disabled:opacity-40" />
    </div>
  )
}

export function TabBar(props: TabBarProps) {
  return useIsMobile() ? <PhonePills {...props} /> : <DesktopStrip {...props} />
}
