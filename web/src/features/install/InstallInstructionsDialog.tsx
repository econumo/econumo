import { Share, SquarePlus, Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'

export function InstallInstructionsDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  const steps = [
    { icon: Share, text: t('install.dialog.step_share') },
    { icon: SquarePlus, text: t('install.dialog.step_add') },
    { icon: Check, text: t('install.dialog.step_confirm') },
  ]
  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose()
      }}
      title={t('install.dialog.title')}
      description={t('install.dialog.intro')}
      footer={
        <Button type="button" className="h-11 w-full" onClick={onClose}>
          {t('install.dialog.close')}
        </Button>
      }
    >
      <ol className="flex flex-col gap-3">
        {steps.map(({ icon: Icon, text }, i) => (
          <li key={i} className="flex items-center gap-3 text-sm">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-econumo-card">
              <Icon className="size-5 text-econumo-link" />
            </span>
            <span>{text}</span>
          </li>
        ))}
      </ol>
    </ResponsiveDialog>
  )
}
