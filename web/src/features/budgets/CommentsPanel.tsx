import { useMemo } from 'react'
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useIsPhone } from '@/hooks/useIsPhone'
import { CommentThread } from './CommentThread'
import type { CommentThreadProps } from './CommentThread'

interface CommentsPanelProps extends CommentThreadProps {
  open: boolean
  onClose: () => void
  title: string
  /** the cell the thread belongs to; null (or a phone) presents a bottom sheet instead */
  anchor: HTMLElement | null
}

export function CommentsPanel({ open, onClose, title, anchor, ...threadProps }: CommentsPanelProps) {
  const isPhone = useIsPhone()
  const virtualRef = useMemo(() => ({ current: anchor }), [anchor])
  if (!open) {
    return null
  }
  if (anchor && !isPhone) {
    return (
      <Popover open onOpenChange={(o) => !o && onClose()}>
        <PopoverAnchor virtualRef={virtualRef as { current: HTMLElement }} />
        <PopoverContent className="w-80 p-3" align="end" aria-label={title} data-testid="comments-popover">
          <CommentThread {...threadProps} />
        </PopoverContent>
      </Popover>
    )
  }
  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={title}>
      <div data-testid="comments-sheet">
        <CommentThread {...threadProps} layout="sheet" />
      </div>
    </ResponsiveDialog>
  )
}
