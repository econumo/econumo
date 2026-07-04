import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PromptDialog } from '@/components/PromptDialog'
import { isNotEmpty, isValidTagName } from '@/lib/validation'
import type { TagDto } from '@/api/dto/tag'
import { useUserData } from '@/features/user/queries'
import { ClassificationList } from './ClassificationList'
import { useTags, useCreateTag, useUpdateTag, useArchiveTag, useUnarchiveTag, useDeleteTag, useOrderTags } from './queries'

export function TagsPage() {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: tags = [] } = useTags()
  const createTag = useCreateTag()
  const updateTag = useUpdateTag()
  const archiveTag = useArchiveTag()
  const unarchiveTag = useUnarchiveTag()
  const deleteTag = useDeleteTag()
  const orderTags = useOrderTags()

  const [dialog, setDialog] = useState<{ open: boolean; tag: TagDto | null }>({ open: false, tag: null })
  const own = tags.filter((tg) => !user || tg.ownerUserId === user.id)

  const validate = (value: string): string | null => {
    if (!isNotEmpty(value)) {
      return t('modules.classifications.tags.forms.tag.name.validation.required_field')
    }
    if (!isValidTagName(value)) {
      return t('modules.classifications.tags.forms.tag.name.validation.invalid_name')
    }
    return null
  }

  return (
    <>
      <ClassificationList
        title={t('modules.classifications.tags.pages.settings.header')}
        createLabel={t('modules.classifications.tags.pages.settings.create_tag')}
        deleteTitle={t('modules.classifications.tags.modals.delete.title')}
        items={own}
        onCreate={() => setDialog({ open: true, tag: null })}
        onEdit={(tag) => setDialog({ open: true, tag })}
        onDelete={(id) => deleteTag.mutate(id)}
        onToggleArchive={(tag) => (tag.isArchived === 0 ? archiveTag.mutate(tag.id) : unarchiveTag.mutate(tag.id))}
        onOrder={(changes) => orderTags.mutate(changes)}
      />
      <PromptDialog
        open={dialog.open}
        onClose={() => setDialog({ open: false, tag: null })}
        onSubmit={(name) => {
          if (dialog.tag) {
            updateTag.mutate({ id: dialog.tag.id, name }, { onSuccess: () => setDialog({ open: false, tag: null }) })
          } else {
            createTag.mutate({ name, ownerUserId: user?.id }, { onSuccess: () => setDialog({ open: false, tag: null }) })
          }
        }}
        title={dialog.tag ? t('modules.classifications.tags.modals.edit.header') : t('modules.classifications.tags.modals.create.header')}
        inputLabel={t('modules.classifications.tags.forms.tag.name.label')}
        initialValue={dialog.tag?.name ?? ''}
        validate={validate}
        submitLabel={dialog.tag ? t('elements.button.update.label') : t('elements.button.create.label')}
        cancelLabel={t('elements.button.cancel.label')}
      />
    </>
  )
}
