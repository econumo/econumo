import { useId } from 'react'
import type { ReactNode } from 'react'
import { Info } from 'lucide-react'
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

/** the current answer as a settings row: the answer in plain words, where in the file it comes from, and a quiet Change */
export function AnswerRow({ id, from, onChange, children }: { id?: string; from?: string | null; onChange: () => void; children: ReactNode }) {
  const { t } = useTranslation()
  return (
    <div className="flex items-start gap-3 rounded-xl border border-border px-4 py-3">
      <div id={id} className="flex min-w-0 flex-1 flex-col gap-0.5">
        <div className="text-base break-words">{children}</div>
        {from ? <div className="text-xs text-muted-foreground">{from}</div> : null}
      </div>
      <QuietButton onClick={onChange}>{t('transactions.import_wizard.change')}</QuietButton>
    </div>
  )
}

/** closes a list of choices that takes more than one pick */
export function ChoicesDone({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex justify-end">
      <QuietButton onClick={onDone}>{t('transactions.import_wizard.done')}</QuietButton>
    </div>
  )
}

function QuietButton({ onClick, children }: { onClick: () => void; children: ReactNode }) {
  return (
    <button
      type="button"
      className="-my-1 -mr-2 shrink-0 rounded-md px-2 py-1 text-sm text-primary hover:bg-econumo-card focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
      onClick={onClick}
    >
      {children}
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
