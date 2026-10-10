import { useEffect, useMemo, useState, type ReactNode } from 'react'
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
import { clearPostLoginRedirect, rememberPostLoginRedirect } from './postLoginRedirect'
import {
  InvalidRedirectError,
  isSafeRedirectUrl,
  useApproveAuthorization,
  useAuthorizationRequest,
  useDeclineAuthorization,
} from './queries'

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

const ARM_DELAY_MS = 500

// Double-clickjacking guard: a page that opens this one under the user's
// cursor can make the second click of a double-click land on Allow. Allow is
// armed only while this page is visible AND focused, and only after the user
// demonstrably interacts with it (a pointer that actually moves, or a key) or
// after it has stayed visible and focused for ARM_DELAY_MS. The listeners stay
// installed for the component's lifetime: losing focus or visibility disarms
// the button again and cancels the pending timer, so returning to an
// already-open consent window restarts the wait instead of leaving Allow
// clickable.
function useInteractionArmed() {
  const [armed, setArmed] = useState(false)
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined
    const clearTimer = () => {
      if (timer !== undefined) {
        clearTimeout(timer)
        timer = undefined
      }
    }
    const ready = () => document.visibilityState === 'visible' && document.hasFocus()
    const arm = () => {
      clearTimer()
      setArmed(true)
    }
    const armIfReady = () => {
      if (ready()) arm()
    }
    const disarm = () => {
      clearTimer()
      setArmed(false)
    }
    const sync = () => {
      if (ready()) {
        timer ??= setTimeout(() => {
          timer = undefined
          armIfReady()
        }, ARM_DELAY_MS)
      } else {
        disarm()
      }
    }
    // Browsers dispatch motionless pointer events when content appears under a
    // resting cursor; only real movement counts.
    const onPointerMove = (e: PointerEvent) => {
      if (e.movementX !== 0 || e.movementY !== 0) armIfReady()
    }
    sync()
    document.addEventListener('visibilitychange', sync)
    window.addEventListener('focus', sync)
    window.addEventListener('blur', disarm)
    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('keydown', armIfReady)
    return () => {
      clearTimer()
      document.removeEventListener('visibilitychange', sync)
      window.removeEventListener('focus', sync)
      window.removeEventListener('blur', disarm)
      window.removeEventListener('pointermove', onPointerMove)
      window.removeEventListener('keydown', armIfReady)
    }
  }, [])
  return armed
}

function AllowButton({ disabled, onClick, children }: { disabled: boolean; onClick: () => void; children: ReactNode }) {
  const armed = useInteractionArmed()
  return (
    <Button disabled={disabled || !armed} onClick={onClick}>
      {children}
    </Button>
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

  const errorStatus = isAxiosError(request.error) ? request.error.response?.status : undefined
  // A 401 means the stored session is stale: the api client is already sending the user to
  // /login, and the remembered URL is what brings them back here afterwards.
  const unusable = !!request.data?.errorRedirectUrl || (request.isError && errorStatus !== 401)

  // Remembered on mount so even a request that dies on a stale session survives the sign-in;
  // dropped once the request is known to be dead, so a later sign-in does not land on it.
  useEffect(() => {
    if (unusable) {
      clearPostLoginRedirect()
    } else {
      rememberPostLoginRedirect(location.pathname + location.search)
    }
  }, [unusable, location.pathname, location.search])

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
  const badDecision = approve.error instanceof InvalidRedirectError || decline.error instanceof InvalidRedirectError
  if (result.errorRedirectUrl || badDecision) {
    const target = result.errorRedirectUrl
    return (
      <Shell>
        <h1 className="text-lg font-semibold">{t('authserver.consent.invalidTitle')}</h1>
        <p className="text-sm text-muted-foreground">{t('authserver.consent.invalidBody')}</p>
        {target && isSafeRedirectUrl(target) && (
          <Button onClick={() => window.location.assign(target)}>{returnLabel(t, result)}</Button>
        )}
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
        <AllowButton disabled={busy} onClick={() => approve.mutate(req)}>
          {t('authserver.consent.allow')}
        </AllowButton>
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
