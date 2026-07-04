import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'

interface FailDialogProps {
  open: boolean
  onClose: () => void
  title: string
  description: string
}

export function FailDialog({ open, onClose, title, description }: FailDialogProps) {
  const { t } = useTranslation()
  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={title} description={description}>
      <Button className="w-full" onClick={onClose}>
        {t('elements.button.ok.label')}
      </Button>
    </ResponsiveDialog>
  )
}
