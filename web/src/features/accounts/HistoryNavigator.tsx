import { useImperativeHandle, useRef, useState } from 'react'
import type { KeyboardEvent, PointerEvent, Ref } from 'react'
import { RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useIsPhone } from '@/hooks/useIsPhone'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import coins from '@/assets/econumo-coins.svg?inline'
import { formatDate, parseDateTime } from '@/lib/datetime'
import type { HistoryMonth } from './TransactionHistory'

const MARKER_WIDTH = 40
const monthNumber = (key: string) => Number(key.slice(0, 4)) * 12 + Number(key.slice(5, 7))
const clampPosition = (value: number) => Math.max(0, Math.min(1, value))

export interface HistoryNavigatorHandle {
  resetPosition: () => void
}

export function HistoryNavigator({ months, activeMonth, onJump, onToday, ref }: {
  ref?: Ref<HistoryNavigatorHandle>
  months: HistoryMonth[]
  activeMonth: string
  onJump: (month: HistoryMonth) => void
  onToday: () => void
}) {
  const { t, i18n } = useTranslation()
  const isPhone = useIsPhone()
  const [open, setOpen] = useState(false)
  const [year, setYear] = useState(activeMonth.slice(0, 4))
  const [previewIndex, setPreviewIndex] = useState<number | null>(null)
  const [markerPosition, setMarkerPosition] = useState<number | null>(null)
  const [dateVisible, setDateVisible] = useState(false)
  const markerRef = useRef<HTMLButtonElement>(null)
  const suppressClickRef = useRef(false)
  const dragRef = useRef<{
    startX: number
    grabOffset: number
    startPosition: number
    fromLabel: boolean
    moved: boolean
    capture: HTMLElement
  } | null>(null)
  const todayMonth = formatDate(new Date()).slice(0, 7)
  // The left endpoint always means today, even on an inactive account. Future
  // entries remain reachable by scrolling or through the exact month picker.
  const timeline = [todayMonth, ...months.filter((month) => month.key < todayMonth).map((month) => month.key)]
  const activeIndex = Math.max(0, timeline.indexOf(activeMonth))
  const shownIndex = Math.min(previewIndex ?? activeIndex, timeline.length - 1)
  const newest = monthNumber(todayMonth)
  const span = Math.max(1, newest - monthNumber(timeline.at(-1)!))
  const positionForMonth = (key: string) => clampPosition((newest - monthNumber(key)) / span)
  // Pointer position is continuous and survives the programmatic scroll.
  // Only an explicit scroll gesture or picker selection repositions it.
  const fraction = markerPosition ?? positionForMonth(activeMonth)
  useImperativeHandle(ref, () => ({ resetPosition: () => setMarkerPosition(null) }), [])
  const shownMonth = previewIndex === null ? activeMonth : timeline[shownIndex]
  const years = [...new Set(months.map((month) => month.key.slice(0, 4)))]
  const monthLabel = (key: string) => new Intl.DateTimeFormat(i18n.language, {
    month: 'long', year: 'numeric',
  }).format(parseDateTime(`${key}-01`))

  const closestIndex = (position: number) => {
    const target = newest - position * span
    return timeline.reduce((closest, key, index) =>
      Math.abs(monthNumber(key) - target) < Math.abs(monthNumber(timeline[closest]) - target) ? index : closest, 0)
  }
  const jumpToPosition = (position: number) => {
    const next = clampPosition(position)
    setMarkerPosition(next)
    setPreviewIndex(null)
    const index = closestIndex(next)
    if (index === 0) onToday()
    else {
      const month = months.find((item) => item.key === timeline[index])
      if (month) onJump(month)
    }
  }
  const pickPosition = (event: PointerEvent<HTMLElement>) => {
    const rect = event.currentTarget.getBoundingClientRect()
    const width = rect.width - MARKER_WIDTH
    const offset = dragRef.current?.grabOffset ?? 0
    return width > 0 ? clampPosition((event.clientX - rect.left - offset - MARKER_WIDTH / 2) / width) : 0
  }
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    let next = fraction
    switch (event.key) {
      case 'ArrowRight': case 'ArrowDown': next += 0.01; break
      case 'ArrowLeft': case 'ArrowUp': next -= 0.01; break
      case 'PageDown': next += 0.1; break
      case 'PageUp': next -= 0.1; break
      case 'Home': next = 0; break
      case 'End': next = 1; break
      case 'Enter': case ' ': setYear(activeMonth.slice(0, 4)); setOpen(true); event.preventDefault(); return
      default: return
    }
    event.preventDefault()
    jumpToPosition(next)
  }

  const onOpenChange = (value: boolean) => {
    if (value) setYear(activeMonth.slice(0, 4))
    setDateVisible(false)
    setOpen(value)
  }
  const trigger = (
    <Button
      ref={markerRef}
      type="button"
      variant="ghost"
      size="sm"
      className="absolute top-full h-6 max-sm:h-11 w-10 max-w-full touch-none cursor-grab px-0 transition-none active:cursor-grabbing"
      style={{ left: `calc(${fraction * 100}% - ${fraction * MARKER_WIDTH}px)`, transform: 'translateY(-50%)' }}
      aria-label={t('accounts.page.history.jump')}
      aria-haspopup="dialog"
      aria-expanded={open}
      onClick={isPhone ? () => onOpenChange(true) : undefined}
      aria-description={monthLabel(shownMonth)}
    >
      <img src={coins} alt="" draggable={false} className="pointer-events-none h-2.5 w-auto bg-background select-none" />
      <span className="sr-only">{monthLabel(shownMonth)}</span>
    </Button>
  )
  const marker = (
    <TooltipProvider>
      <Tooltip disableHoverableContent open={!isPhone && !open && (dateVisible || previewIndex !== null)} onOpenChange={setDateVisible}>
        <TooltipTrigger asChild>
          {isPhone ? trigger : <PopoverTrigger asChild>{trigger}</PopoverTrigger>}
        </TooltipTrigger>
        <TooltipContent side="bottom" sideOffset={5}>{monthLabel(shownMonth)}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
  const picker = (
    <div className="grid gap-3">
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium">{t(isPhone ? 'accounts.page.history.year' : 'accounts.page.history.jump')}</span>
        <NativeSelect aria-label={t('accounts.page.history.year')} className={isPhone ? '[&_select]:h-11 [&_select]:text-base' : undefined} value={year} onChange={(event) => setYear(event.target.value)}>
          {years.map((value) => <NativeSelectOption key={value} value={value}>{value}</NativeSelectOption>)}
        </NativeSelect>
      </div>
      <div className="grid grid-cols-3 gap-1">
        {Array.from({ length: 12 }, (_, index) => {
          const key = `${year}-${String(index + 1).padStart(2, '0')}`
          const month = months.find((item) => item.key === key)
          return (
            <Button key={key} type="button" variant={activeMonth === key ? 'secondary' : 'ghost'} className={isPhone ? 'h-11 text-base' : 'h-10'} aria-label={monthLabel(key)} aria-current={activeMonth === key ? 'date' : undefined} disabled={!month} onClick={() => {
              setMarkerPosition(positionForMonth(key))
              if (month) onJump(month)
              setOpen(false)
            }}>
              {new Intl.DateTimeFormat(i18n.language, { month: 'short' }).format(parseDateTime(`${key}-01`))}
            </Button>
          )
        })}
      </div>
      <Button type="button" variant="ghost" size="sm" className={isPhone ? 'h-11 w-full justify-start border-t text-base text-muted-foreground' : 'justify-start border-t pt-2 text-xs text-muted-foreground'} onClick={() => {
        setMarkerPosition(0)
        onToday()
        setOpen(false)
      }} aria-label={t('accounts.page.history.back_today')}>
        <RotateCcw className="size-3.5" />
        {t('accounts.page.history.back_today')}
      </Button>
    </div>
  )

  return (
    <nav
      aria-label={t('accounts.page.history.navigation')}
      className={`relative z-10 h-full min-h-px w-full ${previewIndex === null ? 'cursor-pointer' : 'cursor-grabbing'}`}
      onPointerDown={(event) => {
        // Portaled picker events bubble through React, but belong to the picker.
        if (open || event.button !== 0 || !event.currentTarget.contains(event.target as Node)) return
        suppressClickRef.current = false
        const fromLabel = markerRef.current?.contains(event.target as Node) ?? false
        const capture = fromLabel ? markerRef.current! : event.currentTarget
        const markerRect = markerRef.current?.getBoundingClientRect()
        const grabOffset = fromLabel && markerRect ? event.clientX - markerRect.left - markerRect.width / 2 : 0
        dragRef.current = { startX: event.clientX, grabOffset, startPosition: fraction, fromLabel, moved: false, capture }
        capture.setPointerCapture(event.pointerId)
      }}
      onPointerMove={(event) => {
        const drag = dragRef.current
        if (!drag) return
        if (Math.abs(event.clientX - drag.startX) > 4) drag.moved = true
        if (drag.moved) {
          const position = pickPosition(event)
          setMarkerPosition(position)
          setPreviewIndex(closestIndex(position))
        }
      }}
      onPointerUp={(event) => {
        const drag = dragRef.current
        if (!drag) return
        if (drag.moved || !drag.fromLabel) {
          const position = pickPosition(event)
          jumpToPosition(position)
          setDateVisible(true)
        }
        dragRef.current = null
        suppressClickRef.current = drag.moved
        if (drag.capture.hasPointerCapture(event.pointerId)) drag.capture.releasePointerCapture(event.pointerId)
      }}
      onPointerCancel={() => {
        if (dragRef.current) setMarkerPosition(dragRef.current.startPosition)
        dragRef.current = null
        setPreviewIndex(null)
      }}
      onLostPointerCapture={() => { dragRef.current = null; setPreviewIndex(null) }}
      onClickCapture={(event) => {
        if (!suppressClickRef.current) return
        suppressClickRef.current = false
        event.preventDefault()
        event.stopPropagation()
      }}
    >
      <div
        role="slider"
        tabIndex={0}
        aria-label={t('accounts.page.history.navigation')}
        aria-orientation="horizontal"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={fraction * 100}
        aria-valuetext={monthLabel(shownMonth)}
        onKeyDown={onKeyDown}
        className="absolute inset-x-0 -bottom-1 h-2 touch-none rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
      {isPhone ? (
        <>
          {marker}
          <ResponsiveDialog
            open={open}
            onOpenChange={onOpenChange}
            title={t('accounts.page.history.jump')}
            onCloseAutoFocus={(event) => {
              event.preventDefault()
              markerRef.current?.focus()
            }}
            footer={<Button type="button" variant="secondary" className="w-full" onClick={() => onOpenChange(false)}>{t('common.button.cancel.label')}</Button>}
          >
            {picker}
          </ResponsiveDialog>
        </>
      ) : (
        <Popover open={open} onOpenChange={onOpenChange}>
          {marker}
          <PopoverContent align="start" side="bottom" aria-label={t('accounts.page.history.jump')} className="w-64 p-3">
            {picker}
          </PopoverContent>
        </Popover>
      )}
    </nav>
  )
}
