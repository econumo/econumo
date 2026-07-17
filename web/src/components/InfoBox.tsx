import type { ReactNode } from 'react'
import { Info } from 'lucide-react'

// Informational hint block for settings sections: soft card with a leading icon.
// mb-3 gives every page the same breathing room below the hint (it collapses
// with an mt-* the next block may carry, e.g. on the accounts page).
export function InfoBox({ children }: { children: ReactNode }) {
  return (
    <div className="mb-3 flex items-start gap-3 rounded-md bg-econumo-card px-3 py-2.5 text-sm text-muted-foreground">
      <Info className="mt-0.5 size-4 shrink-0 text-econumo-purple" aria-hidden="true" />
      <div className="min-w-0">{children}</div>
    </div>
  )
}
