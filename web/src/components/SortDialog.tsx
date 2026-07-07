import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'

interface SortDialogProps {
  open: boolean
  onClose: () => void
  onPick: (direction: 'asc' | 'desc') => void
}

// Port of SortDialogModal.vue: only the alphabetical options are live
// (the usage-count buttons are commented out in the Vue template).
export function SortDialog({ open, onClose, onPick }: SortDialogProps) {
  const { t } = useTranslation()
  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={t('modals.sort.header')}>
      <div className="flex flex-col gap-2 max-md:[&_button]:h-11">
        <Button type="button" variant="secondary" onClick={() => onPick('asc')}>
          {t('modals.sort.mode.alphabet.asc')}
        </Button>
        <Button type="button" variant="secondary" onClick={() => onPick('desc')}>
          {t('modals.sort.mode.alphabet.desc')}
        </Button>
        <Button type="button" variant="ghost" onClick={onClose}>
          {t('elements.button.cancel.label')}
        </Button>
      </div>
    </ResponsiveDialog>
  )
}
