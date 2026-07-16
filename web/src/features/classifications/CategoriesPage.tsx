import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { CategoryDto } from '@/api/dto/category'
import { useUserData } from '@/features/user/queries'
import { ClassificationList } from './ClassificationList'
import { CategoryDialog } from './CategoryDialog'
import {
  useCategories,
  useCreateCategory,
  useUpdateCategory,
  useArchiveCategory,
  useUnarchiveCategory,
  useDeleteCategory,
  useOrderCategories,
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
  const orderCategories = useOrderCategories()

  const [dialog, setDialog] = useState<{ open: boolean; category: CategoryDto | null }>({ open: false, category: null })

  const own = categories.filter((c) => !user || c.ownerUserId === user.id)

  return (
    <>
      <ClassificationList
        title={t('classifications.categories.pages.settings.header')}
        createLabel={t('classifications.categories.pages.settings.create_category')}
        deleteTitle={t('classifications.categories.modals.delete.title')}
        items={own}
        storageKey="settings.categories.activeOnly"
        sections={[
          { label: t('classifications.categories.forms.category.type.expense'), match: (c) => c.type === 'expense' },
          { label: t('classifications.categories.forms.category.type.income'), match: (c) => c.type === 'income' },
        ]}
        showIcon
        onCreate={() => setDialog({ open: true, category: null })}
        onEdit={(category) => setDialog({ open: true, category })}
        onDelete={(id) => deleteCategory.mutate(id)}
        onToggleArchive={(category) => (category.isArchived === 0 ? archiveCategory.mutate(category.id) : unarchiveCategory.mutate(category.id))}
        onOrder={(changes) => orderCategories.mutate(changes)}
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
