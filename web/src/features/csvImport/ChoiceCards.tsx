import { useId } from 'react'

interface ChoiceCardsProps<T extends string> {
  label: string
  value: T | null
  options: { value: T; label: string }[]
  onChange: (value: T) => void
}

/** A plain question with a few answers, each a card to tap */
export function ChoiceCards<T extends string>({ label, value, options, onChange }: ChoiceCardsProps<T>) {
  const name = useId()
  return (
    <div role="radiogroup" aria-label={label} className="grid grid-cols-1 gap-2 sm:grid-cols-2">
      {options.map((option) => (
        <label
          key={option.value}
          className="flex min-w-0 cursor-pointer items-center rounded-lg border p-3 text-left text-sm transition-colors hover:bg-muted/50 has-[:checked]:border-primary has-[:checked]:bg-primary/5 has-[:focus-visible]:ring-3 has-[:focus-visible]:ring-ring/50"
        >
          <input type="radio" name={name} className="sr-only" checked={value === option.value} onChange={() => onChange(option.value)} />
          {option.label}
        </label>
      ))}
    </div>
  )
}
