import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { EntityIcon } from '@/components/EntityIcon'
import { kindAccentClass, type ClassificationKind } from '@/lib/classificationKind'
import type { ClassificationChip } from './useTransactionForm'

interface ClassificationChipsProps {
  chips: ClassificationChip[]
  onToggle: (chip: ClassificationChip) => void
}

/** One flat row of tag and label chips. Tags and labels are independent
 *  namespaces, so a name may appear twice; data-kind tells the two apart. */
export function ClassificationChips({ chips, onToggle }: ClassificationChipsProps) {
  const { t } = useTranslation()
  // The kind rides in the accessible name because a tag and a label may share
  // one name and data-kind is invisible to assistive tech; the visible name
  // leads, so the accessible name still contains the visible label.
  const kindWord = (kind: ClassificationKind) =>
    kind === 'tag' ? t('classifications.tags.forms.tag.kind.tag') : t('classifications.tags.forms.tag.kind.label')
  return (
    <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1.5 py-0.5">
      {chips.map((chip) => (
        <Badge
          key={`${chip.kind}:${chip.id}`}
          role="checkbox"
          aria-checked={chip.checked}
          aria-label={`${chip.name} ${kindWord(chip.kind)}`}
          data-kind={chip.kind}
          tabIndex={0}
          variant={chip.checked ? 'default' : 'secondary'}
          className="cursor-pointer gap-1"
          onClick={() => onToggle(chip)}
          onKeyDown={(e) => {
            if (!e.repeat && (e.key === 'Enter' || e.key === ' ')) {
              e.preventDefault()
              onToggle(chip)
            }
          }}
        >
          {/* the row's STORED icon, never a kind default: a user-picked icon
              must survive; only the tint encodes the kind */}
          <EntityIcon name={chip.icon} className={`text-sm ${chip.checked ? '' : kindAccentClass(chip.kind)}`} />
          {chip.name}
        </Badge>
      ))}
    </div>
  )
}
