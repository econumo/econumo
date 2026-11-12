import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'
import { pluralPick } from '@/lib/plural'
import { lockoutRemainingMs, MAX_FAILED_ATTEMPTS, readAttempts, type SecretKind } from './config'
import { formatCountdown } from './format'
import { checkSecret } from './secret'
import { PinPad } from './PinPad'
import { PatternPad } from './PatternPad'

// Shown when fewer attempts than this are left before the sign-out.
const WARN_REMAINING = 5

export type SecretFlow = 'enable' | 'change' | 'disable'
type Step = 'verify' | 'create' | 'confirm'

interface SecretDialogProps {
  open: boolean
  flow: SecretFlow
  /** the stored secret, for the current-secret step */
  current: { kind: SecretKind; length: number } | null
  onClose: () => void
  /** enable/change: the new secret (PIN digits or an encoded pattern), confirmed; disable: nothing */
  onComplete: (secret?: string) => void | Promise<void>
  onExceeded: () => void
}

// Remount it (key) for every new flow: the steps keep no state across opens.
export function SecretDialog({ open, flow, current, onClose, onComplete, onExceeded }: SecretDialogProps) {
  const { t, i18n } = useTranslation()
  const [step, setStep] = useState<Step>(flow === 'enable' ? 'create' : 'verify')
  const [kind, setKind] = useState<SecretKind>(flow === 'change' && current ? current.kind : 'pin')
  const [round, setRound] = useState(0)
  const [error, setError] = useState('')
  const [remaining, setRemaining] = useState(() => MAX_FAILED_ATTEMPTS - readAttempts().failedAttempts)
  const [lockout, setLockout] = useState(() => (flow === 'enable' ? 0 : lockoutRemainingMs()))
  const firstSecret = useRef('')

  const currentKind = current?.kind ?? 'pin'
  const padKind = step === 'verify' ? currentKind : kind
  const lockedOut = step === 'verify' && lockout > 0
  useEffect(() => {
    if (!lockedOut) {
      return
    }
    const timer = setInterval(() => setLockout(lockoutRemainingMs()), 1000)
    return () => clearInterval(timer)
  }, [lockedOut])

  const goTo = (next: Step, message = '') => {
    setStep(next)
    setError(message)
    setRound((n) => n + 1)
  }

  const verified = () => {
    if (flow === 'disable') {
      void onComplete()
      return
    }
    goTo('create')
  }

  const onVerify = async (value: string): Promise<boolean> => {
    setError('')
    const result = await checkSecret(value)
    switch (result.status) {
      case 'ok':
        verified()
        return true
      case 'wrong':
        setRemaining(result.remaining)
        setLockout(lockoutRemainingMs())
        setError(currentKind === 'pattern' ? t('appLock.lock_screen.wrong_pattern') : t('appLock.lock_screen.wrong_pin'))
        return false
      case 'locked-out':
        setLockout(lockoutRemainingMs())
        return false
      case 'exceeded':
        onExceeded()
        return false
      case 'unconfigured':
        onClose()
        return false
    }
  }

  const onCreate = (value: string): boolean => {
    firstSecret.current = value
    goTo('confirm')
    return true
  }

  const onConfirm = async (value: string): Promise<boolean> => {
    if (value !== firstSecret.current) {
      firstSecret.current = ''
      goTo('create', kind === 'pattern' ? t('appLock.settings.dialog.mismatch_pattern') : t('appLock.settings.dialog.mismatch'))
      return false
    }
    await onComplete(value)
    return true
  }

  const switchKind = () => {
    firstSecret.current = ''
    setKind(kind === 'pin' ? 'pattern' : 'pin')
    goTo('create')
  }

  const pattern = padKind === 'pattern'
  const title =
    step === 'verify'
      ? pattern
        ? t('appLock.settings.dialog.current_title_pattern')
        : t('appLock.settings.dialog.current_title')
      : step === 'confirm'
        ? pattern
          ? t('appLock.settings.dialog.confirm_title_pattern')
          : t('appLock.settings.dialog.confirm_title')
        : flow === 'change'
          ? pattern
            ? t('appLock.settings.dialog.new_title_pattern')
            : t('appLock.settings.dialog.new_title')
          : pattern
            ? t('appLock.settings.dialog.create_title_pattern')
            : t('appLock.settings.dialog.create_title')

  const shownError = lockedOut ? t('appLock.lock_screen.try_again_in', { time: formatCountdown(lockout) }) : error
  const hint =
    step === 'create'
      ? pattern
        ? t('appLock.settings.dialog.pattern_hint')
        : t('appLock.settings.dialog.pin_hint')
      : step === 'verify' && remaining < WARN_REMAINING
        ? pluralPick(t('appLock.lock_screen.attempts_left'), remaining, i18n.language)
        : undefined
  const onSubmit = step === 'verify' ? onVerify : step === 'create' ? onCreate : onConfirm

  return (
    <ResponsiveDialog open={open} onOpenChange={(value) => !value && onClose()} title={title}>
      <div className="flex flex-col items-center pt-1 pb-3">
        {pattern ? (
          <PatternPad
            key={`${step}-${round}`}
            onSubmit={onSubmit}
            disabled={lockedOut}
            error={shownError}
            hint={hint}
          />
        ) : (
          <PinPad
            key={`${step}-${round}`}
            mode={step === 'verify' ? 'verify' : 'create'}
            length={current?.length ?? 4}
            onSubmit={onSubmit}
            disabled={lockedOut}
            error={shownError}
            hint={hint}
            keyboard={open}
            submitLabel={step === 'confirm' ? t('appLock.settings.dialog.save') : undefined}
          />
        )}
        {step === 'create' && (
          <Button
            type="button"
            variant="link"
            className="mt-3 h-9 normal-case! tracking-normal!"
            onClick={switchKind}
          >
            {kind === 'pin' ? t('appLock.settings.dialog.use_pattern') : t('appLock.settings.dialog.use_pin')}
          </Button>
        )}
      </div>
    </ResponsiveDialog>
  )
}
