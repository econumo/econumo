import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router'
import { CoinLoader } from '@/components/CoinLoader'
import { RouterPage } from '@/app/router-pages'
import { useExchangeHandoff } from './oauthQueries'

// The handoff rides in the fragment so it never reaches server logs; it is
// consumed exactly once and the fragment is dropped from history on arrival.
export function OAuthCallbackPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { hash } = useLocation()
  const exchange = useExchangeHandoff()
  const started = useRef(false)

  useEffect(() => {
    if (started.current) {
      return
    }
    started.current = true
    const code = new URLSearchParams(hash.replace(/^#/, '')).get('handoff')
    if (window.location.hash) {
      window.history.replaceState(null, '', window.location.pathname)
    }
    if (!code) {
      void navigate(`${RouterPage.LOGIN}?oauthError=invalid_state`, { replace: true })
      return
    }
    exchange
      .mutateAsync(code)
      .then(() => navigate(RouterPage.HOME, { replace: true }))
      .catch(() => navigate(`${RouterPage.LOGIN}?oauthError=invalid_state`, { replace: true }))
  }, [exchange, hash, navigate])

  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-4">
      <CoinLoader label={t('auth.oauth.callback.signing_in')} />
      <p className="text-sm text-muted-foreground">{t('auth.oauth.callback.signing_in')}</p>
    </div>
  )
}
