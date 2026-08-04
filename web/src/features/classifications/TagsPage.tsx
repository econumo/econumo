import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { kindAccentClass, type ClassificationKind } from '@/lib/classificationKind'
import { useUserData } from '@/features/user/queries'
import { ClassificationList, type ClassificationItem } from './ClassificationList'
import { TagDialog, type TagDialogItem } from './TagDialog'
import {
  useTags,
  useLabels,
  useArchiveTag,
  useUnarchiveTag,
  useDeleteTag,
  useOrderTags,
  useArchiveLabel,
  useUnarchiveLabel,
  useDeleteLabel,
  useOrderLabels,
} from './queries'

interface ClassificationRow extends ClassificationItem {
  kind: ClassificationKind
}

export function TagsPage() {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: tags = [] } = useTags()
  const { data: labels = [] } = useLabels()
  const archiveTag = useArchiveTag()
  const unarchiveTag = useUnarchiveTag()
  const deleteTag = useDeleteTag()
  const orderTags = useOrderTags()
  const archiveLabel = useArchiveLabel()
  const unarchiveLabel = useUnarchiveLabel()
  const deleteLabel = useDeleteLabel()
  const orderLabels = useOrderLabels()

  const [dialog, setDialog] = useState<{ open: boolean; item: TagDialogItem | null }>({ open: false, item: null })

  const ownTags = tags.filter((tg) => !user || tg.ownerUserId === user.id)
  const ownLabels = labels.filter((lb) => !user || lb.ownerUserId === user.id)
  // Tags and labels hold INDEPENDENT position sequences on the backend, so the
  // merged row's position is a local, 0-based-per-kind index rather than the
  // real stored value — real values would collide across kinds and corrupt
  // ClassificationList's reorder diffing (see orderScope + onOrder below).
  const rows: ClassificationRow[] = [
    ...ownTags.map((tg, i) => ({ ...tg, kind: 'tag' as const, position: i })),
    ...ownLabels.map((lb, i) => ({ ...lb, kind: 'label' as const, position: ownTags.length + i })),
  ]

  return (
    <>
      <ClassificationList
        title={t('classifications.tags.pages.settings.header')}
        info={t('classifications.tags.pages.settings.tags_and_labels_info')}
        createLabel={t('classifications.tags.pages.settings.create_tag')}
        deleteTitle={(row) => (row.kind === 'tag' ? t('classifications.tags.modals.delete.title') : t('classifications.labels.modals.delete.title'))}
        archivedLabel={(row) => (row.kind === 'tag' ? t('classifications.tags.pages.settings.archived_item') : t('classifications.labels.pages.settings.archived_item'))}
        items={rows}
        storageKey="settings.tags.activeOnly"
        analyticsType="tag"
        sections={[
          { label: t('classifications.tags.pages.settings.header'), match: (row) => row.kind === 'tag' },
          { label: t('classifications.labels.pages.settings.header'), match: (row) => row.kind === 'label' },
        ]}
        showIcon
        iconClassName={(row) => kindAccentClass(row.kind)}
        orderScope={(row) => row.kind}
        onCreate={() => setDialog({ open: true, item: null })}
        onEdit={(row) => setDialog({ open: true, item: { id: row.id, name: row.name, kind: row.kind, icon: row.icon ?? '' } })}
        onDelete={(id) => {
          const row = rows.find((r) => r.id === id)
          if (row?.kind === 'label') {
            deleteLabel.mutate(id)
          } else {
            deleteTag.mutate(id)
          }
        }}
        onToggleArchive={(row) => {
          const archive = row.kind === 'tag' ? archiveTag : archiveLabel
          const unarchive = row.kind === 'tag' ? unarchiveTag : unarchiveLabel
          const mutation = row.isArchived === 0 ? archive : unarchive
          mutation.mutate(row.id)
        }}
        onOrder={(changes) => {
          const kindOf = (id: string) => rows.find((r) => r.id === id)?.kind
          const tagChanges = changes.filter((c) => kindOf(c.id) === 'tag')
          const labelChanges = changes
            .filter((c) => kindOf(c.id) === 'label')
            .map((c) => ({ id: c.id, position: c.position - ownTags.length }))
          if (tagChanges.length > 0) {
            orderTags.mutate(tagChanges)
          }
          if (labelChanges.length > 0) {
            orderLabels.mutate(labelChanges)
          }
        }}
      />
      <TagDialog open={dialog.open} item={dialog.item} onClose={() => setDialog({ open: false, item: null })} />
    </>
  )
}
