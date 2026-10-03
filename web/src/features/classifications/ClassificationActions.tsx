import { useState } from 'react'
import type { ReactNode, SyntheticEvent } from 'react'
import { MoreVertical } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { PromptDialog } from '@/components/PromptDialog'
import type { ClassificationType } from '@/lib/search'
import type { CategoryDto } from '@/api/dto/category'
import type { PayeeDto } from '@/api/dto/payee'
import type { TagDto } from '@/api/dto/tag'
import type { LabelDto } from '@/api/dto/label'
import { useUserData } from '@/features/user/queries'
import type { ClassificationItem } from './ClassificationList'
import { CategoryDialog } from './CategoryDialog'
import { MergeDialog } from './MergeDialog'
import { TagDialog, type TagDialogItem } from './TagDialog'
import { validatePayeeName } from './payeeName'
import {
  useArchiveCategory,
  useArchiveLabel,
  useArchivePayee,
  useArchiveTag,
  useCategories,
  useDeleteCategory,
  useDeleteLabel,
  useDeletePayee,
  useDeleteTag,
  useLabels,
  useMergeCategory,
  useMergeLabel,
  useMergePayee,
  useMergeTag,
  usePayees,
  useTags,
  useUnarchiveCategory,
  useUnarchiveLabel,
  useUnarchivePayee,
  useUnarchiveTag,
  useUpdateCategory,
  useUpdatePayee,
} from './queries'

const stop = (e: SyntheticEvent) => e.stopPropagation()

interface ActionsMenuProps<T extends ClassificationItem> {
  item: T
  candidates: T[]
  deleteTitle: string
  mergeInfo?: string
  showIcon?: boolean
  onEdit: () => void
  onToggleArchive: () => void
  onDelete: () => void
  onMerge: (targetId: string) => void
  /** the edit dialog, owned by the per-type wrapper */
  children: ReactNode
}

