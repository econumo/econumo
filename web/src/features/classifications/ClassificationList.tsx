import { useRef, useState, type ReactNode } from 'react'
import { ArrowDownUp, GripVertical, MoreVertical, Plus, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Switch } from '@/components/ui/switch'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { InfoBox } from '@/components/InfoBox'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { SortDialog } from '@/components/SortDialog'
import { SortableList, type SortableHandleProps } from '@/components/SortableList'
import { fuzzyMatch } from '@/lib/fuzzy'
import { METRICS, trackEvent } from '@/lib/metrics'
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

export interface RowAction {
  label: string
  destructive?: boolean
  disabled?: boolean
  /** tooltip, e.g. why a disabled action is unavailable */
  title?: string
  onSelect: () => void
}

export interface RowSwitchState {
  checked: boolean
  disabled?: boolean
  title?: string
  ariaLabel: string
  onToggle: () => void
}

interface ClassificationSection<T> {
  label: string
  match: (item: T) => boolean
  /** control rendered on the caption row's right edge (e.g. a bulk action) */
  action?: ReactNode
}

interface ClassificationListProps<T extends ClassificationItem> {
  title: string
  heading?: string
  /** informational hint rendered above the list */
  info?: string
  /** page-level banner (e.g. a server refusal) rendered between info and the list */
  alert?: ReactNode
  createLabel: string
  deleteTitle: string
  items: T[]
  /** localStorage key for the active-only filter; absent = no filter control */
  storageKey?: string
  /** entity name reported with the search analytics event */
  analyticsType: string
  /** optional visual grouping (e.g. category income/expense) */
  sections?: ClassificationSection<T>[]
  showIcon?: boolean
  /** extra muted lines rendered under the name */
  meta?: (item: T) => ReactNode
  /** per-item switch semantics; default = the archive toggle */
  rowSwitch?: (item: T) => RowSwitchState
  /** false suppresses the kebab/tap-sheet for the row; default true */
  hasActions?: (item: T) => boolean
  /** actions inserted between Edit and Delete in the menu and the sheet */
  extraActions?: (item: T) => RowAction[]
  /** when defined for an item, the menu/sheet shows ONLY these actions (no Edit/Delete) */
  rowActions?: (item: T) => RowAction[] | undefined
  onCreate: () => void
  onEdit: (item: T) => void
  onDelete: (id: string) => void
  onToggleArchive?: (item: T) => void
  /** absent = the list is not orderable: no drag grips, no reorder button */
  onOrder?: (changes: { id: string; position: number }[]) => void
}

