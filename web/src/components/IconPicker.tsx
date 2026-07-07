import { useEffect, useMemo, useRef, useState } from 'react'
import { Carousel, CarouselContent, CarouselItem } from '@/components/ui/carousel'
import type { CarouselApi } from '@/components/ui/carousel'
import { EntityIcon } from '@/components/EntityIcon'
import { availableIcons } from '@/lib/icons'

const COLS = 9
const BASE_ROWS = 4
// cell = 20px glyph + 2×6px padding; rows are gap-1 (4px) apart; the dot strip
// below the carousel is pt-2 (8px) + 8px dots
const CELL_H = 32
const GAP = 4
const DOTS_H = 16

interface IconPickerProps {
  value: string
  onChange: (icon: string) => void
  'aria-label': string
  /** grow into the height the parent allocates (full-screen mobile forms):
      free space becomes more icon rows per page instead of dead space */
  fill?: boolean
}

// Swipeable icon pages with dot navigation — no scrollbar in the dialog.
// Opens on the page holding the current icon.
export function IconPicker({ value, onChange, 'aria-label': ariaLabel, fill = false }: IconPickerProps) {
  const [api, setApi] = useState<CarouselApi>()
  const measureRef = useRef<HTMLDivElement>(null)
  const [rows, setRows] = useState(BASE_ROWS)

  useEffect(() => {
    if (!fill) {
      return
    }
    const el = measureRef.current
    if (!el) {
      return
    }
    const measure = () => {
      const avail = el.clientHeight - DOTS_H
      setRows(Math.max(BASE_ROWS, Math.floor((avail + GAP) / (CELL_H + GAP))))
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(el)
    return () => observer.disconnect()
  }, [fill])

  const pages = useMemo(() => {
    const pageSize = COLS * rows
    const result: string[][] = []
    for (let i = 0; i < availableIcons.length; i += pageSize) {
      result.push(availableIcons.slice(i, i + pageSize))
    }
    return result
  }, [rows])
  // the page holding the current icon — embla only reads startIndex on init,
  // so a rows-change remount (key) re-anchors there too
  const startPage = Math.max(
    0,
    pages.findIndex((page) => page.includes(value)),
  )
  const [page, setPage] = useState(startPage)

  useEffect(() => {
    if (!api) {
      return
    }
    const onSelect = () => setPage(api.selectedScrollSnap())
    api.on('select', onSelect)
    onSelect()
    return () => {
      api.off('select', onSelect)
    }
  }, [api])

  const picker = (
    <>
      <Carousel key={rows} setApi={setApi} opts={{ startIndex: startPage }} className={fill ? 'min-h-0 flex-1' : undefined}>
        <CarouselContent>
          {pages.map((pageIcons, i) => (
            <CarouselItem key={i}>
              <div className={`grid grid-cols-9 content-start gap-1 ${fill ? '' : 'min-h-38'}`}>
                {pageIcons.map((iconName) => (
                  <button
                    key={iconName}
                    type="button"
                    role="option"
                    aria-selected={value === iconName}
                    aria-label={iconName}
                    title={iconName}
                    // ring-inset: an outer ring gets clipped by the carousel's
                    // overflow on the first column/row (reads as cut corners)
                    className={`flex items-center justify-center rounded-md p-1.5 hover:bg-accent ${
                      value === iconName ? 'bg-accent ring-1 ring-ring ring-inset' : ''
                    }`}
                    onClick={() => onChange(iconName)}
                  >
                    <EntityIcon name={iconName} className="text-xl" />
                  </button>
                ))}
              </div>
            </CarouselItem>
          ))}
        </CarouselContent>
      </Carousel>
      <div className="flex justify-center gap-1.5 pt-2">
        {pages.map((_, i) => (
          <button
            key={i}
            type="button"
            aria-label={`icons page ${i + 1}`}
            title={`${i + 1} / ${pages.length}`}
            className={`size-2 rounded-full transition-colors ${i === page ? 'bg-econumo-magenta' : 'bg-border hover:bg-muted-foreground/40'}`}
            onClick={() => api?.scrollTo(i)}
          />
        ))}
      </div>
    </>
  )

  if (fill) {
    // The measured layer is absolute so its height is the flex allocation, never
    // its own content height — measuring in-flow would feed the ResizeObserver
    // its own row growth. min-h-40 keeps the BASE_ROWS fallback usable.
    return (
      <div className="relative min-h-40 flex-1">
        <div ref={measureRef} role="listbox" aria-label={ariaLabel} className="absolute inset-0 flex flex-col">
          {picker}
        </div>
      </div>
    )
  }
  return (
    <div role="listbox" aria-label={ariaLabel}>
      {picker}
    </div>
  )
}
