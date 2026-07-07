import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { MONTHS_AROUND, periodRange } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'

const EXTEND_STEP = 12
const EDGE_THRESHOLD_PX = 300

export function PeriodStrip({ startedAt }: { startedAt: string | null }) {
  const selectedDate = useBudgetPeriodStore((s) => s.selectedDate)
  const setPeriod = useBudgetPeriodStore((s) => s.setPeriod)
  const containerRef = useRef<HTMLDivElement | null>(null)
  const activeRef = useRef<HTMLButtonElement | null>(null)
  // scrollWidth captured right before a prepend; consumed to keep the viewport still
  const prependAnchor = useRef<number | null>(null)

  const [extend, setExtend] = useState({ before: 0, after: 0 })

  const items = periodRange(selectedDate, startedAt, MONTHS_AROUND + extend.before, MONTHS_AROUND + extend.after)

  useLayoutEffect(() => {
    activeRef.current?.scrollIntoView({ inline: 'center', block: 'nearest' })
  }, [selectedDate])

  useLayoutEffect(() => {
    const el = containerRef.current
    if (el && prependAnchor.current !== null) {
      el.scrollLeft += el.scrollWidth - prependAnchor.current
      prependAnchor.current = null
    }
  }, [extend.before])

  // the scrollbar is hidden, so plain mouse wheels need vertical→horizontal
  // translation (native listener: React's wheel handlers are passive)
  useEffect(() => {
    const el = containerRef.current
    if (!el) {
      return
    }
    const onWheel = (e: WheelEvent) => {
      const delta = Math.abs(e.deltaX) > Math.abs(e.deltaY) ? e.deltaX : e.deltaY
      if (delta !== 0) {
        el.scrollLeft += delta
        e.preventDefault()
      }
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [])

  const handleScroll = () => {
    const el = containerRef.current
    if (!el || el.clientWidth === 0) {
      return
    }
    if (el.scrollLeft < EDGE_THRESHOLD_PX && prependAnchor.current === null) {
      prependAnchor.current = el.scrollWidth
      setExtend((e) => ({ ...e, before: e.before + EXTEND_STEP }))
    } else if (el.scrollWidth - el.scrollLeft - el.clientWidth < EDGE_THRESHOLD_PX) {
      setExtend((e) => ({ ...e, after: e.after + EXTEND_STEP }))
    }
  }

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      className="flex gap-1 overflow-x-auto py-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      role="tablist"
      aria-label="period"
    >
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
          onClick={() => {
            // recenter the window on the new month so both directions stay deep
            setExtend({ before: 0, after: 0 })
            setPeriod(item.value)
          }}
        >
          {item.label}
        </button>
      ))}
    </div>
  )
}
