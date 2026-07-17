import { useState } from 'react'
import { MonitorSmartphone } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { InfoBox } from '@/components/InfoBox'
import { RouterPage } from '@/app/router-pages'
import type { SessionDto } from '@/api/dto/user'
import { useRevokeOtherSessions, useRevokeSession, useSessions } from './security'
import { describeUserAgent, relativeTime } from './securityFormat'
import { SettingsShell } from './SettingsShell'

export function SessionsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data: sessions } = useSessions()
  const revokeSession = useRevokeSession()
  const revokeOthers = useRevokeOtherSessions()

  const [confirmTarget, setConfirmTarget] = useState<SessionDto | null>(null)
  const [confirmOthers, setConfirmOthers] = useState(false)

  const revoke = (session: SessionDto) => {
    if (session.isCurrent) {
      // Signing out the presenting session IS a logout: the logout flow revokes
      // it server-side and clears local state. Revoking it here first would make
      // the logout call 401 and surface the "session expired" banner instead.
      void navigate(RouterPage.LOGOUT)
    } else {
      revokeSession.mutate(session.id)
    }
    setConfirmTarget(null)
  }

  const others = (sessions ?? []).filter((s) => !s.isCurrent)

  return (
    <SettingsShell
      title={t('user.page.settings.profile.sessions.header')}
      backTo={RouterPage.SETTINGS_PROFILE}
      crumbs={[
        { label: t('settings.page.header_desktop'), to: RouterPage.SETTINGS },
        { label: t('user.page.settings.profile.menu_item'), to: RouterPage.SETTINGS_PROFILE },
      ]}
    >
      <InfoBox>{t('user.page.settings.profile.sessions.description')}</InfoBox>

      <div className="flex max-w-md flex-col gap-2 py-2">
        {(sessions ?? []).map((session) => (
          <div key={session.id} className="flex items-center gap-3 rounded-lg bg-econumo-card px-4 py-3">
            <MonitorSmartphone className="size-5 shrink-0 text-muted-foreground" aria-hidden="true" />
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className="truncate text-sm">
                {describeUserAgent(session.userAgent) || t('user.page.settings.profile.sessions.unknown_device')}
                {session.isCurrent ? (
                  <span className="ml-2 rounded bg-econumo-magenta/10 px-1.5 py-0.5 text-[11px] font-medium text-econumo-magenta">
                    {t('user.page.settings.profile.sessions.current')}
                  </span>
                ) : null}
              </span>
              <span className="text-xs text-muted-foreground">
                {t('user.page.settings.profile.sessions.last_active')} {relativeTime(session.lastUsedAt)}
              </span>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="shrink-0 text-econumo-magenta"
              onClick={() => setConfirmTarget(session)}
            >
              {session.isCurrent
                ? t('user.page.settings.profile.sessions.sign_out')
                : t('user.page.settings.profile.sessions.revoke')}
            </Button>
          </div>
        ))}
      </div>

      {others.length > 0 ? (
        <div className="max-w-md py-2">
          <Button type="button" onClick={() => setConfirmOthers(true)}>
            {t('user.page.settings.profile.sessions.revoke_others')}
          </Button>
        </div>
      ) : null}

      <ConfirmDialog
        open={confirmTarget !== null}
        onClose={() => setConfirmTarget(null)}
        onConfirm={() => confirmTarget && revoke(confirmTarget)}
        question={
          confirmTarget?.isCurrent
            ? t('user.page.settings.profile.sessions.confirm_sign_out')
            : t('user.page.settings.profile.sessions.confirm_revoke')
        }
        confirmLabel={
          confirmTarget?.isCurrent
            ? t('user.page.settings.profile.sessions.sign_out')
            : t('user.page.settings.profile.sessions.revoke')
        }
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
      <ConfirmDialog
        open={confirmOthers}
        onClose={() => setConfirmOthers(false)}
        onConfirm={() => {
          revokeOthers.mutate()
          setConfirmOthers(false)
        }}
        question={t('user.page.settings.profile.sessions.confirm_revoke_others')}
        confirmLabel={t('user.page.settings.profile.sessions.sign_out')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </SettingsShell>
  )
}
