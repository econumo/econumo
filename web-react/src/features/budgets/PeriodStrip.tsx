import { useEffect, useRef } from 'react'
import { periodRange } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'

export function PeriodStrip({ startedAt }: { startedAt: string | null }) {
  const selectedDate = useBudgetPeriodStore((s) => s.selectedDate)
  const setPeriod = useBudgetPeriodStore((s) => s.setPeriod)
  const activeRef = useRef<HTMLButtonElement | null>(null)

  const items = periodRange(selectedDate, startedAt)

  useEffect(() => {
    activeRef.current?.scrollIntoView({ inline: 'center', block: 'nearest' })
  }, [selectedDate])

  return (
    <div className="flex gap-1 overflow-x-auto py-2" role="tablist" aria-label="period">
      {items.map((item) => (
        <button
          key={item.value}
          ref={item.isActive ? activeRef : undefined}
          type="button"
          role="tab"
          aria-selected={item.isActive}
          className={`shrink-0 rounded-full px-3 py-1 text-sm ${
            item.isActive ? 'bg-primary text-primary-foreground' : item.outsideBudget ? 'text-muted-foreground/50' : 'text-muted-foreground hover:bg-accent'
          }`}
          onClick={() => setPeriod(item.value)}
        >
          {item.label}
        </button>
      ))}
    </div>
  )
}
