import { useTranslation } from 'react-i18next'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { Switch } from '@/components/ui/switch'

export function AnalyticsDialog({ open, onClose, enabled, onChange }: {
  open: boolean
  onClose: () => void
  enabled: boolean
  onChange: (enabled: boolean) => void
}) {
  const { t } = useTranslation()
  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={t('user.page.settings.profile.analytics.menu_item')}>
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-2 text-sm text-muted-foreground">
          <p>{t('user.page.settings.profile.analytics.description')}</p>
          <p>{t('user.page.settings.profile.analytics.promise')}</p>
        </div>
        <div className="flex items-center justify-between gap-4 rounded-lg bg-econumo-card px-4 py-3.5">
          <label htmlFor="profile-analytics" className="text-sm">
            {t('user.page.settings.profile.analytics.label')}
          </label>
          <Switch id="profile-analytics" checked={enabled} onCheckedChange={onChange} />
        </div>
      </div>
    </ResponsiveDialog>
  )
}
