import { Info } from 'lucide-react'

/** rows that will be left out: worth knowing, nothing the user must act on */
export function SkippedNote({ children }: { children: string }) {
  return (
    <p className="flex items-start gap-2 text-sm text-muted-foreground">
      <Info aria-hidden className="mt-0.5 size-4 shrink-0" />
      {children}
    </p>
  )
}
