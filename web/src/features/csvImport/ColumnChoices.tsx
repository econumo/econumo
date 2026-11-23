import { useId, useState } from 'react'
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

const rowClass = (selected: boolean) =>
  cn(
    'flex min-w-0 cursor-pointer items-start gap-3 px-4 py-3 text-left transition-colors sm:items-center has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-inset has-[:focus-visible]:ring-ring/50',
    selected ? 'bg-econumo-magenta-light/25' : 'hover:bg-econumo-card/50',
  )

function Dot({ on }: { on: boolean }) {
  return (
    <span aria-hidden className={cn('mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border sm:mt-0', on ? 'border-primary bg-primary' : 'border-input')}>
      {on ? <span className="size-1.5 rounded-full bg-primary-foreground" /> : null}
    </span>
  )
}

/** The file's columns as one list of answers: a header with its first values beside it, below it on a phone */
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
      <div role="radiogroup" aria-label={label} className="flex flex-col divide-y divide-border overflow-hidden rounded-xl border border-border">
        {shown.map((p) => {
          const on = selected === p.index
          const id = `${name}-${p.index}`
          return (
            <label key={p.index} className={rowClass(on)}>
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
              <span className="flex min-w-0 flex-1 flex-col gap-0.5 sm:flex-row sm:items-center sm:gap-4">
                <span className="flex min-w-0 items-center gap-2 sm:max-w-1/2 sm:shrink-0">
                  <span id={`${id}-name`} className="truncate text-sm">
                    {columnName(p, t)}
                  </span>
                  {tags[p.index] ? (
                    <span id={`${id}-tag`} className="shrink-0 rounded-full bg-econumo-card px-2 py-0.5 text-[11px] text-muted-foreground">
                      {tags[p.index]}
                    </span>
                  ) : null}
                </span>
                <span id={`${id}-values`} className="min-w-0 truncate text-xs text-muted-foreground sm:flex-1 sm:text-right">
                  {p.samples.length > 0 ? p.samples.join(' · ') : t('transactions.import_wizard.column_empty')}
                </span>
              </span>
            </label>
          )
        })}
        {noneLabel ? (
          <label className={cn(rowClass(selected === null), 'items-center', selected !== null && 'text-muted-foreground')}>
            <input type="radio" name={name} className="sr-only" checked={selected === null} onChange={() => onSelect(null)} />
            <Dot on={selected === null} />
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
