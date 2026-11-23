import { useId } from 'react'
import type { ReactNode } from 'react'
import { Check, Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

interface SegmentedProps<T extends string> {
  /** the question the answers belong to, their accessible name */
  label: string
  value: T | null
  options: { value: T; label: string }[]
  onChange: (value: T) => void
}

/** a few short answers side by side, one of them on */
export function Segmented<T extends string>({ label, value, options, onChange }: SegmentedProps<T>) {
  const name = useId()
  return (
    <div
      role="radiogroup"
      aria-label={label}
      className={cn(
        'w-full gap-1 rounded-xl bg-econumo-card p-1 sm:inline-flex sm:w-auto sm:flex-wrap sm:self-start',
        // three answers don't fit side by side on a phone; they stack instead of wrapping mid-word
        options.length > 2 ? 'grid grid-cols-1' : 'inline-flex flex-wrap',
      )}
    >
      {options.map((option) => (
        <label
          key={option.value}
          className={cn(
            'flex-1 cursor-pointer rounded-lg px-3 py-1.5 text-center text-sm transition-colors sm:flex-none has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring/50',
            value === option.value ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground',
          )}
        >
          <input type="radio" name={name} className="sr-only" checked={value === option.value} onChange={() => onChange(option.value)} />
          {option.label}
        </label>
      ))}
    </div>
  )
}

/** a labelled block of a step: a small heading, an optional hint and its control */
export function Field({ label, hint, htmlFor, children }: { label: string; hint?: string; htmlFor?: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2">
      <div>
        {htmlFor ? (
          <label htmlFor={htmlFor} className="block text-sm">
            {label}
          </label>
        ) : (
          <div className="text-sm">{label}</div>
        )}
        {hint ? <div className="text-xs text-muted-foreground">{hint}</div> : null}
      </div>
      {children}
    </div>
  )
}

/** the current answer as a sentence about the user's money, with where in the file it comes from */
export function Answer({ id, from, children }: { id?: string; from?: string | null; children: ReactNode }) {
  return (
    <div id={id} className="flex items-start gap-2 text-base">
      <Check aria-hidden className="mt-1 size-4 shrink-0 text-income" />
      <p className="min-w-0 break-words">
        {children}
        {from ? (
          <>
            {' '}
            <span className="ml-1 text-sm whitespace-nowrap text-muted-foreground">({from})</span>
          </>
        ) : null}
      </p>
    </div>
  )
}

/** under a guessed answer, the quiet way to the list of choices; "Looks right" is the footer's primary button */
export function AnswerActions({ onChoose }: { onChoose: () => void }) {
  const { t } = useTranslation()
  return (
    <button type="button" className="self-start text-sm text-muted-foreground underline-offset-2 hover:text-foreground hover:underline" onClick={onChoose}>
      {t('transactions.import_wizard.choose_differently')}
    </button>
  )
}

/** rows that will be left out: worth knowing, nothing the user must act on */
export function SkippedNote({ children }: { children: string }) {
  return (
    <p className="flex items-start gap-2 text-sm text-muted-foreground">
      <Info aria-hidden className="mt-0.5 size-4 shrink-0" />
      {children}
    </p>
  )
}
