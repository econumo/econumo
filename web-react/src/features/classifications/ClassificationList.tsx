import { useState } from 'react'
import { ArrowDownUp, GripVertical, MoreVertical, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Switch } from '@/components/ui/switch'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { SortDialog } from '@/components/SortDialog'
import { SortableList } from '@/components/SortableList'
import { getChangedPositions } from '@/lib/ordering'
import { RouterPage } from '@/app/router-pages'
import { SettingsShell } from '@/features/settings/SettingsShell'

export interface ClassificationItem {
  id: string
  name: string
  position: number
  isArchived: 0 | 1
  icon?: string
}

interface ClassificationListProps<T extends ClassificationItem> {
  title: string
  heading?: string
  createLabel: string
  deleteTitle: string
  items: T[]
  showIcon?: boolean
  onCreate: () => void
  onEdit: (item: T) => void
  onDelete: (id: string) => void
  onToggleArchive: (item: T) => void
  onOrder: (changes: { id: string; position: number }[]) => void
}

export function ClassificationList<T extends ClassificationItem>({
  title,
  heading,
  createLabel,
  deleteTitle,
  items,
  showIcon,
  onCreate,
  onEdit,
  onDelete,
  onToggleArchive,
  onOrder,
}: ClassificationListProps<T>) {
  const { t } = useTranslation()
  const [sortOpen, setSortOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<T | null>(null)

  const commitOrder = (orderedIds: string[]) => {
    const changes = getChangedPositions(items, orderedIds)
    if (changes.length > 0) {
      onOrder(changes)
    }
  }

  return (
    <SettingsShell
      title={title}
      heading={heading}
      backTo={RouterPage.SETTINGS}
      actions={
        <>
          {items.length > 1 ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="normal-case tracking-normal text-muted-foreground"
              onClick={() => setSortOpen(true)}
            >
              <ArrowDownUp className="size-4 text-econumo-purple" />
              <span className="hidden sm:inline">{t('blocks.list.order_list')}</span>
            </Button>
          ) : null}
          <Button type="button" size="sm" onClick={onCreate}>
            <Plus className="size-4" />
            <span className="hidden sm:inline">{createLabel}</span>
          </Button>
        </>
      }
    >
      {items.length === 0 ? (
        <p className="px-1 py-2 text-sm text-muted-foreground">{t('blocks.list.list_empty')}</p>
      ) : (
        <SortableList
          items={items}
          onReorder={commitOrder}
          renderItem={(item, handle) => (
            <div className="flex items-center gap-2 rounded-md px-1 py-1.5">
              <button
                type="button"
                aria-label={`drag ${item.name}`}
                className="cursor-grab text-muted-foreground"
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
                  <span className="text-xs text-muted-foreground">{t('modules.classifications.categories.pages.settings.archived_item')}</span>
                ) : null}
              </span>
              <Switch
                aria-label={`archive ${item.name}`}
                checked={item.isArchived === 0}
                onCheckedChange={() => onToggleArchive(item)}
              />
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button type="button" variant="ghost" size="icon" aria-label={`actions ${item.name}`}>
                    <MoreVertical className="size-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onSelect={() => onEdit(item)}>{t('elements.button.edit.label')}</DropdownMenuItem>
                  <DropdownMenuItem variant="destructive" onSelect={() => setDeleteTarget(item)}>
                    {t('elements.button.delete.label')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          )}
        />
      )}

      <SortDialog
        open={sortOpen}
        onClose={() => setSortOpen(false)}
        onPick={(direction) => {
          const ordered = [...items].sort((a, b) => (direction === 'asc' ? a.name.localeCompare(b.name) : b.name.localeCompare(a.name)))
          commitOrder(ordered.map((i) => i.id))
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
        confirmLabel={t('elements.button.delete.label')}
        cancelLabel={t('elements.button.cancel.label')}
      />
    </SettingsShell>
  )
}
