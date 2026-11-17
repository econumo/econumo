import { useId } from 'react'

interface PillChoiceProps<T extends string> {
  label: string
  /** shows the question above the answers; otherwise it is only their accessible name */
  showLabel?: boolean
  value: T | null
  options: { value: T; label: string }[]
  onChange: (value: T) => void
}

/** a short question with small one-line answers in a row */
export function PillChoice<T extends string>({ label, showLabel = false, value, options, onChange }: PillChoiceProps<T>) {
  const name = useId()
  const labelId = useId()
  return (
    <div className="flex flex-col gap-2">
      {showLabel ? (
        <span id={labelId} className="text-sm">
          {label}
        </span>
      ) : null}
      <div role="radiogroup" aria-label={showLabel ? undefined : label} aria-labelledby={showLabel ? labelId : undefined} className="flex flex-wrap items-center gap-2">
        {options.map((option) => (
          <label
            key={option.value}
            className="cursor-pointer rounded-full border px-3 py-1.5 text-sm transition-colors hover:bg-muted/50 has-[:checked]:border-primary has-[:checked]:bg-primary/5 has-[:focus-visible]:ring-3 has-[:focus-visible]:ring-ring/50"
          >
            <input type="radio" name={name} className="sr-only" checked={value === option.value} onChange={() => onChange(option.value)} />
            {option.label}
          </label>
        ))}
      </div>
    </div>
  )
}
