import type { ComponentProps } from 'react'
import { Search } from 'lucide-react'
import { SearchClearButton } from '@/components/SearchClearButton'

// A phone picker sheet's search, passed as the sheet's pinned bottom row: the
// sheet rests on the keyboard, so it stays right above the keys however many
// rows the filter leaves
export function SheetSearch({ className, onClear, ...props }: ComponentProps<'input'> & { onClear?: () => void }) {
  const clearable = onClear !== undefined && props.value !== undefined && props.value !== ''
  return (
    <div className="relative">
      <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
      <input
        type="text"
        autoComplete="off"
        enterKeyHint="search"
        className={`h-11 w-full rounded-lg border border-input bg-transparent pl-9 text-base outline-none placeholder:text-muted-foreground focus:border-ring focus:ring-3 focus:ring-ring/50 ${clearable ? 'pr-11' : 'pr-3'} ${className ?? ''}`}
        {...props}
      />
      {clearable ? (
        <SearchClearButton
          className="absolute top-1/2 right-1 size-9 -translate-y-1/2"
          onClear={onClear}
        />
      ) : null}
    </div>
  )
}
