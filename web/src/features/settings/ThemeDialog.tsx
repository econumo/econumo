import { Check, Monitor, Moon, Sun } from 'lucide-react'
import { RadioGroup } from 'radix-ui'
import { useTranslation } from 'react-i18next'
import { UserOptions } from '@/api/dto/user'
import { useUpdateTheme, useUserData, userOption } from '@/features/user/queries'
import { themePreference, themePreferences } from '@/lib/theme'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'

const icons = { system: Monitor, light: Sun, dark: Moon }

export function ThemeDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const updateTheme = useUpdateTheme()
  const selected = themePreference(userOption(user, UserOptions.THEME))

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(value) => !value && onClose()}
      title={t('user.page.settings.profile.theme.label')}
      footer={<Button type="button" variant="secondary" className="h-11 w-full" onClick={onClose}>{t('common.button.close.label')}</Button>}
    >
      <div>
        <RadioGroup.Root
          aria-label={t('user.page.settings.profile.theme.label')}
          aria-describedby="profile-theme-description"
          value={selected}
          disabled={!user || updateTheme.isPending}
          onValueChange={(value) => updateTheme.mutate(themePreference(value))}
          className="grid grid-cols-3 gap-2 p-1 sm:gap-3"
        >
          {themePreferences.map((value) => {
            const Icon = icons[value]
            return (
              <RadioGroup.Item
                key={value}
                value={value}
                className="group min-w-0 rounded-lg border border-border bg-background p-1.5 text-sm outline-none transition-shadow hover:border-muted-foreground/60 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-wait data-[state=checked]:border-econumo-magenta data-[state=checked]:ring-1 data-[state=checked]:ring-econumo-magenta"
              >
                <span aria-hidden="true" className={`theme-preview theme-preview-${value}`}>
                  <span className="theme-preview-sidebar"><span /><span /><span /></span>
                  <span className="theme-preview-content"><span /><span /><span /></span>
                </span>
                <span className="flex items-center justify-center gap-1.5 py-2 text-xs font-medium sm:text-sm">
                  <Icon className="size-3.5 shrink-0" aria-hidden="true" />
                  {t(`user.page.settings.profile.theme.${value}`)}
                </span>
                <span aria-hidden="true" className="mx-auto mb-1 flex size-4 items-center justify-center rounded-full border border-border group-data-[state=checked]:border-econumo-magenta group-data-[state=checked]:bg-econumo-magenta group-data-[state=checked]:text-white">
                  {selected === value ? <Check className="size-3" strokeWidth={3} /> : null}
                </span>
              </RadioGroup.Item>
            )
          })}
        </RadioGroup.Root>
        <p id="profile-theme-description" className="mt-3 text-xs leading-relaxed text-muted-foreground">
          {t('user.page.settings.profile.theme.description')}
        </p>
        {updateTheme.isError ? <p role="alert" className="mt-2 text-xs text-destructive">{t('user.page.settings.profile.theme.save_error')}</p> : null}
      </div>
    </ResponsiveDialog>
  )
}
