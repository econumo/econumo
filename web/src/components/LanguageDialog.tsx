import { Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { applyLocale } from '@/components/LanguageSelector'
import { getLocaleOptions } from '@/lib/config'
import i18n from '@/app/i18n'

// Two languages, pick-one: a short list of buttons with a check on the
// active one, same idiom as SortDialog.
export function LanguageDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={t('settings.language.menu_item')}>
      <div className="flex flex-col gap-2 [&_button]:h-11">
        {getLocaleOptions().map((option) => (
          <Button
            key={option.value}
            type="button"
            variant={option.value === i18n.language ? 'secondary' : 'ghost'}
            className="justify-between"
            onClick={() => {
              applyLocale(option.value)
              onClose()
            }}
          >
            {option.label}
            {option.value === i18n.language ? <Check className="size-4" /> : null}
          </Button>
        ))}
      </div>
    </ResponsiveDialog>
  )
}