function ActionsMenu<T extends ClassificationItem>({
  item,
  candidates,
  deleteTitle,
  mergeInfo,
  showIcon,
  onEdit,
  onToggleArchive,
  onDelete,
  onMerge,
  children,
}: ActionsMenuProps<T>) {
  const { t } = useTranslation()
  const [mergeOpen, setMergeOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={`actions ${item.name}`}
            onClick={stop}
            onPointerDown={stop}
            onKeyDown={stop}
          >
            <MoreVertical className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        {/* portaled content still bubbles React events to the host row — don't reselect it */}
        <DropdownMenuContent align="end" onClick={stop} onKeyDown={stop}>
          <DropdownMenuItem onSelect={onEdit}>{t('common.button.edit.label')}</DropdownMenuItem>
          <DropdownMenuItem onSelect={onToggleArchive}>
            {item.isArchived === 0 ? t('classifications.common.archive.action') : t('classifications.common.unarchive.action')}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={() => setMergeOpen(true)}>{t('classifications.common.merge.action')}</DropdownMenuItem>
          <DropdownMenuItem variant="destructive" onSelect={() => setDeleteOpen(true)}>
            {t('common.button.delete.label')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {/* the dialogs portal too; pointerdown must still reach the document so outside clicks dismiss them */}
      <span className="contents" onClick={stop} onKeyDown={stop}>
        {children}
        <MergeDialog
          open={mergeOpen}
          source={item}
          candidates={candidates}
          warning={t('classifications.common.merge.warning', { name: item.name })}
          info={mergeInfo}
          showIcon={showIcon}
          onClose={() => setMergeOpen(false)}
          onConfirm={(targetId) => {
            onMerge(targetId)
            setMergeOpen(false)
          }}
        />
        <ConfirmDialog
          open={deleteOpen}
          onClose={() => setDeleteOpen(false)}
          onConfirm={() => {
            onDelete()
            setDeleteOpen(false)
          }}
          title={deleteTitle}
          question={item.name}
          confirmLabel={t('common.button.delete.label')}
          cancelLabel={t('common.button.cancel.label')}
          destructive
        />
      </span>
    </>
  )
}

function useIsOwn() {
  const { data: user } = useUserData()
  return (item: { ownerUserId: string }) => !user || item.ownerUserId === user.id
}

function CategoryActions({ item }: { item: CategoryDto }) {
  const { t } = useTranslation()
  const isOwn = useIsOwn()
  const { data: categories = [] } = useCategories()
  const updateCategory = useUpdateCategory()
  const archiveCategory = useArchiveCategory()
  const unarchiveCategory = useUnarchiveCategory()
  const deleteCategory = useDeleteCategory()
  const mergeCategory = useMergeCategory()
  // snapshot on open: the dialog resets its form whenever the category object changes
  const [editing, setEditing] = useState<CategoryDto | null>(null)
  return (
    <ActionsMenu
      item={item}
      // own items only, and same type: income and expense sit in different
      // halves of the budget, so the server refuses to merge across them
      candidates={categories.filter((c) => isOwn(c) && c.type === item.type)}
      deleteTitle={t('classifications.categories.modals.delete.title')}
      mergeInfo={t('classifications.common.merge.envelope_info')}
      showIcon
      onEdit={() => setEditing(item)}
      onToggleArchive={() => (item.isArchived === 0 ? archiveCategory : unarchiveCategory).mutate(item.id)}
      onDelete={() => deleteCategory.mutate(item.id)}
      onMerge={(targetId) => mergeCategory.mutate({ sourceId: item.id, targetId })}
    >
      <CategoryDialog
        open={editing !== null}
        category={editing}
        onClose={() => setEditing(null)}
        onSubmit={(form) => {
          if (editing) {
            updateCategory.mutate({ id: editing.id, name: form.name, icon: form.icon }, { onSuccess: () => setEditing(null) })
          }
        }}
      />
    </ActionsMenu>
  )
}

function PayeeActions({ item }: { item: PayeeDto }) {
  const { t } = useTranslation()
  const isOwn = useIsOwn()
  const { data: payees = [] } = usePayees()
  const updatePayee = useUpdatePayee()
  const archivePayee = useArchivePayee()
  const unarchivePayee = useUnarchivePayee()
  const deletePayee = useDeletePayee()
  const mergePayee = useMergePayee()
  const [editing, setEditing] = useState<PayeeDto | null>(null)
  return (
    <ActionsMenu
      item={item}
      candidates={payees.filter(isOwn)}
      deleteTitle={t('classifications.payees.modals.delete.title')}
      onEdit={() => setEditing(item)}
      onToggleArchive={() => (item.isArchived === 0 ? archivePayee : unarchivePayee).mutate(item.id)}
      onDelete={() => deletePayee.mutate(item.id)}
      onMerge={(targetId) => mergePayee.mutate({ sourceId: item.id, targetId })}
    >
      <PromptDialog
        open={editing !== null}
        onClose={() => setEditing(null)}
        onSubmit={(name) => {
          if (editing) {
            updatePayee.mutate({ id: editing.id, name }, { onSuccess: () => setEditing(null) })
          }
        }}
        title={t('classifications.payees.modals.edit.header')}
        inputLabel={t('classifications.payees.forms.payee.name.label')}
        initialValue={editing?.name ?? ''}
        validate={validatePayeeName(t)}
        submitLabel={t('common.button.update.label')}
        cancelLabel={t('common.button.cancel.label')}
      />
    </ActionsMenu>
  )
}

function TagLabelActions({ kind, item }: { kind: 'tag' | 'label'; item: TagDto | LabelDto }) {
  const { t } = useTranslation()
  const isOwn = useIsOwn()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()
  const archiveTag = useArchiveTag()
  const unarchiveTag = useUnarchiveTag()
  const deleteTag = useDeleteTag()
  const mergeTag = useMergeTag()
  const archiveLabel = useArchiveLabel()
  const unarchiveLabel = useUnarchiveLabel()
  const deleteLabel = useDeleteLabel()
  const mergeLabel = useMergeLabel()
  const [editing, setEditing] = useState<TagDialogItem | null>(null)
  const isTag = kind === 'tag'
  return (
    <ActionsMenu<TagDto | LabelDto>
      item={item}
      // same kind only: tags and labels are different entities with different endpoints
      candidates={(isTag ? tags : labels).filter(isOwn)}
      deleteTitle={isTag ? t('classifications.tags.modals.delete.title') : t('classifications.labels.modals.delete.title')}
      mergeInfo={isTag ? t('classifications.common.merge.envelope_info') : undefined}
      showIcon
      onEdit={() => setEditing({ id: item.id, name: item.name, kind, icon: item.icon })}
      onToggleArchive={() => {
        const archive = isTag ? archiveTag : archiveLabel
        const unarchive = isTag ? unarchiveTag : unarchiveLabel
        ;(item.isArchived === 0 ? archive : unarchive).mutate(item.id)
      }}
      onDelete={() => (isTag ? deleteTag : deleteLabel).mutate(item.id)}
      onMerge={(targetId) => (isTag ? mergeTag : mergeLabel).mutate({ sourceId: item.id, targetId })}
    >
      <TagDialog open={editing !== null} item={editing} onClose={() => setEditing(null)} />
    </ActionsMenu>
  )
}

export function ClassificationActionsMenu({
  type,
  item,
}: {
  type: ClassificationType
  item: CategoryDto | PayeeDto | TagDto | LabelDto
}) {
  switch (type) {
    case 'category':
      return <CategoryActions item={item as CategoryDto} />
    case 'payee':
      return <PayeeActions item={item as PayeeDto} />
    case 'tag':
    case 'label':
      return <TagLabelActions kind={type} item={item as TagDto | LabelDto} />
  }
}
