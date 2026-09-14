import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router'
import { CoinLoader } from '@/components/CoinLoader'
import { RouterPage } from '@/app/router-pages'
import { takeOAuthFlow, useExchangeHandoff } from './oauthQueries'

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
    const flow = takeOAuthFlow()
    if (window.location.hash) {
      window.history.replaceState(null, '', window.location.pathname)
    }
    // A missing flow secret means this browser did not start the sign-in — the
    // same dead end as a missing handoff.
    if (!code || !flow) {
      void navigate(`${RouterPage.LOGIN}?oauthError=invalid_state`, { replace: true })
      return
    }
    exchange
      .mutateAsync({ code, flow })
      .then(() => navigate(RouterPage.HOME, { replace: true }))
      .catch((err: unknown) => {
        // 401 is the handoff itself (expired, replayed, or a foreign flow);
        // anything else went wrong on the way, which reads differently.
        const status = (err as { response?: { status?: number } })?.response?.status
        const code = status === 401 ? 'invalid_state' : 'provider_error'
        void navigate(`${RouterPage.LOGIN}?oauthError=${code}`, { replace: true })
      })
  }, [exchange, hash, navigate])

  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-4">
      <CoinLoader label={t('auth.oauth.callback.signing_in')} />
      <p className="text-sm text-muted-foreground">{t('auth.oauth.callback.signing_in')}</p>
    </div>
  )
}
