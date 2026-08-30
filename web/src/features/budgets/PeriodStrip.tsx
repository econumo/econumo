import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { MONTHS_AROUND, periodRange } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'

const EXTEND_STEP = 12
const EDGE_THRESHOLD_PX = 300
// roughly a screenful of month chips per arrow click
const SCROLL_STEP_PX = 320

export function PeriodStrip({ startedAt, endedAt = null }: { startedAt: string | null; endedAt?: string | null }) {
  const { t, i18n } = useTranslation()
  const selectedDate = useBudgetPeriodStore((s) => s.selectedDate)
  const setPeriod = useBudgetPeriodStore((s) => s.setPeriod)
  const containerRef = useRef<HTMLDivElement | null>(null)
  const activeRef = useRef<HTMLButtonElement | null>(null)
  // scrollWidth captured right before a prepend; consumed to keep the viewport still
  const prependAnchor = useRef<number | null>(null)

  const [extend, setExtend] = useState({ before: 0, after: 0 })

  const allItems = periodRange(selectedDate, startedAt, MONTHS_AROUND + extend.before, MONTHS_AROUND + extend.after, i18n.language, endedAt)
  // Months before the start stay browsable so past spending can be reviewed
  // while tuning a budget. The reader handles them safely — the carry-over walk
  // is [startedAt, period), so it is simply empty — and they are read-only: the
  // server rejects set-limit before the start month and canUpdateLimits mirrors
  // it. They render dimmed via outsideBudget.
  // Past the end month is different: an ended budget covers no later month, so
  // those stay hidden. The active month is always kept, even when the stored
  // selection points outside the window.
  const items = allItems.filter((item) => !item.afterEnd || item.isActive)
  const canExtendBefore = true
  const canExtendAfter = allItems.length > 0 && !allItems[allItems.length - 1].afterEnd
  // The desktop arrows pan the strip only — the selected month never moves, so
  // the table below stays put while you look around (same idea as the plan
  // sheet's nav shifting its window without moving the selection). Scrolling
  // triggers handleScroll, so the window keeps extending at either edge.
  // Assign scrollLeft rather than scrollBy({behavior:'smooth'}): smooth scrolling
  // is a no-op under prefers-reduced-motion (and in jsdom), which would leave the
  // arrows dead. handleScroll still fires, so the window extends at either edge.
  const scrollBy = (delta: number) => {
    const el = containerRef.current
    if (el) {
      el.scrollLeft += delta
    }
  }

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
    if (canExtendBefore && el.scrollLeft < EDGE_THRESHOLD_PX && prependAnchor.current === null) {
      prependAnchor.current = el.scrollWidth
      setExtend((e) => ({ ...e, before: e.before + EXTEND_STEP }))
    } else if (canExtendAfter && el.scrollWidth - el.scrollLeft - el.clientWidth < EDGE_THRESHOLD_PX) {
      setExtend((e) => ({ ...e, after: e.after + EXTEND_STEP }))
    }
  }

  return (
    <div className="flex items-center gap-1">
      {/* desktop-only month steppers, mirroring the plan sheet's nav; touch
          viewports scroll the strip directly */}
      <div className="hidden shrink-0 items-center gap-1 md:flex">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-8"
          aria-label={t('budgets.page.budget.nav.prev')}
          onClick={() => scrollBy(-SCROLL_STEP_PX)}
        >
          <ChevronLeft className="size-4" />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-8"
          aria-label={t('budgets.page.budget.nav.next')}
          onClick={() => scrollBy(SCROLL_STEP_PX)}
        >
          <ChevronRight className="size-4" />
        </Button>
      </div>
      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="flex flex-1 gap-1 overflow-x-auto py-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
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
    </div>
  )
}
