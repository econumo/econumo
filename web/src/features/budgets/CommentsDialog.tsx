import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { CommentThread } from './CommentThread'
import type { CommentThreadProps } from './CommentThread'

interface CommentsDialogProps extends CommentThreadProps {
  open: boolean
  onClose: () => void
  title: string
}

// Compact-viewport and non-editable-cell entry point (a guest's view, or any cell
// with no LimitEditor popover to hang the thread's disclosure off of).
export function CommentsDialog({ open, onClose, title, ...threadProps }: CommentsDialogProps) {
  if (!open) {
    return null
  }
  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={title}>
      <CommentThread {...threadProps} />
    </ResponsiveDialog>
  )
}
