import { useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import type { CategoryDto, CategoryType } from '@/api/dto/category'
import type { TagDto } from '@/api/dto/tag'
import type { Id } from '@/api/types'
import { CategoryDialog } from '@/features/classifications/CategoryDialog'
import { MergeDialog } from '@/features/classifications/MergeDialog'
import {
  useArchiveCategory,
  useArchiveTag,
  useCategories,
  useCreateCategory,
  useDeleteCategory,
  useDeleteTag,
  useMergeCategory,
  useMergeTag,
  useTags,
  useUnarchiveCategory,
  useUnarchiveTag,
} from '@/features/classifications/queries'
import { useUserData } from '@/features/user/queries'
import type { MenuAction } from './monthLayout'
import { useMoveElement } from './queries'

/** a category or tag a budget line stands for */
export interface ClassificationRef {
  kind: 'category' | 'tag'
  id: Id
}

type Pending = { kind: 'category'; item: CategoryDto } | { kind: 'tag'; item: TagDto }

/**
 * The budget's ⋮ menus manage the categories and tags behind their lines the way
 * the Categories and Tags pages do: archive or unarchive, merge, delete, and create
 * a category right where it is wanted. Categories and tags are personal, so only
 * their owner may change them; for anyone else the actions show greyed out.
 */
export function useClassificationMenu(budgetId: Id) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: categories = [] } = useCategories()
  const { data: tags = [] } = useTags()
  const archiveCategory = useArchiveCategory()
  const unarchiveCategory = useUnarchiveCategory()
  const deleteCategory = useDeleteCategory()
  const mergeCategory = useMergeCategory()
  const archiveTag = useArchiveTag()
  const unarchiveTag = useUnarchiveTag()
  const deleteTag = useDeleteTag()
  const mergeTag = useMergeTag()
  const createCategory = useCreateCategory()
  const moveElement = useMoveElement()
  const [merging, setMerging] = useState<Pending | null>(null)
  const [deleting, setDeleting] = useState<Pending | null>(null)
  const [creating, setCreating] = useState<{ type: CategoryType; folderId: Id | null } | null>(null)

  const resolve = (ref: ClassificationRef): Pending | null => {
    if (ref.kind === 'category') {
      const item = categories.find((c) => c.id === ref.id)
      return item ? { kind: 'category', item } : null
    }
    const item = tags.find((c) => c.id === ref.id)
    return item ? { kind: 'tag', item } : null
  }

  const actionsFor = (ref: ClassificationRef): MenuAction[] => {
    const pending = resolve(ref)
    // the lists hold the user's own items and the shared ones; ownership decides
    const own = pending !== null && !!user && pending.item.ownerUserId === user.id
    const reason = own ? undefined : t('budgets.page.plan.menu.no_access')
    const archived = pending?.item.isArchived === 1
    return [
      {
        label: archived ? t('classifications.common.unarchive.action') : t('classifications.common.archive.action'),
        disabled: !own,
        reason,
        onSelect: () => {
          if (!pending) {
            return
          }
          if (pending.kind === 'category') {
            ;(archived ? unarchiveCategory : archiveCategory).mutate(pending.item.id)
          } else {
            ;(archived ? unarchiveTag : archiveTag).mutate(pending.item.id)
          }
        },
      },
      { label: t('classifications.common.merge.action'), disabled: !own, reason, onSelect: () => setMerging(pending) },
      { label: t('common.button.delete.label'), destructive: true, disabled: !own, reason, onSelect: () => setDeleting(pending) },
    ]
  }

  /** New category: lands in the folder (or No folder) the menu belongs to */
  const newCategoryAction = (type: CategoryType, folderId: Id | null): MenuAction => ({
    label: t('classifications.categories.modals.create.header'),
    onSelect: () => setCreating({ type, folderId }),
  })

  const mergeCandidates = (): (CategoryDto | TagDto)[] => {
    if (!merging || !user) {
      return []
    }
    if (merging.kind === 'category') {
      // the server refuses to merge across income and expense
      return categories.filter((c) => c.ownerUserId === user.id && c.type === merging.item.type && c.id !== merging.item.id)
    }
    return tags.filter((c) => c.ownerUserId === user.id && c.id !== merging.item.id)
  }

  const dialogs: ReactNode = (
    <>
      <MergeDialog<CategoryDto | TagDto>
        open={merging !== null}
        source={merging?.item ?? null}
        candidates={mergeCandidates()}
        warning={t('classifications.common.merge.warning', { name: merging?.item.name ?? '' })}
        info={t('classifications.common.merge.envelope_info')}
        showIcon
        onClose={() => setMerging(null)}
        onConfirm={(targetId) => {
          if (merging?.kind === 'category') {
            mergeCategory.mutate({ sourceId: merging.item.id, targetId })
          } else if (merging?.kind === 'tag') {
            mergeTag.mutate({ sourceId: merging.item.id, targetId })
          }
          setMerging(null)
        }}
      />
      <ConfirmDialog
        open={deleting !== null}
        onClose={() => setDeleting(null)}
        onConfirm={() => {
          if (deleting?.kind === 'category') {
            deleteCategory.mutate(deleting.item.id)
          } else if (deleting?.kind === 'tag') {
            deleteTag.mutate(deleting.item.id)
          }
          setDeleting(null)
        }}
        title={deleting?.kind === 'tag' ? t('classifications.tags.modals.delete.title') : t('classifications.categories.modals.delete.title')}
        question={deleting?.item.name ?? ''}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
      <CategoryDialog
        open={creating !== null}
        fixedType={creating?.type}
        onClose={() => setCreating(null)}
        onSubmit={(form) => {
          const target = creating
          if (!target) {
            return
          }
          createCategory.mutate(
            { name: form.name, type: form.type, icon: form.icon, ownerUserId: user?.id },
            {
              onSuccess: (item) => {
                setCreating(null)
                // placing it also gives the brand-new category its budget line
                moveElement.mutate({ budgetId, item: { id: item.id, folderId: target.folderId, position: 0, afterId: null } })
              },
            },
          )
        }}
      />
    </>
  )

  return { actionsFor, newCategoryAction, dialogs }
}
