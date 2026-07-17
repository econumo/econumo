import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { LanguageDialog } from '@/components/LanguageDialog'
import { getLocaleOptions } from '@/lib/config'
import i18n from '@/app/i18n'

export function LanguageBadge() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const current = getLocaleOptions().find((o) => o.value === i18n.language)
  return (
    <>
      <Badge asChild variant="outline" className="rounded-md bg-background px-1.5 text-muted-foreground transition-colors hover:text-foreground">
        <button
          type="button"
          aria-label={t('settings.language.menu_item')}
          title={t('settings.language.menu_item')}
          onClick={() => setOpen(true)}
        >
          {current?.short ?? 'EN'}
        </button>
      </Badge>
      {open ? <LanguageDialog open onClose={() => setOpen(false)} /> : null}
    </>
  )
}
