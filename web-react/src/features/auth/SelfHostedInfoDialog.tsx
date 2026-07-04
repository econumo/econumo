import { useTranslation } from 'react-i18next'
import { FailDialog } from '@/components/FailDialog'

export function SelfHostedInfoDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  return (
    <FailDialog
      open={open}
      onClose={onClose}
      title={t('elements.econumo.label')}
      description={t('modules.app.modal.self_hosted.information')}
    />
  )
}
