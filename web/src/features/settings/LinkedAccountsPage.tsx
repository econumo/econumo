import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { InfoBox } from '@/components/InfoBox'
import { RouterPage } from '@/app/router-pages'
import type { OAuthProviderId } from '@/api/dto/oauth'
import { useUserData } from '@/features/user/queries'
import { useProviders, useStartOAuth } from '@/features/auth/oauthQueries'
import { ProviderMark } from '@/features/auth/providerIcons'
import { METRICS, trackEvent } from '@/lib/metrics'
import { SettingsShell } from './SettingsShell'
import { useIdentities, useUnlinkIdentity } from './security'
import { parseUtcDateTime } from './securityFormat'

export function LinkedAccountsPage() {
  const { t, i18n } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()
  const providers = useProviders()
  const identities = useIdentities()
  const user = useUserData()
  const start = useStartOAuth()
  const unlink = useUnlinkIdentity()
  const [confirm, setConfirm] = useState<OAuthProviderId | null>(null)
  const toasted = useRef(false)
  const queryClient = useQueryClient()
  // Held in state because the parameter is cleared from the URL immediately —
  // a reload must not resurrect the message.
  const [linkError, setLinkError] = useState('')

  const linked = searchParams.get('linked')
  const oauthError = searchParams.get('oauthError')
  useEffect(() => {
    if (!linked || toasted.current) {
      return
    }
    toasted.current = true
    const name = providers.data?.find((p) => p.id === linked)?.name ?? t(`auth.oauth.provider_name.${linked}`)
    toast.success(t('user.page.settings.profile.linked_accounts.linked_toast', { provider: name }))
    trackEvent(METRICS.IDENTITY_LINKED, { provider: linked })
    // The link happened on the backend while the browser was away, so the
    // cached identity list is stale on return.
    void queryClient.invalidateQueries({ queryKey: ['oauth', 'identities'] })
    setSearchParams({}, { replace: true })
  }, [linked, providers.data, queryClient, setSearchParams, t])

  useEffect(() => {
    if (!oauthError) {
      return
    }
    setLinkError(oauthError)
    setSearchParams({}, { replace: true })
  }, [oauthError, setSearchParams])

  const hasPassword = user.data?.hasPassword ?? true
  const lastIdentityLocked = !hasPassword && (identities.data?.length ?? 0) <= 1
  const linkedIds = new Set(identities.data?.map((i) => i.provider))
  const unlinked = (providers.data ?? []).filter((p) => !linkedIds.has(p.id))

  return (
    <SettingsShell
      title={t('user.page.settings.profile.linked_accounts.header')}
      backTo={RouterPage.SETTINGS_PROFILE}
      crumbs={[
        { label: t('settings.page.header_desktop'), to: RouterPage.SETTINGS },
        { label: t('user.page.settings.profile.menu_item'), to: RouterPage.SETTINGS_PROFILE },
      ]}
    >
      <InfoBox>{t('user.page.settings.profile.linked_accounts.description')}</InfoBox>
      {linkError ? (
        <Alert variant="destructive" className="mb-2">
          <AlertDescription>
            {t(`auth.oauth.errors.${linkError}`, { defaultValue: t('auth.oauth.errors.provider_error') })}
          </AlertDescription>
        </Alert>
      ) : null}
      {identities.data?.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('user.page.settings.profile.linked_accounts.empty')}</p>
      ) : null}
      <ul className="flex flex-col gap-2">
        {identities.data?.map((i) => (
          <li key={i.provider} className="flex items-center gap-3 rounded-md bg-econumo-card px-3 py-2.5">
            <ProviderMark id={i.provider} />
            <div className="min-w-0 flex-1">
              <div className="truncate text-sm">{i.email}</div>
              <div className="text-xs text-muted-foreground">
                {t('user.page.settings.profile.linked_accounts.linked_on')} {parseUtcDateTime(i.createdAt).toLocaleDateString(i18n.language)}
              </div>
            </div>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              disabled={lastIdentityLocked}
              aria-describedby={lastIdentityLocked ? 'linked-accounts-last-identity-hint' : undefined}
              onClick={() => setConfirm(i.provider)}
            >
              {t('user.page.settings.profile.linked_accounts.unlink')}
            </Button>
          </li>
        ))}
      </ul>
      {lastIdentityLocked && (identities.data?.length ?? 0) > 0 ? (
        <p id="linked-accounts-last-identity-hint" className="mt-2 text-xs text-muted-foreground">
          {t('user.page.settings.profile.linked_accounts.last_identity_hint')}
        </p>
      ) : null}
      {unlinked.length > 0 ? (
        <ul className="mt-4 flex flex-col gap-2">
          {unlinked.map((p) => (
            <li key={p.id} className="flex items-center gap-3 rounded-md px-3 py-2.5">
              <ProviderMark id={p.id} />
              <div className="flex-1 text-sm">{p.name}</div>
              <Button type="button" size="sm" disabled={start.isPending} onClick={() => start.mutate({ provider: p.id, intent: 'link' })}>
                {t('user.page.settings.profile.linked_accounts.link')}
              </Button>
            </li>
          ))}
        </ul>
      ) : null}
      <ConfirmDialog
        open={confirm !== null}
        onClose={() => setConfirm(null)}
        onConfirm={() => {
          if (confirm) {
            unlink.mutate(confirm)
          }
          setConfirm(null)
        }}
        question={t('user.page.settings.profile.linked_accounts.confirm_unlink')}
        confirmLabel={t('user.page.settings.profile.linked_accounts.unlink')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </SettingsShell>
  )
}
