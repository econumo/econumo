import { useId } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { ColumnProfile } from './columns'

interface ColumnCardsProps {
  profiles: ColumnProfile[]
  value: number | null
  onChange: (index: number | null) => void
  allowNone?: boolean
  label: string
}

const cardClass =
  'flex min-w-0 cursor-pointer flex-col gap-1 rounded-lg border p-3 text-left transition-colors hover:bg-muted/50 has-[:checked]:border-primary has-[:checked]:bg-primary/5 has-[:focus-visible]:ring-3 has-[:focus-visible]:ring-ring/50'

function Card({ name, checked, onSelect, children }: { name: string; checked: boolean; onSelect: () => void; children: ReactNode }) {
  return (
    <label className={cardClass}>
      <input type="radio" name={name} className="sr-only" checked={checked} onChange={onSelect} />
      {children}
    </label>
  )
}

/** The file's columns as choice cards: the header name with its first values, so a
    column is recognised by what it holds. Empty columns have nothing to show and are left out. */
export function ColumnCards({ profiles, value, onChange, allowNone = false, label }: ColumnCardsProps) {
  const { t } = useTranslation()
  const name = useId()
  return (
    <div role="radiogroup" aria-label={label} className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
      {profiles
        .filter((p) => p.type !== 'empty')
        .map((p) => (
          <Card key={p.index} name={name} checked={value === p.index} onSelect={() => onChange(p.index)}>
            <span className="truncate text-sm">{p.name}</span>
            {p.samples.map((sample, i) => (
              <span key={i} className="truncate text-xs text-muted-foreground">
                {sample}
              </span>
            ))}
          </Card>
        ))}
      {allowNone ? (
        <Card name={name} checked={value === null} onSelect={() => onChange(null)}>
          <span className="text-sm">{t('transactions.import_wizard.none')}</span>
        </Card>
      ) : null}
    </div>
  )
}

interface ChoiceCardsProps<T extends string> {
  label: string
  value: T | null
  options: { value: T; label: string }[]
  onChange: (value: T) => void
}

/** A plain question with a few answers, in the same card look as the columns */
export function ChoiceCards<T extends string>({ label, value, options, onChange }: ChoiceCardsProps<T>) {
  const name = useId()
  return (
    <div role="radiogroup" aria-label={label} className="grid grid-cols-1 gap-2 sm:grid-cols-2">
      {options.map((option) => (
        <Card key={option.value} name={name} checked={value === option.value} onSelect={() => onChange(option.value)}>
          <span className="text-sm">{option.label}</span>
        </Card>
      ))}
    </div>
  )
}
