import { useTranslation } from 'react-i18next'
import { Globe } from 'lucide-react'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { getLocaleOptions, locale } from '@/lib/config'
import { getToken } from '@/lib/storage'
import { updateLanguage } from '@/api/user'
import i18n from '@/app/i18n'

export function applyLocale(value: string): void {
  locale(value)
  void i18n.changeLanguage(value)
  document.documentElement.lang = value
  // Best-effort server-side persist for future background emails; login
  // capture self-corrects if this is offline/fails.
  if (getToken() !== null) {
    updateLanguage(value).catch(() => {})
  }
}

export function LanguageSelector() {
  const { t } = useTranslation()
  const current = getLocaleOptions().find((o) => o.value === i18n.language)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="ghost" size="sm" aria-label={t('settings.language.menu_item')}>
          <Globe className="size-4" />
          {current?.short ?? 'EN'}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {getLocaleOptions().map((o) => (
          <DropdownMenuItem key={o.value} onSelect={() => applyLocale(o.value)}>
            {o.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
