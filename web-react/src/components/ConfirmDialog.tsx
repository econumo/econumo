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
  destructive?: boolean
}

export function ConfirmDialog({ open, onClose, onConfirm, title, question, confirmLabel, cancelLabel, destructive = false }: ConfirmDialogProps) {
  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={title ?? question} description={title ? question : undefined}>
      <div className="grid grid-cols-2 gap-3">
        <Button type="button" variant="secondary" onClick={onClose}>
          {cancelLabel}
        </Button>
        <Button type="button" variant={destructive ? 'destructive' : 'default'} onClick={onConfirm}>
          {confirmLabel}
        </Button>
      </div>
    </ResponsiveDialog>
  )
}
