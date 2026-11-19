import { useId, useState } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { pluralPick } from '@/lib/plural'
import { cn } from '@/lib/utils'
import { columnName } from './columnLabels'
import type { ColumnProfile, ColumnType } from './columns'

interface ColumnChoicesProps {
  profiles: ColumnProfile[]
  /** the step's question, the accessible name of the choice */
  label: string
  selected: number | null
  onSelect: (column: number | null) => void
  /** the kinds of column that can answer the question; the others wait behind "more columns" */
  fits?: (type: ColumnType) => boolean
  /** what another step uses a column for, shown as a small tag */
  tags?: Record<number, string>
  /** offers "no column" as an answer */
  noneLabel?: string
}

const optionClass = (selected: boolean) =>
  cn(
    'flex min-w-0 cursor-pointer items-start gap-3 rounded-xl border p-3 text-left transition-colors has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring/50',
    selected ? 'border-primary bg-econumo-magenta-light/25' : 'border-border hover:border-primary/40 hover:bg-econumo-card/50',
  )

function Dot({ on }: { on: boolean }) {
  return (
    <span aria-hidden className={cn('mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border', on ? 'border-primary bg-primary' : 'border-input')}>
      {on ? <span className="size-1.5 rounded-full bg-primary-foreground" /> : null}
    </span>
  )
}

/** The file's columns as a list of answers: each shows its header and first values in one line */
export function ColumnChoices({ profiles, label, selected, onSelect, fits, tags = {}, noneLabel }: ColumnChoicesProps) {
  const { t, i18n } = useTranslation()
  const name = useId()
  const [showAll, setShowAll] = useState(false)
  const usable = profiles.filter((p) => p.type !== 'empty' || p.index === selected)
  // the chosen column is always in view, whatever its kind
  const shown = showAll || !fits ? usable : usable.filter((p) => fits(p.type) || p.index === selected)
  const hidden = usable.length - shown.length

  return (
    <div className="flex flex-col gap-2">
      <div role="radiogroup" aria-label={label} className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        {shown.map((p) => {
          const on = selected === p.index
          const id = `${name}-${p.index}`
          return (
            <label key={p.index} className={optionClass(on)}>
              <input
                type="radio"
                name={name}
                className="sr-only"
                checked={on}
                onChange={() => onSelect(p.index)}
                aria-labelledby={`${id}-name`}
                aria-describedby={tags[p.index] ? `${id}-tag ${id}-values` : `${id}-values`}
              />
              <Dot on={on} />
              <span className="min-w-0 flex-1">
                <span className="flex items-center gap-2">
                  <span id={`${id}-name`} className="truncate text-sm">
                    {columnName(p, t)}
                  </span>
                  {tags[p.index] ? (
                    <span id={`${id}-tag`} className="shrink-0 rounded-full bg-econumo-card px-2 py-0.5 text-[11px] text-muted-foreground">
                      {tags[p.index]}
                    </span>
                  ) : null}
                </span>
                <span id={`${id}-values`} className="mt-1 block truncate text-xs text-muted-foreground">
                  {p.samples.length > 0 ? p.samples.join(' · ') : t('transactions.import_wizard.column_empty')}
                </span>
              </span>
            </label>
          )
        })}
        {noneLabel ? (
          <label className={cn(optionClass(selected === null), 'items-center border-dashed', selected !== null && 'text-muted-foreground')}>
            <input type="radio" name={name} className="sr-only" checked={selected === null} onChange={() => onSelect(null)} />
            <X aria-hidden className="size-4 shrink-0" />
            <span className="text-sm">{noneLabel}</span>
          </label>
        ) : null}
      </div>
      {hidden > 0 ? (
        <button type="button" className="self-start text-sm text-primary underline-offset-2 hover:underline" onClick={() => setShowAll(true)}>
          {pluralPick(t('transactions.import_wizard.more_columns'), hidden, i18n.language)}
        </button>
      ) : null}
    </div>
  )
}
