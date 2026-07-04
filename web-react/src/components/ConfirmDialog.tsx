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
      <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
        <Button type="button" variant="secondary" onClick={onClose}>
          {cancelLabel}
        </Button>
        <Button type="button" variant="destructive" onClick={onConfirm}>
          {confirmLabel}
        </Button>
      </div>
    </ResponsiveDialog>
  )
}
