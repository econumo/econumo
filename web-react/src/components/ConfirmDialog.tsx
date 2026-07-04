import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'

interface ConfirmDialogProps {
  open: boolean
  onClose: () => void
  onConfirm: () => void
  title?: string
  question: string
  confirmLabel: string
  cancelLabel: string
}

export function ConfirmDialog({ open, onClose, onConfirm, title, question, confirmLabel, cancelLabel }: ConfirmDialogProps) {
  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={title ?? question} description={title ? question : undefined}>
      {/* Vue pairs full-width grey + magenta actions, even for deletions */}
      <div className="grid grid-cols-2 gap-3">
        <Button type="button" variant="secondary" onClick={onClose}>
          {cancelLabel}
        </Button>
        <Button type="button" onClick={onConfirm}>
          {confirmLabel}
        </Button>
      </div>
    </ResponsiveDialog>
  )
}
