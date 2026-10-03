import { useEffect, useMemo, type ReactNode } from 'react'
import { isAxiosError } from 'axios'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useSearchParams } from 'react-router'
import { authorizationRequestFromSearch } from '@/api/authserver'
import type { AuthorizationRequestResultDto } from '@/api/dto/authserver'
import { CoinLoader } from '@/components/CoinLoader'
import { LanguageBadge } from '@/components/LanguageBadge'
import { Button } from '@/components/ui/button'
import { apiErrorMessage } from '@/lib/apiError'
import { RouterPage } from '@/app/router-pages'
import { useUserData } from '@/features/user/queries'
import logo from '@/assets/econumo.svg'
import { rememberPostLoginRedirect } from './postLoginRedirect'
import { useApproveAuthorization, useAuthorizationRequest, useDeclineAuthorization } from './queries'

function Shell({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-dvh flex-col items-center justify-center gap-6 bg-econumo-card p-4 pt-[max(env(safe-area-inset-top),1rem)] pb-[max(env(safe-area-inset-bottom),1rem)]">
      <div className="relative">
        <img src={logo} width={194} height={20} alt={t('common.econumo.label')} />
        <div className="absolute top-1/2 left-full ml-2 -translate-y-1/2">
          <LanguageBadge />
        </div>
      </div>
      <div className="flex w-full max-w-md flex-col gap-5 rounded-xl border bg-background p-6 shadow-sm">{children}</div>
    </div>
  )
}

function returnLabel(t: (key: string, opts?: Record<string, string>) => string, r: AuthorizationRequestResultDto) {
  return r.isLoopback ? t('authserver.consent.returnToApp') : t('authserver.consent.returnToHost', { host: r.redirectHost })
}

export function ConsentPage() {
  const { t } = useTranslation()
  const location = useLocation()
  const [search] = useSearchParams()
  const req = useMemo(() => authorizationRequestFromSearch(search), [search])
  const request = useAuthorizationRequest(req)
  const approve = useApproveAuthorization()
  const decline = useDeclineAuthorization()
  const { data: user } = useUserData()

  const consentable = !!request.data && !request.data.errorRedirectUrl

  // Survives "Switch account" (logout, sign-in elsewhere) so the user lands back here.
  useEffect(() => {
    if (consentable) {
      rememberPostLoginRedirect(location.pathname + location.search)
    }
  }, [consentable, location.pathname, location.search])

  if (request.isPending) {
    return (
      <Shell>
        <div className="flex justify-center py-6">
          <CoinLoader />
        </div>
      </Shell>
    )
  }

  if (request.isError) {
    return (
      <Shell>
        <h1 className="text-lg font-semibold">{t('authserver.consent.invalidTitle')}</h1>
        <p className="text-sm text-muted-foreground">{apiErrorMessage(request.error)}</p>
      </Shell>
    )
  }

  const result = request.data

  // The server's error redirect is attacker-controlled (any client may register any https
  // redirect), so it is only ever followed on an explicit click, never on render.
  if (result.errorRedirectUrl) {
    return (
      <Shell>
        <h1 className="text-lg font-semibold">{t('authserver.consent.invalidTitle')}</h1>
        <p className="text-sm text-muted-foreground">{t('authserver.consent.invalidBody')}</p>
        <Button onClick={() => window.location.assign(result.errorRedirectUrl)}>{returnLabel(t, result)}</Button>
      </Shell>
    )
  }

  const readonly = isAxiosError(approve.error) && approve.error.response?.status === 402
  if (readonly) {
    return (
      <Shell>
        <h1 className="text-lg font-semibold">{result.clientName}</h1>
        <p className="text-sm text-muted-foreground">{t('authserver.consent.readonly')}</p>
        {decline.error && <p className="text-sm text-destructive">{apiErrorMessage(decline.error)}</p>}
        <Button disabled={decline.isPending} onClick={() => decline.mutate(req)}>
          {t('authserver.consent.returnToApp')}
        </Button>
      </Shell>
    )
  }

  const busy = approve.isPending || approve.isSuccess || decline.isPending || decline.isSuccess
  const failure = approve.error ?? decline.error

  return (
    <Shell>
      <div className="flex flex-col gap-1">
        <h1 className="text-sm text-muted-foreground">{t('authserver.consent.title')}</h1>
        <p className="text-xl font-bold break-words">{result.clientName}</p>
        <p>{t('authserver.consent.wantsAccess')}</p>
      </div>
      <div className="flex flex-col gap-2 text-sm text-muted-foreground">
        <p>{t('authserver.consent.fullAccess')}</p>
        <p>
          {result.isLoopback
            ? t('authserver.consent.loopback')
            : t('authserver.consent.redirect', { host: result.redirectHost })}
        </p>
      </div>
      {failure && <p className="text-sm text-destructive">{apiErrorMessage(failure)}</p>}
      <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
        <Button variant="outline" disabled={busy} onClick={() => decline.mutate(req)}>
          {t('authserver.consent.deny')}
        </Button>
        <Button disabled={busy} onClick={() => approve.mutate(req)}>
          {t('authserver.consent.allow')}
        </Button>
      </div>
      {user && (
        <p className="text-xs text-muted-foreground">
          {t('authserver.consent.signedInAs', { email: user.email })}{' '}
          <Link to={RouterPage.LOGOUT} className="underline">
            {t('authserver.consent.switchAccount')}
          </Link>
        </p>
      )}
    </Shell>
  )
}
