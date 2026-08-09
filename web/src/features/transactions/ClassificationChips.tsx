import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { EntityIcon } from '@/components/EntityIcon'
import { type ClassificationKind } from '@/lib/classificationKind'
import type { ClassificationChip } from './useTransactionForm'

interface ClassificationChipsProps {
  chips: ClassificationChip[]
  onToggle: (chip: ClassificationChip) => void
}

/** One flat row of budget-tag and reporting-tag chips. */
export function ClassificationChips({ chips, onToggle }: ClassificationChipsProps) {
  const { t } = useTranslation()
  // The kind rides in the accessible name because nothing else conveys it to
  // assistive tech: data-kind is invisible and the icon is decorative. The
  // visible name leads, so the accessible name still contains it.
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
          /* A transaction takes ONE budget tag but any number of reporting
             tags, so it is the reporting chips that pile up. The budget tag
             keeps the solid fill — it is the single, deliberate choice —
             while a selected reporting tag gets a tint plus a border: still
             clearly on, without a row of them competing with the dialog's
             own actions. */
          className={`cursor-pointer gap-1 ${
            chip.checked && chip.kind === 'label' ? 'border border-primary/40 bg-primary/10 text-foreground' : ''
          }`}
          onClick={() => onToggle(chip)}
          onKeyDown={(e) => {
            if (!e.repeat && (e.key === 'Enter' || e.key === ' ')) {
              e.preventDefault()
              onToggle(chip)
            }
          }}
        >
          {/* the row's STORED icon, never a kind default: a user-picked icon
              must survive. The icon alone distinguishes the kinds (seeded per
              kind at create); only the solid budget chip inherits the badge's
              own colour, every other state keeps the muted tone. */}
          <EntityIcon name={chip.icon} className={`text-sm ${chip.checked && chip.kind === 'tag' ? '' : 'text-muted-foreground'}`} />
          {chip.name}
        </Badge>
      ))}
    </div>
  )
}
