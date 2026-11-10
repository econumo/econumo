import type { ReactNode } from 'react'
import { Label } from '@/components/ui/label'

// Card-style form field: tiny muted label inside a gray card, borderless
// control, focus ring on the whole card. Pair the inner Input/control with
// cardFieldControlClass so only the card carries the chrome.
export function CardField({
  label,
  htmlFor,
  error,
  children,
}: {
  label: string
  htmlFor?: string
  error?: string | null
  children: ReactNode
}) {
  return (
    <div className="flex flex-col gap-1">
      <div className="flex w-full flex-col gap-0.5 rounded-lg bg-econumo-card px-4 py-2.5 focus-within:ring-1 focus-within:ring-ring">
        <Label htmlFor={htmlFor} className="text-[11px] font-normal text-muted-foreground">
          {label}
        </Label>
        {children}
      </div>
      {error ? <p className="px-1 text-sm text-destructive">{error}</p> : null}
    </div>
  )
}

// The card is the field's frame, so a disabled control (offline) keeps no fill
// of its own: the shared input's disabled background would show as a bar.
export const cardFieldControlClass =
  'h-auto rounded-none border-0 bg-transparent p-0 text-sm shadow-none focus-visible:ring-0 dark:bg-transparent disabled:cursor-default disabled:bg-transparent dark:disabled:bg-transparent'

// The oversized borderless money input (the transaction dialog's amount field).
// Wrap the Input/CalculatorInput in a div with this class so the calculator
// keypad stays inside the same card as the input.
export const amountCardInputClass =
  '[&_input]:h-12 [&_input]:rounded-none [&_input]:border-0 [&_input]:bg-transparent [&_input]:px-0 [&_input]:text-[28px] [&_input]:font-light [&_input]:shadow-none [&_input]:focus-visible:ring-0 [&_input]:placeholder:text-muted-foreground/50 [&_input]:disabled:bg-transparent dark:[&_input]:disabled:bg-transparent'
