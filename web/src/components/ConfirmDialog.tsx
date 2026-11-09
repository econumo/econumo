import { Button } from '@/components/ui/button'
import { useOnline } from '@/hooks/useOnline'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'

interface ConfirmDialogProps {
  open: boolean
  onClose: () => void
  onConfirm: () => void
  title?: string
  question: string
  confirmLabel: string
  cancelLabel: string
  destructive?: boolean
  /** The confirmed action works without a connection (a queued write, signing out). */
  offlineOk?: boolean
  /** The answer the dialog gives is not ready yet. */
  confirmDisabled?: boolean
}

export function ConfirmDialog({ open, onClose, onConfirm, title, question, confirmLabel, cancelLabel, destructive = false, offlineOk = false, confirmDisabled = false }: ConfirmDialogProps) {
  const online = useOnline()
  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={title ?? question} description={title ? question : undefined}>
      <div className={dialogActionsClass}>
        <Button type="button" variant="secondary" onClick={onClose}>
          {cancelLabel}
        </Button>
        <Button type="button" variant={destructive ? 'destructive' : 'default'} disabled={confirmDisabled || (!online && !offlineOk)} onClick={onConfirm}>
          {confirmLabel}
        </Button>
      </div>
    </ResponsiveDialog>
  )
}
