import { useEffect, useState } from 'react'
import { Carousel, CarouselContent, CarouselItem } from '@/components/ui/carousel'
import type { CarouselApi } from '@/components/ui/carousel'
import { EntityIcon } from '@/components/EntityIcon'
import { availableIcons } from '@/lib/icons'

// 9 columns × 4 rows per slide
const PAGE_SIZE = 36

const pages: string[][] = []
for (let i = 0; i < availableIcons.length; i += PAGE_SIZE) {
  pages.push(availableIcons.slice(i, i + PAGE_SIZE))
}

interface IconPickerProps {
  value: string
  onChange: (icon: string) => void
  'aria-label': string
}

// Swipeable icon pages with dot navigation — no scrollbar in the dialog.
// Opens on the page holding the current icon.
export function IconPicker({ value, onChange, 'aria-label': ariaLabel }: IconPickerProps) {
  const [api, setApi] = useState<CarouselApi>()
  const [startPage] = useState(() => Math.max(0, pages.findIndex((page) => page.includes(value))))
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

  return (
    <div role="listbox" aria-label={ariaLabel}>
      <Carousel setApi={setApi} opts={{ startIndex: startPage }}>
        <CarouselContent>
          {pages.map((pageIcons, i) => (
            <CarouselItem key={i}>
              <div className="grid min-h-38 grid-cols-9 content-start gap-1">
                {pageIcons.map((iconName) => (
                  <button
                    key={iconName}
                    type="button"
                    role="option"
                    aria-selected={value === iconName}
                    aria-label={iconName}
                    title={iconName}
                    className={`flex items-center justify-center rounded-md p-1.5 hover:bg-accent ${
                      value === iconName ? 'bg-accent ring-1 ring-ring' : ''
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
    </div>
  )
}
