import { useMemo, useRef, useState } from 'react'
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
  if (!open) {
    return null
  }
  if (anchor && !isPhone) {
    return <AnchoredThread anchor={anchor} title={title} onClose={onClose} threadProps={threadProps} />
  }
  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={title}>
      <div data-testid="comments-sheet">
        <CommentThread {...threadProps} layout="sheet" />
      </div>
    </ResponsiveDialog>
  )
}

function AnchoredThread({
  anchor,
  title,
  onClose,
  threadProps,
}: {
  anchor: HTMLElement
  title: string
  onClose: () => void
  threadProps: CommentThreadProps
}) {
  const virtualRef = useMemo(() => ({ current: anchor }), [anchor])
  // the popover's only anchor is virtual, so Radix has no trigger to hand focus
  // back to: Esc would drop it on <body>. Remember the opener instead.
  const [openedAt] = useState(() => ({ anchor, opener: document.activeElement }))
  const opener = openedAt.anchor === anchor ? openedAt.opener : null
  const escaped = useRef(false)
  return (
    <Popover open onOpenChange={(o) => !o && onClose()}>
      <PopoverAnchor virtualRef={virtualRef} />
      <PopoverContent
        className="w-80 p-3"
        align="end"
        aria-label={title}
        data-testid="comments-popover"
        onEscapeKeyDown={() => {
          escaped.current = true
        }}
        // only Esc returns focus: an outside click already put it where the user clicked
        onCloseAutoFocus={(e) => {
          if (!escaped.current) {
            return
          }
          e.preventDefault()
          const target =
            opener instanceof HTMLElement && opener !== document.body && opener.isConnected
              ? opener
              : anchor.querySelector<HTMLElement>('button')
          target?.focus()
        }}
      >
        <CommentThread {...threadProps} />
      </PopoverContent>
    </Popover>
  )
}
