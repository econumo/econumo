import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getProviderList } from '@/api/oauth'
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
  // Latches the effect body to a single run: deps are [] (a language switch
  // must not re-run the flow and slip in an extra redirect while the notice
  // is on screen), so `t` is read through a ref instead of a dependency.
  const started = useRef(false)
  const tRef = useRef(t)
  tRef.current = t

  useEffect(() => {
    if (started.current) {
      return
    }
    started.current = true
    const run = async () => {
      let logoutUrl = ''
      let provider = ''
      if (hasToken()) {
        // Started alongside logout(), not awaited after it and after the token
        // is purged: a slow/hung get-provider-list must not strand the caller
        // on a blank page with no token to retry with.
        const [res, providers] = await Promise.all([
          logout().catch(() => undefined),
          getProviderList().catch(() => undefined),
        ])
        if (res) {
          logoutUrl = res.logoutUrl
          provider = res.provider
        }
        // the tail flush rides the visibilitychange beacon across the redirect
        trackEvent(METRICS.USER_LOGOUT)
        // must run after the logout event above, so it still carries the outgoing identity
        resetAnalyticsIdentity()
        removeToken()
        clearPersistedQueryCache()
        // The IdP redirect is web-only: an IdP logout page in the app's browser
        // sheet leaves the user with no clean way back.
        if (logoutUrl && !isNativeApp()) {
          window.location.assign(logoutUrl)
          return
        }
        if (provider) {
          setNotice(tRef.current('auth.oauth.logout_notice', { provider: providerDisplayName(provider, providers, tRef.current) }))
          return
        }
      } else {
        removeToken()
        clearPersistedQueryCache()
      }
      window.location.assign(RouterPage.LOGIN)
    }
    void run()
  }, [])

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
