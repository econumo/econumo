import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { pluralPick } from '@/lib/plural'
import { cn } from '@/lib/utils'
import { displaySamples, fromName } from './columnLabels'
import type { ColumnProfile, ColumnType } from './columns'

interface ColumnChoicesProps {
  profiles: ColumnProfile[]
  /** the question, shown above the list and its accessible name */
  label: string
  selected: number | null
  onSelect: (column: number | null) => void
  /** the kinds of values that can answer the question; the others wait behind "Show N more" */
  fits?: (type: ColumnType) => boolean
  /** what another step uses a part of the file for, shown as a small tag */
  tags?: Record<number, string>
  /** offers "none of these" as an answer */
  noneLabel?: string
}

const rowClass = (selected: boolean) =>
  cn(
    'flex min-w-0 cursor-pointer items-start gap-3 px-4 py-3 text-left transition-colors has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-inset has-[:focus-visible]:ring-ring/50',
    selected ? 'bg-econumo-magenta-light/25' : 'hover:bg-econumo-card/50',
  )

function Dot({ on }: { on: boolean }) {
  return (
    <span aria-hidden className={cn('mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border', on ? 'border-primary bg-primary' : 'border-input')}>
      {on ? <span className="size-1.5 rounded-full bg-primary-foreground" /> : null}
    </span>
  )
}

/** The parts of the file as one list of answers, each led by its first values; the file's own name for it is a grey caption */
export function ColumnChoices({ profiles, label, selected, onSelect, fits, tags = {}, noneLabel }: ColumnChoicesProps) {
  const { t, i18n } = useTranslation()
  const name = useId()
  const [showAll, setShowAll] = useState(false)
  // the chosen one is always in view, whatever it holds
  const answers = (p: ColumnProfile) => p.index === selected || (p.type !== 'empty' && !p.internal && (!fits || fits(p.type)))
  const shown = showAll ? profiles : profiles.filter(answers)
  const hidden = profiles.length - shown.length

  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm">{label}</p>
      <div role="radiogroup" aria-label={label} className="flex flex-col divide-y divide-border overflow-hidden rounded-xl border border-border">
        {shown.map((p) => {
          const on = selected === p.index
          const id = `${name}-${p.index}`
          const from = fromName(p, profiles, t)
          const values = displaySamples(p, i18n.language)
          return (
            <label key={p.index} className={rowClass(on)}>
              <input
                type="radio"
                name={name}
                className="sr-only"
                checked={on}
                onChange={() => onSelect(p.index)}
                aria-labelledby={from ? `${id}-values ${id}-from` : `${id}-values`}
                aria-describedby={tags[p.index] ? `${id}-tag` : undefined}
              />
              <Dot on={on} />
              <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                <span id={`${id}-values`} className={cn('truncate text-sm', values.length === 0 && 'text-muted-foreground')}>
                  {values.length > 0 ? values.join(' · ') : t('transactions.import_wizard.column_empty')}
                </span>
                {from || tags[p.index] ? (
                  <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                    {from ? (
                      <span id={`${id}-from`} className="truncate text-xs text-muted-foreground">
                        {from}
                      </span>
                    ) : null}
                    {tags[p.index] ? (
                      <span id={`${id}-tag`} className="shrink-0 rounded-full bg-econumo-card px-2 py-0.5 text-[11px] text-muted-foreground">
                        {tags[p.index]}
                      </span>
                    ) : null}
                  </span>
                ) : null}
              </span>
            </label>
          )
        })}
        {noneLabel ? (
          <label className={cn(rowClass(selected === null), selected !== null && 'text-muted-foreground')}>
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
