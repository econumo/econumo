import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import type { DailyListEntry } from '@/features/transactions/useAccountTransactions'
import { HistoryNavigator } from './HistoryNavigator'
import type { HistoryNavigatorHandle } from './HistoryNavigator'

const LIST_CHUNK = 100

export interface HistoryMonth {
  key: string
  index: number
}

function entryKey(entry: DailyListEntry): string {
  if (entry.kind === 'separator') return `sep-${entry.day}`
  const tx = entry.transaction
  return tx.recurring ? `${tx.id}@${tx.date}` : tx.id
}

/** Jumping replaces the rendered section instead of mounting every intervening
 * transaction. Sentinels let ordinary scrolling continue in either direction. */
export function TransactionHistory({ entries, anchorIndex, ready, navigationTarget, children }: {
  entries: DailyListEntry[]
  anchorIndex: number
  ready: boolean
  /** The account header's month selector slot, above its divider. */
  navigationTarget?: HTMLElement | null
  children: (entry: DailyListEntry) => ReactNode
}) {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement>(null)
  const navigatorRef = useRef<HistoryNavigatorHandle>(null)
  const topRef = useRef<HTMLDivElement>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const anchoredRef = useRef(false)
  const pendingRef = useRef<{ index: number; offset: number; align?: boolean } | null>(null)
  const [range, setRange] = useState({ start: 0, end: LIST_CHUNK })
  const [activeMonth, setActiveMonth] = useState('')
  const [trailingSpace, setTrailingSpace] = useState(0)
  const months = useMemo(() => {
    const result: HistoryMonth[] = []
    entries.forEach((entry, index) => {
      if (entry.kind !== 'separator') return
      const key = entry.day.slice(0, 7)
      if (result.at(-1)?.key !== key) result.push({ key, index })
    })
    return result
  }, [entries])

  const monthAt = useCallback((index: number) => months.findLast((month) => month.index <= index)?.key ?? '', [months])
  const rowAt = useCallback((index: number) => containerRef.current?.querySelector<HTMLElement>(`[data-history-index="${index}"]`), [])

  const jumpTo = (index: number) => {
    if (index < 0) return
    anchoredRef.current = true
    pendingRef.current = { index, offset: 0, align: true }
    setActiveMonth(monthAt(index))
    setRange({ start: Math.max(0, index - LIST_CHUNK), end: index + LIST_CHUNK })
  }

  useLayoutEffect(() => {
    const container = containerRef.current
    if (!container) return
    if (!anchoredRef.current && ready && entries.length > 0) {
      const index = Math.max(0, anchorIndex)
      pendingRef.current = { index, offset: 0, align: true }
      anchoredRef.current = true
      setActiveMonth(monthAt(index))
      if (index >= range.end) {
        setRange({ start: Math.max(0, index - LIST_CHUNK), end: index + LIST_CHUNK })
        return
      }
    }
    const pending = pendingRef.current
    if (!pending) return
    const row = rowAt(pending.index)
    if (!row) return
    if (pending.align) {
      // A sparse oldest month still needs to land at the top. Add only the
      // space needed below it, rather than letting the browser clamp the jump
      // and leave a newer month at the top of the viewport.
      const lastRow = rowAt(Math.min(range.end, entries.length) - 1)
      const below = lastRow ? lastRow.getBoundingClientRect().bottom - row.getBoundingClientRect().top + 16 : 0
      const needed = range.end >= entries.length ? Math.max(0, container.clientHeight - below) : 0
      if (Math.abs(needed - trailingSpace) > 1) {
        setTrailingSpace(needed)
        return
      }
    }
    container.scrollTop += row.getBoundingClientRect().top - container.getBoundingClientRect().top - pending.offset
    pendingRef.current = null
  }, [ready, entries.length, anchorIndex, range, monthAt, rowAt, trailingSpace])

  useEffect(() => {
    const container = containerRef.current
    if (!container || (!topRef.current && !bottomRef.current)) return
    let connected = true
    const observer = new IntersectionObserver((hits) => {
      if (!connected || pendingRef.current) return
      const prepend = hits.some((hit) => hit.isIntersecting && hit.target === topRef.current)
      const append = hits.some((hit) => hit.isIntersecting && hit.target !== topRef.current)
      if (prepend && range.start > 0) {
        const row = rowAt(range.start)
        if (row) {
          pendingRef.current = {
            index: range.start,
            offset: row.getBoundingClientRect().top - container.getBoundingClientRect().top,
          }
        }
      }
      if (prepend || append) {
        setRange((current) => ({
          start: prepend ? Math.max(0, current.start - LIST_CHUNK) : current.start,
          end: append ? Math.min(entries.length, current.end + LIST_CHUNK) : current.end,
        }))
      }
    }, { root: container, rootMargin: '600px' })
    if (topRef.current) observer.observe(topRef.current)
    if (bottomRef.current) observer.observe(bottomRef.current)
    return () => { connected = false; observer.disconnect() }
  }, [range, entries.length, rowAt])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    let frame = 0
    const update = () => {
      frame = 0
      // There is no layout in jsdom or while the account panel is hidden.
      if (!container.clientHeight || pendingRef.current) return
      const rows = container.querySelectorAll<HTMLElement>('[data-history-index]')
      const top = container.getBoundingClientRect().top + 8
      let low = 0
      let high = rows.length - 1
      while (low < high) {
        const mid = Math.floor((low + high) / 2)
        if (rows[mid].getBoundingClientRect().bottom <= top) low = mid + 1
        else high = mid
      }
      if (rows[low]) setActiveMonth(monthAt(Number(rows[low].dataset.historyIndex)))
    }
    const onScroll = () => {
      if (!frame) frame = requestAnimationFrame(update)
    }
    container.addEventListener('scroll', onScroll, { passive: true })
    const resize = new ResizeObserver(onScroll)
    resize.observe(container)
    onScroll()
    return () => {
      container.removeEventListener('scroll', onScroll)
      resize.disconnect()
      cancelAnimationFrame(frame)
    }
  }, [monthAt, range])

  const navigation = months.length > 0 ? (
    <HistoryNavigator
      ref={navigatorRef}
      months={months}
      activeMonth={months.some((month) => month.key === activeMonth) ? activeMonth : months[0].key}
      onJump={(month) => jumpTo(month.index)}
      onToday={() => jumpTo(Math.max(0, anchorIndex))}
    />
  ) : null

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {navigationTarget ? createPortal(navigation, navigationTarget) : navigation}
      <div
        ref={containerRef}
        data-testid="transaction-history"
        className="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain pb-4"
        onWheel={() => navigatorRef.current?.resetPosition()}
        onTouchStart={() => navigatorRef.current?.resetPosition()}
        onPointerDown={(event) => { if (event.target === event.currentTarget) navigatorRef.current?.resetPosition() }}
        onKeyDown={(event) => {
          if (['ArrowDown', 'ArrowUp', 'PageDown', 'PageUp', 'Home', 'End', ' '].includes(event.key)) navigatorRef.current?.resetPosition()
        }}
      >
        {range.start > 0 ? <div ref={topRef} data-history-boundary="before" aria-hidden="true" className="h-px" /> : null}
        {entries.slice(range.start, range.end).map((entry, offset) => {
          const index = range.start + offset
          return (
            <div key={entryKey(entry)} data-history-index={index}>
              {index === anchorIndex && anchorIndex > 0 ? <div data-testid="future-anchor" aria-hidden="true" /> : null}
              {children(entry)}
            </div>
          )
        })}
        {range.end < entries.length ? <div ref={bottomRef} data-history-boundary="after" aria-hidden="true" className="h-px" /> : null}
        {trailingSpace > 0 ? <div data-history-trailing-space aria-hidden="true" style={{ height: trailingSpace }} /> : null}
        {entries.length === 0 && ready ? <p className="p-4 text-center text-sm text-muted-foreground">{t('accounts.page.transaction_list.none')}</p> : null}
      </div>
    </div>
  )
}
