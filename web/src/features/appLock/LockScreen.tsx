import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Fingerprint } from 'lucide-react'
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
import { verifyBiometric } from './biometric'
import { lockoutRemainingMs, MAX_FAILED_ATTEMPTS, readAttempts } from './config'
import { unlock, useAppLock } from './lockStore'
import { checkPin } from './pin'
import { PinPad } from './PinPad'

// Shown when fewer attempts than this are left before the sign-out.
const WARN_REMAINING = 5

function formatCountdown(ms: number): string {
  const seconds = Math.ceil(ms / 1000)
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
}

function remainingAttempts(): number {
  return MAX_FAILED_ATTEMPTS - readAttempts().failedAttempts
}

function autoBiometricAllowed(): boolean {
  return (
    document.visibilityState === 'visible' &&
    document.hasFocus() &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(pointer: coarse)').matches
  )
}

export function LockScreen() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { config } = useAppLock()
  // Read from the cache only: a request would be data fetched behind the lock.
  const user = queryClient.getQueryData<CurrentUserDto>(queryKeys.user)
  const [error, setError] = useState('')
  const [lockout, setLockout] = useState(() => lockoutRemainingMs())
  const [remaining, setRemaining] = useState(remainingAttempts)
  const [forgotOpen, setForgotOpen] = useState(false)
  const autoTried = useRef(false)
  const autoAttempt = useRef<AbortController | null>(null)
  const leaving = useRef(false)
  const credentialId = config?.biometric?.credentialId ?? null

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

  const unlockWith = (method: 'pin' | 'biometric') => {
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

  // Once per appearance, and only where the prompt is likely to be allowed
  // without a tap; a browser that wants a gesture rejects it and the user
  // taps the button. The timer lets StrictMode's throwaway mount cancel itself.
  useEffect(() => {
    if (credentialId === null || autoTried.current || !autoBiometricAllowed()) {
      return
    }
    const controller = new AbortController()
    const timer = setTimeout(() => {
      autoTried.current = true
      autoAttempt.current = controller
      void verifyBiometric(credentialId, controller.signal).then((result) => {
        if (result === 'ok' && !controller.signal.aborted) {
          unlockWith('biometric')
        }
      })
    }, 0)
    return () => {
      clearTimeout(timer)
      controller.abort()
    }
  }, [credentialId])

  if (config === null) {
    return null
  }

  const onBiometric = () => {
    if (credentialId === null) {
      return
    }
    autoAttempt.current?.abort()
    setError('')
    // No await before this call: the click is the user gesture it needs.
    void verifyBiometric(credentialId).then((result) => {
      if (result === 'ok') {
        unlockWith('biometric')
      } else if (result === 'failed') {
        setError(t('appLock.lock_screen.biometric_failed'))
      }
    })
  }

  const onSubmit = async (pin: string): Promise<boolean> => {
    setError('')
    const result = await checkPin(pin)
    switch (result.status) {
      case 'ok':
        unlockWith('pin')
        return true
      case 'wrong': {
        setRemaining(result.remaining)
        setLockout(lockoutRemainingMs())
        setError(t('appLock.lock_screen.wrong_pin'))
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
    trackEvent(METRICS.APP_LOCK_FORGOT_PIN)
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
        <h1 className="text-xl">{t('appLock.lock_screen.title')}</h1>
        <PinPad
          mode="verify"
          length={config.pin.length}
          onSubmit={onSubmit}
          disabled={lockedOut}
          error={error}
          keyboard={!forgotOpen}
          biometric={
            credentialId !== null && (
              <Button
                type="button"
                variant="ghost"
                className="size-16 rounded-full"
                aria-label={t('appLock.lock_screen.biometric')}
                onClick={onBiometric}
              >
                <Fingerprint className="size-7" aria-hidden="true" />
              </Button>
            )
          }
        />
        <div className="flex min-h-10 flex-col items-center gap-1 text-center text-sm text-muted-foreground">
          {lockedOut && <span data-testid="lockout">{t('appLock.lock_screen.try_again_in', { time: formatCountdown(lockout) })}</span>}
          {remaining < WARN_REMAINING && (
            <span>{pluralPick(t('appLock.lock_screen.attempts_left'), remaining, i18n.language)}</span>
          )}
        </div>
        <Button type="button" variant="link" onClick={() => setForgotOpen(true)}>
          {t('appLock.lock_screen.forgot_pin')}
        </Button>
      </div>
      <ConfirmDialog
        open={forgotOpen}
        onClose={() => setForgotOpen(false)}
        onConfirm={onForgot}
        title={t('appLock.lock_screen.forgot_title')}
        question={t('appLock.lock_screen.forgot_question')}
        confirmLabel={t('appLock.lock_screen.forgot_confirm')}
        cancelLabel={t('common.button.cancel.label')}
        offlineOk
      />
    </div>
  )
}
