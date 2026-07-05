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

export const cardFieldControlClass =
  'h-auto rounded-none border-0 bg-transparent p-0 text-sm shadow-none focus-visible:ring-0 dark:bg-transparent'
