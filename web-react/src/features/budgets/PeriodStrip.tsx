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
          className={`shrink-0 px-2.5 py-1 text-sm uppercase tracking-wide ${
            item.isActive
              ? 'font-bold text-foreground'
              : item.outsideBudget
                ? 'text-[#E6E6E6] hover:text-muted-foreground'
                : 'text-[#999999] hover:text-foreground'
          }`}
          onClick={() => setPeriod(item.value)}
        >
          {item.label}
        </button>
      ))}
    </div>
  )
}
