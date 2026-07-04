import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'

interface AcceptInviteDialogProps {
  open: boolean
  pending: boolean
  /** server-side rejection message; keeps the dialog open (divergence from Vue, which closes) */
  error: string | null
  onSubmit: (code: string) => void
  onClose: () => void
}

export function AcceptInviteDialog({ open, pending, error, onSubmit, onClose }: AcceptInviteDialogProps) {
  const { t } = useTranslation()
  const [code, setCode] = useState('')
  const [touched, setTouched] = useState(false)

  useEffect(() => {
    if (open) {
      setCode('')
      setTouched(false)
    }
  }, [open])

  const empty = code.trim() === ''

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={t('modules.connections.modals.accept_invite.label')}
    >
      <form
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          setTouched(true)
          if (!empty) {
            onSubmit(code.trim())
          }
        }}
      >
        <p className="text-sm text-muted-foreground">{t('modules.connections.modals.accept_invite.instruction')}</p>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="invite-code">{t('modules.connections.modals.accept_invite.code.label')}</Label>
          <Input id="invite-code" value={code} onChange={(e) => setCode(e.target.value)} autoFocus />
          {touched && empty ? (
            <p className="text-sm text-destructive">{t('modules.connections.forms.invitation_code.validation.required_field')}</p>
          ) : null}
          {error ? (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          ) : null}
        </div>
        <div className="grid grid-cols-2 gap-3">
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('elements.button.cancel.label')}
          </Button>
          <Button type="submit" disabled={pending}>
            {t('elements.button.accept.label')}
          </Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
