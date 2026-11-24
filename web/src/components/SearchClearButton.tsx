import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

// Clears a search field in one tap and hands focus back to it, so a new query
// can be typed straight away. Rendered only while the field has text.
export function SearchClearButton({ onClear, className }: { onClear: () => void; className?: string }) {
  const { t } = useTranslation()
  return (
    <button
      type="button"
      aria-label={t('common.list.clear_search')}
      title={t('common.list.clear_search')}
      // a pointer shortcut only: kept out of the Tab order, which runs from the
      // search to the rows (cmdk also claims Enter around it)
      tabIndex={-1}
      // keeps the field focused (and a phone keyboard up) through the tap
      onMouseDown={(e) => e.preventDefault()}
      onClick={(e) => {
        onClear()
        let box = e.currentTarget.parentElement
        while (box && !box.querySelector('input')) box = box.parentElement
        box?.querySelector('input')?.focus()
      }}
      className={cn(
        'grid size-8 shrink-0 cursor-pointer place-items-center rounded-md text-muted-foreground outline-none hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50',
        className,
      )}
    >
      <X className="size-4" />
    </button>
  )
}