export function ClassificationList<T extends ClassificationItem>({
  title,
  heading,
  info,
  alert,
  createLabel,
  deleteTitle,
  items,
  storageKey,
  analyticsType,
  sections,
  showIcon,
  meta,
  rowSwitch,
  hasActions,
  extraActions,
  rowActions,
  onCreate,
  onEdit,
  onDelete,
  onToggleArchive,
  onOrder,
}: ClassificationListProps<T>) {
  const { t } = useTranslation()
  const isCompact = useIsCompact()
  const [sortOpen, setSortOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<T | null>(null)
  const [openMenuId, setOpenMenuId] = useState<string | null>(null)
  // compact rows open a bottom-sheet action menu instead of the tiny kebab
  const [sheetItem, setSheetItem] = useState<T | null>(null)
  // No storageKey = no active-only filter: every item stays visible.
  const [activeOnly, setActiveOnly] = useState<boolean>(() =>
    storageKey ? ((getItem(storageKey) as boolean | null) ?? true) : false,
  )
  const [query, setQuery] = useState('')
  // desktop only: the field is collapsed behind the magnifier until asked for
  const [searchOpen, setSearchOpen] = useState(false)
  // one analytics event per visit, not one per keystroke
  const searchTracked = useRef(false)

  const toggleActiveOnly = (value: boolean) => {
    setActiveOnly(value)
    if (storageKey) {
      setItem(storageKey, value)
    }
  }

  const handleSearch = (value: string) => {
    setQuery(value)
    if (value.trim() && !searchTracked.current) {
      searchTracked.current = true
      trackEvent(METRICS.CLASSIFICATION_SEARCH, { type: analyticsType })
    }
  }

  // Items archived from THIS screen stay in place (greyed, switch off) even
  // with the active-only filter on — they disappear only on the next visit.
  const [stickyArchivedIds] = useState(() => new Set<string>())
  const switchFor = (item: T): RowSwitchState => {
    const base: RowSwitchState = rowSwitch?.(item) ?? {
      checked: item.isArchived === 0,
      ariaLabel: `archive ${item.name}`,
      onToggle: () => onToggleArchive?.(item),
    }
    return {
      ...base,
      onToggle: () => {
        if (item.isArchived === 0 && activeOnly) {
          stickyArchivedIds.add(item.id)
        }
        base.onToggle()
      },
    }
  }

  const active = activeOnly ? items.filter((item) => item.isArchived === 0 || stickyArchivedIds.has(item.id)) : items
  const trimmedQuery = query.trim()
  const searching = trimmedQuery.length > 0
  const visible = searching ? active.filter((item) => fuzzyMatch(item.name, trimmedQuery)) : active

  // Every list is group-shaped (tags/payees get one implicit group); captions
  // appear only when more than one group is actually visible.
  const sectionDefs: ClassificationSection<T>[] = sections ?? [{ label: '', match: () => true }]
  const visibleSections = sectionDefs
    .map((section) => ({ label: section.label, action: section.action, items: visible.filter(section.match) }))
    .filter((section) => section.items.length > 0)
  // A lone group needs no caption to disambiguate — but its caption row is
  // also the only mount point for a section action (the currencies bulk
  // enable/disable link), so keep the header when one is present.
  const showGroupHeaders = visibleSections.length > 1 || visibleSections.some((section) => section.action)

  // A drag reorders only the rows on screen (a section, possibly with the
  // archived ones filtered out); rebuild the full id order so every other
  // item keeps its slot before diffing positions.
  const rebuildFullOrder = (subsetIds: string[]): string[] => {
    const subset = new Set(subsetIds)
    const queue = [...subsetIds]
    return items.map((item) => (subset.has(item.id) ? (queue.shift() as string) : item.id))
  }

  const commitOrder = (orderedIds: string[]) => {
    if (!onOrder) {
      return
    }
    const changes = getChangedPositions(items, rebuildFullOrder(orderedIds))
    if (changes.length > 0) {
      onOrder(changes)
    }
  }

  const orderable = onOrder !== undefined && items.length > 1
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

  const searchField = (className: string, autoFocus?: boolean) => (
    <Input
      aria-label={t('common.list.search')}
      placeholder={t('common.list.search')}
      className={`border-0 bg-econumo-card shadow-none ${className}`}
      value={query}
      autoFocus={autoFocus}
      onChange={(e) => handleSearch(e.target.value)}
      // an empty field that lost focus has nothing to show for the space it takes
      onBlur={() => !query.trim() && setSearchOpen(false)}
      onKeyDown={(e) => {
        if (e.key === 'Escape') {
          handleSearch('')
          setSearchOpen(false)
        }
      }}
    />
  )

  const searchToggle = searchOpen ? (
    searchField('w-56', true)
  ) : (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={t('common.list.search')}
      title={t('common.list.search')}
      onClick={() => setSearchOpen(true)}
    >
      <Search className="size-4" />
    </Button>
  )

  const filterControl = storageKey ? (
    <label className="flex cursor-pointer items-center gap-2 text-sm text-muted-foreground" title={t('common.list.active_only')}>
      <Switch aria-label={t('common.list.active_only')} checked={activeOnly} onCheckedChange={toggleActiveOnly} />
      {t('common.list.active_only')}
    </label>
  ) : null

  const renderRow = (item: T, handle?: SortableHandleProps) => {
    const actionable = hasActions?.(item) ?? true
    const rowSwitchState = switchFor(item)
    return (
      <div
        className={`flex items-center rounded-md px-1 ${
          isCompact
            ? `gap-3 py-3 ${actionable ? 'active:bg-accent' : ''}`
            : `gap-2 py-1.5 ${actionable ? 'cursor-pointer hover:bg-accent' : ''}`
        }`}
        onClick={actionable ? () => (isCompact ? setSheetItem(item) : setOpenMenuId(item.id)) : undefined}
      >
        {handle ? (
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
        ) : null}
        {showIcon ? <EntityIcon name={item.icon} className="text-base text-muted-foreground" /> : null}
        <span className="flex min-w-0 flex-1 flex-col">
          <span className={`truncate text-sm ${item.isArchived === 1 ? 'text-muted-foreground' : ''}`} title={item.name}>
            {item.name}
          </span>
          {item.isArchived === 1 ? (
            <span className="text-xs text-muted-foreground">{t('classifications.categories.pages.settings.archived_item')}</span>
          ) : null}
          {meta?.(item)}
        </span>
        <Switch
          aria-label={rowSwitchState.ariaLabel}
          checked={rowSwitchState.checked}
          disabled={rowSwitchState.disabled}
          title={rowSwitchState.title}
          onClick={(e) => e.stopPropagation()}
          onCheckedChange={() => rowSwitchState.onToggle()}
        />
        {actionable && !isCompact ? (
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
              {(() => {
                const only = rowActions?.(item)
                if (only) {
                  return only.map((action) => (
                    <DropdownMenuItem
                      key={action.label}
                      variant={action.destructive ? 'destructive' : undefined}
                      disabled={action.disabled}
                      title={action.title}
                      onSelect={action.onSelect}
                    >
                      {action.label}
                    </DropdownMenuItem>
                  ))
                }
                return (
                  <>
                    <DropdownMenuItem onSelect={() => onEdit(item)}>{t('common.button.edit.label')}</DropdownMenuItem>
                    {extraActions?.(item).map((action) => (
                      <DropdownMenuItem key={action.label} variant={action.destructive ? 'destructive' : undefined} onSelect={action.onSelect}>
                        {action.label}
                      </DropdownMenuItem>
                    ))}
                    <DropdownMenuItem variant="destructive" onSelect={() => setDeleteTarget(item)}>
                      {t('common.button.delete.label')}
                    </DropdownMenuItem>
                  </>
                )
              })()}
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null}
      </div>
    )
  }

  return (
    <SettingsShell
      title={title}
      heading={heading}
      backTo={RouterPage.SETTINGS}
      titleAction={isCompact ? undefined : searchToggle}
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
              {orderable ? reorderButton : null}
              {filterControl}
            </span>
          </>
        )
      }
    >
      {info ? <InfoBox>{info}</InfoBox> : null}
      {alert ?? null}
      {isCompact ? (
        // compact toolbar: full-width search, then reorder on the left and the active-only filter on the right
        <div className="flex flex-col gap-2 pb-1">
          {searchField('')}
          {orderable || filterControl ? (
            <div className="flex items-center justify-between">
              {orderable ? reorderButton : <span />}
              {filterControl}
            </div>
          ) : null}
        </div>
      ) : null}
      {visible.length === 0 ? (
        <p className="px-1 py-2 text-sm text-muted-foreground">
          {searching ? t('common.list.search_empty') : t('common.list.list_empty')}
        </p>
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
                  {section.action ?? null}
                </div>
              ) : null}
              {onOrder ? (
                // reordering a fuzzy-filtered subset is disorienting — handles return when the query clears
                <SortableList items={sectionItems} onReorder={commitOrder} renderItem={(item, handle) => renderRow(item, searching ? undefined : handle)} />
              ) : (
                sectionItems.map((item) => <div key={item.id}>{renderRow(item)}</div>)
              )}
            </div>
          )
        })
      )}

      {/* compact tap-on-row action sheet */}
      <ResponsiveDialog open={sheetItem !== null} onOpenChange={(open) => !open && setSheetItem(null)} title={sheetItem?.name ?? ''}>
        <div className="flex flex-col gap-2 [&_button]:h-11">
          {(() => {
            const only = sheetItem ? rowActions?.(sheetItem) : undefined
            const actionButton = (action: RowAction) => (
              <Button
                key={action.label}
                type="button"
                variant="outline"
                disabled={action.disabled}
                title={action.title}
                className={action.destructive ? 'text-destructive hover:text-destructive' : undefined}
                onClick={() => {
                  action.onSelect()
                  setSheetItem(null)
                }}
              >
                {action.label}
              </Button>
            )
            if (only) {
              return only.map(actionButton)
            }
            return (
              <>
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
                {sheetItem ? extraActions?.(sheetItem).map(actionButton) : null}
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
              </>
            )
          })()}
        </div>
      </ResponsiveDialog>

      {onOrder ? (
        <SortDialog
          open={sortOpen}
          onClose={() => setSortOpen(false)}
          onPick={(direction) => {
            const ordered = [...items].sort((a, b) => (direction === 'asc' ? a.name.localeCompare(b.name) : b.name.localeCompare(a.name)))
            commitOrder(ordered.map((i) => i.id))
            setSortOpen(false)
          }}
        />
      ) : null}

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
