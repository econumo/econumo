import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'

export function GenerateInviteDialog({ open, code, onClose }: { open: boolean; code: string; onClose: () => void }) {
  const { t } = useTranslation()
  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={t('connections.modals.generate_invite.code.label')}
    >
      <div className="flex flex-col gap-2">
        <p className="text-sm text-muted-foreground">{t('connections.modals.generate_invite.instruction')}</p>
        <p className="py-3 text-center font-mono text-3xl tracking-widest" data-testid="invite-code">
          {code}
        </p>
        <Button type="button" className="w-full h-11" onClick={onClose}>
          {t('common.button.ok.label')}
        </Button>
      </div>
    </ResponsiveDialog>
  )
}
