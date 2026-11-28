import { useId } from 'react'
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
  /** the parts of the file that could answer, most likely first; the rest follow under a divider */
  candidates: number[]
  /** what another step uses a part of the file for, shown as a small tag */
  tags?: Record<number, string>
  /** offers "none of these" as an answer */
  noneLabel?: string
  /** nothing was chosen yet: "none of these" is offered but not shown as the pick */
  unanswered?: boolean
  /** said above the list when nothing in the file fits and the whole file is listed */
  nothingFitsNote?: string
  /** said above the list when exactly one part fits and there is no "none of these" */
  onlyOneNote?: string
  /** an optional answer: with nothing that fits, "none of these" is the whole list */
  noneWhenNothingFits?: boolean
  /** parts of the file that can never answer this question, left out of the list */
  exclude?: number[]
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
export function ColumnChoices({ profiles, label, selected, onSelect, candidates, tags = {}, noneLabel, unanswered = false, nothingFitsNote, onlyOneNote, noneWhenNothingFits = false, exclude = [] }: ColumnChoicesProps) {
  const { t, i18n } = useTranslation()
  const name = useId()
  const filled = profiles.filter((p) => (p.type !== 'empty' || p.index === selected) && !exclude.includes(p.index)).map((p) => p.index)
  const likely = [...(selected === null ? [] : [selected]), ...candidates.filter((c) => c !== selected)]
  const rest = filled.filter((c) => !likely.includes(c))
  const nothingFits = candidates.length === 0 && !noneWhenNothingFits
  // an optional answer with nothing that fits is just "none of these"
  const main = nothingFits ? [...likely, ...rest] : likely
  const others = nothingFits || (candidates.length === 0 && noneWhenNothingFits) ? [] : rest
  const noneOn = selected === null && !unanswered
  const note = nothingFits ? nothingFitsNote : main.length === 1 && others.length === 0 && !noneLabel ? onlyOneNote : null

  const renderRow = (index: number) => {
    const p = profiles[index]
    const on = selected === index
    const id = `${name}-${index}`
    const from = fromName(p, profiles, t)
    const values = displaySamples(p, i18n.language)
    const reference = p.internal && values.length > 0
    const empty = values.length === 0 || reference
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
          <span id={`${id}-values`} className={cn('truncate text-sm', empty && 'text-muted-foreground')}>
            {reference
              ? t('transactions.import_wizard.column_reference')
              : values.length > 0
                ? values.join(' · ')
                : t('transactions.import_wizard.column_empty')}
          </span>
          {from || tags[index] ? (
            <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
              {from ? (
                <span id={`${id}-from`} className="truncate text-xs text-muted-foreground/60">
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
  }

  return (
    <div className="flex flex-col gap-2">
      <p className="text-sm">{label}</p>
      {note ? <p className="text-xs text-muted-foreground">{note}</p> : null}
      <div role="radiogroup" aria-label={label} className="flex flex-col divide-y divide-border overflow-hidden rounded-xl border border-border">
        {main.map(renderRow)}
        {noneLabel ? (
          <button
            type="button"
            role="radio"
            aria-checked={noneOn}
            className={cn(rowClass(noneOn), !noneOn && 'text-muted-foreground')}
            onClick={() => onSelect(null)}
          >
            <Dot on={noneOn} />
            <span className="text-sm">{noneLabel}</span>
          </button>
        ) : null}
        {others.length > 0 ? (
          <>
            <div role="presentation" aria-hidden className="bg-econumo-card/40 px-4 py-1.5 text-xs text-muted-foreground">
              {t('transactions.import_wizard.other_parts')}
            </div>
            {others.map(renderRow)}
          </>
        ) : null}
      </div>
    </div>
  )
}
