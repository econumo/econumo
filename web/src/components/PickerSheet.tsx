import { useEffect, useRef, useState } from 'react'
import type { KeyboardEvent, ReactNode } from 'react'
import { Command, CommandInput, CommandList } from '@/components/ui/command'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { SheetSearch } from '@/components/SheetSearch'
import { useIsPhone } from '@/hooks/useIsPhone'
import { cn } from '@/lib/utils'

// invisible fields around the phone search, where iOS's ^ / v keys land. Not
// pointer-events: none: iOS only stops on a field it hit-tests as uncovered
const NAV_STOP = 'size-px opacity-0'

interface PickerSheetProps {
  open: boolean
  onClose: () => void
  title: string
  description?: string
  /** the search's accessible name; also its placeholder unless one is given */
  searchLabel: string
  searchPlaceholder?: string
  /** short lists can leave the search out */
  searchable?: boolean
  /** a search-first picker raises the keyboard on open; a checklist does not */
  focusSearch?: boolean
  /** phone: Enter on a typed query before any arrow key, for the picker's best
      match. Without it, after ↑ / ↓, and on a desktop (cmdk's own input),
      Enter picks the highlighted row */
  onSearchEnter?: (query: string) => void
  /** iOS's ^ / v keys from the search carry on through the form behind */
  onNavigate?: (step: 1 | -1) => void
  /** above the list */
  header?: ReactNode
  /** the rows (CommandItem / CommandGroup / CommandEmpty) for the query */
  children: (query: string) => ReactNode
  /** below the list */
  after?: (query: string) => ReactNode
  /** the pinned action row, under the search on a phone */
  actions?: ReactNode
  listLabel?: string
  listClassName?: string
  onCloseAutoFocus?: (e: Event) => void
}

/**
 * The one searchable picker sheet. On a phone the sheet rests on the keyboard,
 * so its search is the pinned bottom row, right above the keys, with the list
 * above it, and it stays put however many rows the filter leaves. On a desktop
 * the search sits above the list. Rows are cmdk items either way, so ↑ / ↓ and
 * Enter behave the same in every picker.
 */
export function PickerSheet({
  open,
  onClose,
  title,
  description,
  searchLabel,
  searchPlaceholder,
  searchable = true,
  focusSearch = true,
  onSearchEnter,
  onNavigate,
  header,
  children,
  after,
  actions,
  listLabel,
  listClassName,
  onCloseAutoFocus,
}: PickerSheetProps) {
  const isPhone = useIsPhone()
  const [query, setQuery] = useState('')
  // ↑ / ↓ since the query last changed: Enter then means the highlighted row
  const [navigated, setNavigated] = useState(false)
  const searchRef = useRef<HTMLInputElement>(null)
  const commandRef = useRef<HTMLDivElement>(null)

  // a reopened picker starts from the full list
  useEffect(() => {
    if (open) {
      setQuery('')
      setNavigated(false)
    }
  }, [open])

  const changeQuery = (next: string) => {
    setQuery(next)
    setNavigated(false)
  }
  // the rows are filtered here, not by cmdk, which then keeps highlighting the
  // row it had: each query starts again from its best match, the first row
  useEffect(() => {
    commandRef.current?.dispatchEvent(new globalThis.KeyboardEvent('keydown', { key: 'Home', bubbles: true }))
  }, [query])

  // the phone search lives outside the cmdk root (in the sheet's pinned row),
  // so its keys are handed to cmdk, which ignores an IME's composing Enter
  const onPhoneKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.nativeEvent.isComposing || e.keyCode === 229) {
      return
    }
    if (e.key === 'Enter' && onSearchEnter && !navigated) {
      e.preventDefault()
      onSearchEnter(query)
      return
    }
    if (e.key === 'Enter' || e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      commandRef.current?.dispatchEvent(new globalThis.KeyboardEvent('keydown', { key: e.key, bubbles: true }))
      if (e.key !== 'Enter') {
        setNavigated(true)
      }
    }
  }

  const phoneSearch = searchable ? (
    <div className="relative">
      {/* focused as the sheet opens, still inside the tap, so iOS raises the keyboard */}
      <SheetSearch
        ref={searchRef}
        aria-label={searchLabel}
        placeholder={searchPlaceholder ?? searchLabel}
        value={query}
        onChange={(e) => changeQuery(e.target.value)}
        onKeyDown={onPhoneKeyDown}
      />
      {onNavigate ? <input aria-hidden inputMode="none" className={`${NAV_STOP} absolute top-0 left-0`} onFocus={() => onNavigate(1)} /> : null}
    </div>
  ) : null
  const footer = isPhone ? (
    phoneSearch || actions ? (
      <div className="flex flex-col gap-3">
        {phoneSearch}
        {actions}
      </div>
    ) : undefined
  ) : actions

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={title}
      description={description}
      initialFocus={isPhone && searchable && focusSearch ? searchRef : undefined}
      onCloseAutoFocus={onCloseAutoFocus}
      footer={footer}
    >
      {header}
      {/* sticky at the top of the rows, so it stays in view: iOS skips a
          covered field. The rows themselves are reached with ↑ / ↓ */}
      {isPhone && searchable && onNavigate ? (
        <input aria-hidden inputMode="none" className={`${NAV_STOP} sticky top-0 -mb-px block`} onFocus={() => onNavigate(-1)} />
      ) : null}
      <Command ref={commandRef} shouldFilter={false} label={searchLabel}>
        {!isPhone && searchable ? (
          <CommandInput autoFocus={focusSearch} aria-label={searchLabel} placeholder={searchPlaceholder ?? searchLabel} value={query} onValueChange={changeQuery} />
        ) : null}
        <CommandList
          label={listLabel ?? title}
          className={cn(isPhone ? 'max-h-none' : 'max-h-72', !isPhone && searchable && 'mt-2', listClassName)}
        >
          {children(query)}
        </CommandList>
      </Command>
      {after?.(query)}
    </ResponsiveDialog>
  )
}
