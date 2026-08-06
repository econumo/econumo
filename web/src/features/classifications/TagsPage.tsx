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
  useMoveTag,
  useSortTags,
  useArchiveLabel,
  useUnarchiveLabel,
  useDeleteLabel,
  useMoveLabel,
  useSortLabels,
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
  const moveTag = useMoveTag()
  const sortTags = useSortTags()
  const archiveLabel = useArchiveLabel()
  const unarchiveLabel = useUnarchiveLabel()
  const deleteLabel = useDeleteLabel()
  const moveLabel = useMoveLabel()
  const sortLabels = useSortLabels()

  const [dialog, setDialog] = useState<{ open: boolean; item: TagDialogItem | null }>({ open: false, item: null })

  const ownTags = tags.filter((tg) => !user || tg.ownerUserId === user.id)
  const ownLabels = labels.filter((lb) => !user || lb.ownerUserId === user.id)
  // Tags and labels hold INDEPENDENT sort-key sequences on the backend, and each
  // list's wire "position" is a dense 0-based index within its OWN list — so both
  // kinds start at 0. Restamping the merged rows keeps them monotonic, which is
  // what the list's ordering helpers read; orderScope below keeps every reorder
  // request confined to one kind.
  const rows: ClassificationRow[] = [
    ...ownTags.map((tg, i) => ({ ...tg, kind: 'tag' as const, position: i })),
    ...ownLabels.map((lb, i) => ({ ...lb, kind: 'label' as const, position: ownTags.length + i })),
  ]

  const kindOf = (id: string) => rows.find((row) => row.id === id)?.kind

  return (
    <>
      <ClassificationList
        title={t('classifications.tags.pages.settings.header')}
        info={t('classifications.tags.pages.settings.info')}
        createLabel={t('classifications.tags.pages.settings.create_tag')}
        deleteTitle={(row) => (row.kind === 'tag' ? t('classifications.tags.modals.delete.title') : t('classifications.labels.modals.delete.title'))}
        archivedLabel={(row) => (row.kind === 'tag' ? t('classifications.tags.pages.settings.archived_item') : t('classifications.labels.pages.settings.archived_item'))}
        items={rows}
        storageKey="settings.tags.activeOnly"
        analyticsType="tag"
        sections={[
          { label: t('classifications.tags.pages.settings.section_budget'), match: (row) => row.kind === 'tag' },
          { label: t('classifications.tags.pages.settings.section_reporting'), match: (row) => row.kind === 'label' },
        ]}
        showIcon
        iconClassName={(row) => kindAccentClass(row.kind)}
        orderScope={(row) => row.kind}
        onCreate={() => setDialog({ open: true, item: null })}
        onEdit={(row) => setDialog({ open: true, item: { id: row.id, name: row.name, kind: row.kind, icon: row.icon ?? '' } })}
        onDelete={(id) => {
          if (kindOf(id) === 'label') {
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
        // orderScope guarantees the anchor shares the moved row's kind, and that
        // onSort fires once per kind, so each request only ever names ids the
        // receiving endpoint owns.
        onMove={(move) => (kindOf(move.id) === 'label' ? moveLabel.mutate(move) : moveTag.mutate(move))}
        onSort={(ids) => (ids.length > 0 && kindOf(ids[0]) === 'label' ? sortLabels.mutate(ids) : sortTags.mutate(ids))}
      />
      <TagDialog open={dialog.open} item={dialog.item} onClose={() => setDialog({ open: false, item: null })} />
    </>
  )
}
