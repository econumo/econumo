import { useEffect, useRef, useState } from 'react'
import { skipToken, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { queryKeys } from '@/app/queryKeys'
import { RouterPage } from '@/app/router-pages'
import type { CurrentUserDto } from '@/api/dto/user'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { UserAvatar } from '@/components/UserAvatar'
import { Button } from '@/components/ui/button'
import { METRICS, trackEvent } from '@/lib/metrics'
import { pluralPick } from '@/lib/plural'
import logo from '@/assets/econumo.svg'
import darkLogo from '@/assets/econumo-dark.svg'
import { lockoutRemainingMs, MAX_FAILED_ATTEMPTS, readAttempts } from './config'
import { unlock, useAppLock } from './lockStore'
import { formatCountdown } from './format'
import { checkSecret } from './secret'
import { PinPad } from './PinPad'
import { PatternPad } from './PatternPad'

// Shown when fewer attempts than this are left before the sign-out.
const WARN_REMAINING = 5

function remainingAttempts(): number {
  return MAX_FAILED_ATTEMPTS - readAttempts().failedAttempts
}

export function LockScreen() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const { config } = useAppLock()
  // Read from the cache only, watching for the persisted cache to restore: a
  // request would be data fetched behind the lock.
  const { data: user } = useQuery<CurrentUserDto>({ queryKey: queryKeys.user, queryFn: skipToken })
  const [error, setError] = useState('')
  const [lockout, setLockout] = useState(() => lockoutRemainingMs())
  const [remaining, setRemaining] = useState(remainingAttempts)
  const [forgotOpen, setForgotOpen] = useState(false)
  const leaving = useRef(false)

  useEffect(() => {
    if (config === null) {
      window.location.reload()
    }
  }, [config])

  const lockedOut = lockout > 0
  useEffect(() => {
    if (!lockedOut) {
      return
    }
    const timer = setInterval(() => setLockout(lockoutRemainingMs()), 1000)
    return () => clearInterval(timer)
  }, [lockedOut])

  const unlockWith = (method: 'pin' | 'pattern') => {
    trackEvent(METRICS.APP_LOCK_UNLOCKED, { method })
    unlock()
  }

  const signOutToReset = () => {
    if (leaving.current) {
      return
    }
    leaving.current = true
    navigate(RouterPage.LOGOUT, { state: { keepPendingWrites: true, resetAppLock: true } })
  }

  if (config === null) {
    return null
  }
  const pattern = config.secret.kind === 'pattern'

  const onSubmit = async (secret: string): Promise<boolean> => {
    setError('')
    const result = await checkSecret(secret)
    switch (result.status) {
      case 'ok':
        unlockWith(config.secret.kind)
        return true
      case 'wrong': {
        setRemaining(result.remaining)
        setLockout(lockoutRemainingMs())
        setError(pattern ? t('appLock.lock_screen.wrong_pattern') : t('appLock.lock_screen.wrong_pin'))
        return false
      }
      case 'locked-out':
        setLockout(lockoutRemainingMs())
        return false
      case 'exceeded':
        trackEvent(METRICS.APP_LOCK_ATTEMPTS_EXCEEDED)
        signOutToReset()
        return false
      case 'unconfigured':
        window.location.reload()
        return false
    }
  }

  const onForgot = () => {
    setForgotOpen(false)
    trackEvent(METRICS.APP_LOCK_FORGOT_SECRET, { kind: config.secret.kind })
    signOutToReset()
  }

  return (
    <div className="flex min-h-dvh flex-col items-center justify-center gap-6 bg-econumo-card p-4 pt-[max(env(safe-area-inset-top),1rem)] pb-[max(env(safe-area-inset-bottom),1rem)]">
      <img src={logo} width={194} height={20} alt={t('common.econumo.label')} className="dark:hidden" />
      <img src={darkLogo} width={194} height={20} alt={t('common.econumo.label')} className="hidden dark:block" />
      <div className="flex w-full max-w-sm flex-col items-center gap-4 rounded-xl border bg-background px-6 py-8 shadow-sm">
        {user && (
          <div className="flex flex-col items-center gap-2">
            <UserAvatar avatar={user.avatar} size="card" />
            <span className="text-sm text-muted-foreground">{user.name}</span>
          </div>
        )}
        <h1 className="text-xl">{pattern ? t('appLock.lock_screen.title_pattern') : t('appLock.lock_screen.title')}</h1>
        {pattern ? (
          <PatternPad onSubmit={onSubmit} disabled={lockedOut} error={error} />
        ) : (
          <PinPad
            mode="verify"
            length={config.secret.length}
            onSubmit={onSubmit}
            disabled={lockedOut}
            error={error}
            keyboard={!forgotOpen}
          />
        )}
        {(lockedOut || remaining < WARN_REMAINING) && (
          <div className="flex flex-col items-center gap-1 text-center text-sm text-muted-foreground">
            {lockedOut && <span data-testid="lockout">{t('appLock.lock_screen.try_again_in', { time: formatCountdown(lockout) })}</span>}
            {remaining < WARN_REMAINING && (
              <span>{pluralPick(t('appLock.lock_screen.attempts_left'), remaining, i18n.language)}</span>
            )}
          </div>
        )}
        <Button type="button" variant="link" className="normal-case! tracking-normal!" onClick={() => setForgotOpen(true)}>
          {pattern ? t('appLock.lock_screen.forgot_pattern') : t('appLock.lock_screen.forgot_pin')}
        </Button>
      </div>
      <ConfirmDialog
        open={forgotOpen}
        onClose={() => setForgotOpen(false)}
        onConfirm={onForgot}
        title={pattern ? t('appLock.lock_screen.forgot_title_pattern') : t('appLock.lock_screen.forgot_title')}
        question={t('appLock.lock_screen.forgot_question')}
        confirmLabel={t('appLock.lock_screen.forgot_confirm')}
        cancelLabel={t('common.button.cancel.label')}
        offlineOk
      />
    </div>
  )
}
