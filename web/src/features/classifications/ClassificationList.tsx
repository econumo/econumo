import { useState } from 'react'
import { ArrowDownUp, GripVertical, MoreVertical, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Switch } from '@/components/ui/switch'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { InfoBox } from '@/components/InfoBox'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { SortDialog } from '@/components/SortDialog'
import { SortableList } from '@/components/SortableList'
import type { SortListForm } from '@/api/category'
import { getChangedPositions } from '@/lib/ordering'
import { getItem, setItem } from '@/lib/storage'
import { useIsCompact } from '@/hooks/useIsCompact'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'

export interface ClassificationItem {
  id: string
  name: string
  position: number
  isArchived: 0 | 1
  icon?: string
}

interface ClassificationSection<T> {
  label: string
  match: (item: T) => boolean
}

interface ClassificationListProps<T extends ClassificationItem> {
  title: string
  heading?: string
  /** informational hint rendered above the list */
  info?: string
  createLabel: string
  deleteTitle: string
  items: T[]
  /** localStorage key for the active-only filter state */
  storageKey: string
  /** optional visual grouping (e.g. category income/expense) */
  sections?: ClassificationSection<T>[]
  showIcon?: boolean
  onCreate: () => void
  onEdit: (item: T) => void
  onDelete: (id: string) => void
  onToggleArchive: (item: T) => void
  onOrder: (changes: { id: string; position: number }[]) => void
  onSort: (form: SortListForm) => void
}

