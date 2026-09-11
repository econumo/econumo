import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getProviderList } from '@/api/oauth'
import type { ProviderDto } from '@/api/dto/oauth'
import { logout } from '@/api/user'
import { Button } from '@/components/ui/button'
import { resetAnalyticsIdentity } from '@/lib/analytics'
import { METRICS, trackEvent } from '@/lib/metrics'
import { isNativeApp } from '@/lib/platform'
import { clearPersistedQueryCache } from '@/lib/queryPersist'
import { hasToken, removeToken } from '@/lib/storage'
import { RouterPage } from '@/app/router-pages'
import { providerDisplayName } from './oauthQueries'

export function LogoutPage() {
  const { t } = useTranslation()
  const [notice, setNotice] = useState<string | null>(null)

  useEffect(() => {
    const run = async () => {
      let logoutUrl = ''
      let provider = ''
      if (hasToken()) {
        try {
          const res = await logout()
          logoutUrl = res.logoutUrl
          provider = res.provider
        } catch {
          // best effort; the token is purged regardless
        }
        // the tail flush rides the visibilitychange beacon across the redirect
        trackEvent(METRICS.USER_LOGOUT)
        // must run after the logout event above, so it still carries the outgoing identity
        resetAnalyticsIdentity()
      }
      removeToken()
      clearPersistedQueryCache()
      // The IdP redirect is web-only: an IdP logout page in the app's browser
      // sheet leaves the user with no clean way back.
      if (logoutUrl && !isNativeApp()) {
        window.location.assign(logoutUrl)
        return
      }
      if (provider) {
        // The token is already gone at this point, so this is a public, unauthenticated request.
        let providers: ProviderDto[] | undefined
        try {
          providers = await getProviderList()
        } catch {
          // best effort; providerDisplayName falls back to the catalogue name below
        }
        setNotice(t('auth.oauth.logout_notice', { provider: providerDisplayName(provider, providers, t) }))
        return
      }
      window.location.assign(RouterPage.LOGIN)
    }
    void run()
  }, [t])

  if (!notice) {
    return null
  }
  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-4 px-6 text-center">
      <p className="max-w-md text-sm text-muted-foreground">{notice}</p>
      <Button type="button" className="h-11" onClick={() => window.location.assign(RouterPage.LOGIN)}>
        {t('common.button.ok.label')}
      </Button>
    </div>
  )
}
