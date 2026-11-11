import type { ReactNode } from 'react'

export function InboxSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-2">
      <h2 className="px-1 text-xs uppercase text-muted-foreground">{title}</h2>
      {children}
    </section>
  )
}
