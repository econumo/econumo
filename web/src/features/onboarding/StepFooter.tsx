import { useEffect, useRef, useState, type ReactNode } from 'react'
import { cn } from '@/lib/utils'

// The step's Skip / primary buttons, pinned to the bottom of the page's scroll container so a long
// list never pushes them below the fold. It stays in the flow, so the last row is never hidden behind
// it; the top border shows only while content scrolls under it. The offset cancels the page's
// bottom padding, which the sticky position would otherwise leave as a strip rows show through.
export function StepFooter({ children }: { children: ReactNode }) {
  const sentinel = useRef<HTMLDivElement>(null)
  const [stuck, setStuck] = useState(false)
  useEffect(() => {
    const el = sentinel.current
    if (!el || typeof IntersectionObserver === 'undefined') return
    // the sentinel sits right below the bar: while it is out of view, content runs under the bar
    const observer = new IntersectionObserver(([entry]) => setStuck(!entry.isIntersecting))
    observer.observe(el)
    return () => observer.disconnect()
  }, [])
  return (
    <>
      <div
        data-testid="step-footer"
        data-stuck={stuck || undefined}
        className={cn(
          'sticky -bottom-4 z-10 -mx-4 mt-6 flex items-center justify-between border-t border-transparent bg-background px-4 pt-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] transition-colors lg:-mx-8 lg:px-8',
          stuck && 'border-border',
        )}
      >
        {children}
      </div>
      <div ref={sentinel} aria-hidden className="h-px shrink-0" />
    </>
  )
}
