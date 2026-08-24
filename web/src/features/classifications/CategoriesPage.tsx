import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { CategoryDto } from '@/api/dto/category'
import { useUserData } from '@/features/user/queries'
import { ClassificationList } from './ClassificationList'
import { MergeDialog } from './MergeDialog'
import { CategoryDialog } from './CategoryDialog'
import {
  useCategories,
  useCreateCategory,
  useUpdateCategory,
  useArchiveCategory,
  useUnarchiveCategory,
  useDeleteCategory,
  useMoveCategory,
  useSortCategories, useMergeCategory
} from './queries'

export function CategoriesPage() {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: categories = [] } = useCategories()
  const createCategory = useCreateCategory()
  const updateCategory = useUpdateCategory()
  const archiveCategory = useArchiveCategory()
  const unarchiveCategory = useUnarchiveCategory()
  const deleteCategory = useDeleteCategory()
  const moveCategories = useMoveCategory()
  const sortCategories = useSortCategories()
  const mergeCategory = useMergeCategory()

  const [dialog, setDialog] = useState<{ open: boolean; category: CategoryDto | null }>({ open: false, category: null })
  const [mergeSource, setMergeSource] = useState<CategoryDto | null>(null)

  const own = categories.filter((c) => !user || c.ownerUserId === user.id)

  return (
    <>
      <ClassificationList
        title={t('classifications.categories.pages.settings.header')}
        info={t('classifications.categories.pages.settings.info')}
        createLabel={t('classifications.categories.pages.settings.create_category')}
        deleteTitle={t('classifications.categories.modals.delete.title')}
        archivedLabel={t('classifications.categories.pages.settings.archived_item')}
        items={own}
        storageKey="settings.categories.activeOnly"
        analyticsType="category"
        sections={[
          { label: t('classifications.categories.forms.category.type.expense'), match: (c) => c.type === 'expense' },
          { label: t('classifications.categories.forms.category.type.income'), match: (c) => c.type === 'income' },
        ]}
        showIcon
        onCreate={() => setDialog({ open: true, category: null })}
        onEdit={(category) => setDialog({ open: true, category })}
        extraActions={(category) => [{ label: t('classifications.common.merge.action'), onSelect: () => setMergeSource(category) }]}
        onDelete={(id) => deleteCategory.mutate(id)}
        onToggleArchive={(category) => (category.isArchived === 0 ? archiveCategory.mutate(category.id) : unarchiveCategory.mutate(category.id))}
        onMove={(move) => moveCategories.mutate(move)}
        onSort={(ids) => sortCategories.mutate(ids)}
      />
      <MergeDialog
        open={mergeSource !== null}
        source={mergeSource}
        // own items only, and same type: income and expense sit in different
        // halves of the budget, so the server refuses to merge across them
        candidates={own.filter((c) => c.type === mergeSource?.type)}
        warning={t('classifications.common.merge.warning', { name: mergeSource?.name ?? '' })}
        info={t('classifications.common.merge.envelope_info')}
        showIcon
        onClose={() => setMergeSource(null)}
        onConfirm={(targetId) => {
          if (mergeSource) {
            mergeCategory.mutate({ sourceId: mergeSource.id, targetId })
          }
          setMergeSource(null)
        }}
      />
      <CategoryDialog
        open={dialog.open}
        category={dialog.category}
        onClose={() => setDialog({ open: false, category: null })}
        onSubmit={(form) => {
          if (dialog.category) {
            updateCategory.mutate(
              { id: dialog.category.id, name: form.name, icon: form.icon },
              { onSuccess: () => setDialog({ open: false, category: null }) },
            )
          } else {
            createCategory.mutate(
              { name: form.name, type: form.type, icon: form.icon, ownerUserId: user?.id },
              { onSuccess: () => setDialog({ open: false, category: null }) },
            )
          }
        }}
      />
    </>
  )
}