export function ClassificationList<T extends ClassificationItem>({
  title,
  heading,
  info,
  createLabel,
  deleteTitle,
  items,
  storageKey,
  sections,
  showIcon,
  onCreate,
  onEdit,
  onDelete,
  onToggleArchive,
  onOrder,
  onSort,
}: ClassificationListProps<T>) {
  const { t } = useTranslation()
  const isCompact = useIsCompact()
  const [sortOpen, setSortOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<T | null>(null)
  const [openMenuId, setOpenMenuId] = useState<string | null>(null)
  // compact rows open a bottom-sheet action menu instead of the tiny kebab
  const [sheetItem, setSheetItem] = useState<T | null>(null)
  const [activeOnly, setActiveOnly] = useState<boolean>(() => (getItem(storageKey) as boolean | null) ?? true)

  const toggleActiveOnly = (value: boolean) => {
    setActiveOnly(value)
    setItem(storageKey, value)
  }

  // Items archived from THIS screen stay in place (greyed, switch off) even
  // with the active-only filter on — they disappear only on the next visit.
  const [stickyArchivedIds] = useState(() => new Set<string>())
  const handleToggleArchive = (item: T) => {
    if (item.isArchived === 0 && activeOnly) {
      stickyArchivedIds.add(item.id)
    }
    onToggleArchive(item)
  }

  const visible = activeOnly ? items.filter((item) => item.isArchived === 0 || stickyArchivedIds.has(item.id)) : items

  // Every list is group-shaped (tags/payees get one implicit group); captions
  // appear only when more than one group is actually visible.
  const sectionDefs: ClassificationSection<T>[] = sections ?? [{ label: '', match: () => true }]
  const visibleSections = sectionDefs
    .map((section) => ({ label: section.label, items: visible.filter(section.match) }))
    .filter((section) => section.items.length > 0)
  const showGroupHeaders = visibleSections.length > 1

  // A drag reorders only the rows on screen (a section, possibly with the
  // archived ones filtered out); rebuild the full id order so every other
  // item keeps its slot before diffing positions.
  const rebuildFullOrder = (subsetIds: string[]): string[] => {
    const subset = new Set(subsetIds)
    const queue = [...subsetIds]
    return items.map((item) => (subset.has(item.id) ? (queue.shift() as string) : item.id))
  }

  const commitOrder = (orderedIds: string[]) => {
    const changes = getChangedPositions(items, rebuildFullOrder(orderedIds))
    if (changes.length > 0) {
      onOrder(changes)
    }
  }

  const reorderButton = (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="normal-case tracking-normal text-muted-foreground"
      onClick={() => setSortOpen(true)}
    >
      <ArrowDownUp className="size-4 text-econumo-purple" />
      {t('common.list.order_list')}
    </Button>
  )

  const filterControl = (
    <label className="flex cursor-pointer items-center gap-2 text-sm text-muted-foreground" title={t('common.list.active_only')}>
      <Switch aria-label={t('common.list.active_only')} checked={activeOnly} onCheckedChange={toggleActiveOnly} />
      {t('common.list.active_only')}
    </label>
  )

  return (
    <SettingsShell
      title={title}
      heading={heading}
      backTo={RouterPage.SETTINGS}
      actions={
        isCompact ? (
          <Button type="button" size="icon" aria-label={createLabel} title={createLabel} onClick={onCreate}>
            <Plus className="size-4" />
          </Button>
        ) : (
          <>
            <Button type="button" size="sm" onClick={onCreate}>
              <Plus className="size-4" />
              {createLabel}
            </Button>
            <span className="ml-auto flex items-center gap-3">
              {items.length > 1 ? reorderButton : null}
              {filterControl}
            </span>
          </>
        )
      }
    >
      {info ? <InfoBox>{info}</InfoBox> : null}
      {isCompact ? (
        // compact toolbar row: reorder on the left, the active-only filter on the right
        <div className="flex items-center justify-between pb-1">
          {items.length > 1 ? reorderButton : <span />}
          {filterControl}
        </div>
      ) : null}
      {visible.length === 0 ? (
        <p className="px-1 py-2 text-sm text-muted-foreground">{t('common.list.list_empty')}</p>
      ) : (
        visibleSections.map((section) => {
          const sectionItems = section.items
          return (
            <div key={section.label}>
              {/* a single visible group needs no caption */}
              {showGroupHeaders ? (
                <div className="mt-2 mb-1 flex items-center gap-3 px-1 pt-3 first:mt-0">
                  <span className="text-sm font-semibold uppercase tracking-wide">{section.label}</span>
                  <span className="h-px flex-1 bg-border" aria-hidden="true" />
                </div>
              ) : null}
              <SortableList
                items={sectionItems}
                onReorder={commitOrder}
                renderItem={(item, handle) => (
                  <div
                    className={`flex items-center rounded-md px-1 ${
                      isCompact ? 'gap-3 py-3 active:bg-accent' : 'gap-2 py-1.5 cursor-pointer hover:bg-accent'
                    }`}
                    onClick={() => (isCompact ? setSheetItem(item) : setOpenMenuId(item.id))}
                  >
                    <button
                      type="button"
                      aria-label={`drag ${item.name}`}
                      className="cursor-grab touch-none text-muted-foreground"
                      onClick={(e) => e.stopPropagation()}
                      {...handle.attributes}
                      {...(handle.listeners ?? {})}
                    >
                      <GripVertical className="size-4" />
                    </button>
                    {showIcon ? <EntityIcon name={item.icon} className="text-base text-muted-foreground" /> : null}
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span className={`truncate text-sm ${item.isArchived === 1 ? 'text-muted-foreground' : ''}`} title={item.name}>
                        {item.name}
                      </span>
                      {item.isArchived === 1 ? (
                        <span className="text-xs text-muted-foreground">{t('classifications.categories.pages.settings.archived_item')}</span>
                      ) : null}
                    </span>
                    <Switch
                      aria-label={`archive ${item.name}`}
                      checked={item.isArchived === 0}
                      onClick={(e) => e.stopPropagation()}
                      onCheckedChange={() => handleToggleArchive(item)}
                    />
                    {!isCompact ? (
                      <DropdownMenu open={openMenuId === item.id} onOpenChange={(open) => setOpenMenuId(open ? item.id : null)}>
                        <DropdownMenuTrigger asChild>
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon"
                            aria-label={`actions ${item.name}`}
                            onClick={(e) => e.stopPropagation()}
                          >
                            <MoreVertical className="size-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        {/* portaled content still bubbles React clicks to the row — don't reopen the menu */}
                        <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
                          <DropdownMenuItem onSelect={() => onEdit(item)}>{t('common.button.edit.label')}</DropdownMenuItem>
                          <DropdownMenuItem variant="destructive" onSelect={() => setDeleteTarget(item)}>
                            {t('common.button.delete.label')}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    ) : null}
                  </div>
                )}
              />
            </div>
          )
        })
      )}

      {/* compact tap-on-row action sheet */}
      <ResponsiveDialog open={sheetItem !== null} onOpenChange={(open) => !open && setSheetItem(null)} title={sheetItem?.name ?? ''}>
        <div className="flex flex-col gap-2 [&_button]:h-11">
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              if (sheetItem) {
                onEdit(sheetItem)
              }
              setSheetItem(null)
            }}
          >
            {t('common.button.edit.label')}
          </Button>
          <Button
            type="button"
            variant="outline"
            className="text-destructive hover:text-destructive"
            onClick={() => {
              setDeleteTarget(sheetItem)
              setSheetItem(null)
            }}
          >
            {t('common.button.delete.label')}
          </Button>
        </div>
      </ResponsiveDialog>

      <SortDialog
        open={sortOpen}
        onClose={() => setSortOpen(false)}
        onPick={(form) => {
          onSort(form)
          setSortOpen(false)
        }}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) {
            onDelete(deleteTarget.id)
            setDeleteTarget(null)
          }
        }}
        title={deleteTitle}
        question={deleteTarget?.name ?? ''}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </SettingsShell>
  )
}
