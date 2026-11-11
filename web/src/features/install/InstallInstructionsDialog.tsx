import { useTranslation } from 'react-i18next'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'
import { InstallSteps } from './InstallSteps'
import { useInstallStepsContent } from './useInstallStepsContent'
import type { InstructionMode } from './installPrompt'

export function InstallInstructionsDialog({ mode, open, onClose }: { mode: InstructionMode; open: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  const { intro } = useInstallStepsContent(mode)
  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose()
      }}
      title={t('install.dialog.title')}
      description={intro}
      footer={
        <Button type="button" className="h-11 w-full" onClick={onClose}>
          {t('install.dialog.close')}
        </Button>
      }
    >
      <InstallSteps mode={mode} />
    </ResponsiveDialog>
  )
}
