import { useState } from 'react'
import { Check, Copy, Plug } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { InfoBox } from '@/components/InfoBox'
import { RouterPage } from '@/app/router-pages'
import type { ConnectedAppDto } from '@/api/dto/authserver'
import { mcpUrl } from '@/lib/config'
import { useServerConfig } from '@/lib/appConfig'
import { copyText } from '@/lib/clipboard'
import { formatDate } from '@/lib/datetime'
import { parseUtcDateTime, relativeTime } from '@/features/settings/securityFormat'
import { SettingsShell } from '@/features/settings/SettingsShell'
import { useConnectedApps, useRevokeConnectedApp } from './queries'

export function ConnectedAppsPage() {
  const { t, i18n } = useTranslation()
  const { data: apps } = useConnectedApps()
  const revoke = useRevokeConnectedApp()
  const [confirmRevoke, setConfirmRevoke] = useState<ConnectedAppDto | null>(null)
  const [copied, setCopied] = useState(false)

  useServerConfig((s) => s.revision)
  const address = mcpUrl()

  const copy = () => {
    void copyText(address).then(setCopied)
  }

  return (
    <SettingsShell
      title={t('authserver.apps.title')}
      backTo={RouterPage.SETTINGS_PROFILE}
      crumbs={[
        { label: t('settings.page.header_desktop'), to: RouterPage.SETTINGS },
        { label: t('user.page.settings.profile.menu_item'), to: RouterPage.SETTINGS_PROFILE },
      ]}
    >
      <InfoBox>{t('authserver.apps.description')}</InfoBox>

      <div className="flex max-w-md flex-col gap-2 py-2">
        {apps && apps.length === 0 ? (
          <div className="flex flex-col gap-3 px-1">
            <p className="text-sm text-muted-foreground">{t('authserver.apps.empty')}</p>
            {address ? (
              <>
                <div className="flex items-center gap-2 rounded-lg bg-econumo-card px-3 py-2">
                  <code className="min-w-0 flex-1 break-all text-xs">{address}</code>
                  <Button type="button" size="sm" variant="secondary" onClick={copy}>
                    {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
                    {copied ? t('authserver.apps.copied') : t('authserver.apps.copy')}
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">{t('authserver.apps.howTo')}</p>
              </>
            ) : (
              <p className="text-xs text-muted-foreground">{t('authserver.apps.unavailable')}</p>
            )}
          </div>
        ) : null}
        {(apps ?? []).map((app) => (
          <div key={app.id} className="flex items-center gap-3 rounded-lg bg-econumo-card px-4 py-3">
            <Plug className="size-5 shrink-0 text-muted-foreground" aria-hidden="true" />
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className="truncate text-sm">{app.clientName}</span>
              <span className="truncate text-xs text-muted-foreground">
                {app.isLoopback
                  ? t('authserver.apps.loopback')
                  : t('authserver.apps.returnsTo', { host: app.redirectHost })}
              </span>
              <span className="text-xs text-muted-foreground">
                {t('authserver.apps.connected', { date: formatDate(parseUtcDateTime(app.createdAt)) })}
                {' · '}
                {t('authserver.apps.lastUsed', {
                  when: relativeTime(app.lastUsedAt, { lang: i18n.language, justNow: t('common.date.just_now') }),
                })}
              </span>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="shrink-0 text-econumo-magenta"
              onClick={() => setConfirmRevoke(app)}
            >
              {t('authserver.apps.revoke')}
            </Button>
          </div>
        ))}
      </div>

      <ConfirmDialog
        open={confirmRevoke !== null}
        onClose={() => setConfirmRevoke(null)}
        onConfirm={() => {
          if (confirmRevoke) {
            revoke.mutate(confirmRevoke.id)
          }
          setConfirmRevoke(null)
        }}
        question={t('authserver.apps.revokeConfirm', { name: confirmRevoke?.clientName ?? '' })}
        confirmLabel={t('authserver.apps.revoke')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </SettingsShell>
  )
}
