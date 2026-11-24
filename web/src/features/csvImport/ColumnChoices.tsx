import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { displaySamples, fromName } from './columnLabels'
import type { ColumnProfile } from './columns'

interface ColumnChoicesProps {
  profiles: ColumnProfile[]
  /** the question, shown above the list and its accessible name */
  label: string
  selected: number | null
  onSelect: (column: number | null) => void
  /** the parts of the file that could answer, most likely first; the rest wait behind "Show all" */
  candidates: number[]
  /** what another step uses a part of the file for, shown as a small tag */
  tags?: Record<number, string>
  /** offers "none of these" as an answer */
  noneLabel?: string
}

const rowClass = (selected: boolean) =>
  cn(
    'flex w-full min-w-0 cursor-pointer items-start gap-3 px-4 py-3 text-left transition-colors focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none focus-visible:ring-inset',
    selected ? 'bg-econumo-magenta-light/25' : 'hover:bg-econumo-card/50',
  )

function Dot({ on }: { on: boolean }) {
  return (
    <span aria-hidden className={cn('mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border', on ? 'border-primary bg-primary' : 'border-input')}>
      {on ? <span className="size-1.5 rounded-full bg-primary-foreground" /> : null}
    </span>
  )
}

/**
 * The parts of the file that could answer the question, each led by its first values; the
 * file's own name for it is a grey caption. An answer is a button, not a native radio: picking
 * the one already chosen is an answer too ("yes, that one"), and arrow keys must not pick.
 */
export function ColumnChoices({ profiles, label, selected, onSelect, candidates, tags = {}, noneLabel }: ColumnChoicesProps) {
  const { t, i18n } = useTranslation()
  const name = useId()
  const [showAll, setShowAll] = useState(false)
  const filled = profiles.filter((p) => p.type !== 'empty' || p.index === selected).map((p) => p.index)
  const likely = [...(selected === null ? [] : [selected]), ...candidates.filter((c) => c !== selected)]
  // with nothing that fits, the whole file is the list straight away
  const all = showAll || candidates.length === 0
  const shown = all ? [...likely, ...filled.filter((c) => !likely.includes(c))] : likely
  const hidden = filled.filter((c) => !shown.includes(c)).length

  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm">{label}</p>
      <div role="radiogroup" aria-label={label} className="flex flex-col divide-y divide-border overflow-hidden rounded-xl border border-border">
        {shown.map((index) => {
          const p = profiles[index]
          const on = selected === index
          const id = `${name}-${index}`
          const from = fromName(p, profiles, t)
          const values = displaySamples(p, i18n.language)
          return (
            <button
              key={index}
              type="button"
              role="radio"
              aria-checked={on}
              className={rowClass(on)}
              onClick={() => onSelect(index)}
              aria-labelledby={from ? `${id}-values ${id}-from` : `${id}-values`}
              aria-describedby={tags[index] ? `${id}-tag` : undefined}
            >
              <Dot on={on} />
              <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                <span id={`${id}-values`} className={cn('truncate text-sm', values.length === 0 && 'text-muted-foreground')}>
                  {values.length > 0 ? values.join(' · ') : t('transactions.import_wizard.column_empty')}
                </span>
                {from || tags[index] ? (
                  <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                    {from ? (
                      <span id={`${id}-from`} className="truncate text-xs text-muted-foreground">
                        {from}
                      </span>
                    ) : null}
                    {tags[index] ? (
                      <span id={`${id}-tag`} className="shrink-0 rounded-full bg-econumo-card px-2 py-0.5 text-[11px] text-muted-foreground">
                        {tags[index]}
                      </span>
                    ) : null}
                  </span>
                ) : null}
              </span>
            </button>
          )
        })}
        {noneLabel ? (
          <button
            type="button"
            role="radio"
            aria-checked={selected === null}
            className={cn(rowClass(selected === null), selected !== null && 'text-muted-foreground')}
            onClick={() => onSelect(null)}
          >
            <Dot on={selected === null} />
            <span className="text-sm">{noneLabel}</span>
          </button>
        ) : null}
      </div>
      {hidden > 0 ? (
        <button type="button" className="self-start text-sm text-muted-foreground underline-offset-2 hover:text-foreground hover:underline" onClick={() => setShowAll(true)}>
          {t('transactions.import_wizard.show_all')}
        </button>
      ) : null}
    </div>
  )
}
