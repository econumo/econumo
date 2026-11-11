import { useEffect, useRef, useState } from 'react'
import { Fingerprint } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { Button } from '@/components/ui/button'
import { pluralPick } from '@/lib/plural'
import { verifyBiometric } from './biometric'
import { lockoutRemainingMs, MAX_FAILED_ATTEMPTS, readAttempts, resetAttempts } from './config'
import { formatCountdown } from './format'
import { checkPin } from './pin'
import { PinPad } from './PinPad'

// Shown when fewer attempts than this are left before the sign-out.
const WARN_REMAINING = 5

export type PinFlow = 'enable' | 'change' | 'disable'
type Step = 'verify' | 'create' | 'confirm'

interface SetPinDialogProps {
  open: boolean
  flow: PinFlow
  /** the stored PIN's length, for the current-PIN step */
  pinLength: number
  /** offered as an alternative to the current PIN */
  credentialId: string | null
  onClose: () => void
  /** enable/change: the new PIN, confirmed; disable: nothing */
  onComplete: (pin?: string) => void | Promise<void>
  onExceeded: () => void
}

// Remount it (key) for every new flow: the steps keep no state across opens.
export function SetPinDialog({ open, flow, pinLength, credentialId, onClose, onComplete, onExceeded }: SetPinDialogProps) {
  const { t, i18n } = useTranslation()
  const [step, setStep] = useState<Step>(flow === 'enable' ? 'create' : 'verify')
  const [round, setRound] = useState(0)
  const [error, setError] = useState('')
  const [remaining, setRemaining] = useState(() => MAX_FAILED_ATTEMPTS - readAttempts().failedAttempts)
  const [lockout, setLockout] = useState(() => (flow === 'enable' ? 0 : lockoutRemainingMs()))
  const firstPin = useRef('')

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

  const onVerify = async (pin: string): Promise<boolean> => {
    setError('')
    const result = await checkPin(pin)
    switch (result.status) {
      case 'ok':
        verified()
        return true
      case 'wrong':
        setRemaining(result.remaining)
        setLockout(lockoutRemainingMs())
        setError(t('appLock.lock_screen.wrong_pin'))
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

  const onBiometric = () => {
    if (credentialId === null) {
      return
    }
    setError('')
    // No await before this call: the click is the user gesture it needs.
    void verifyBiometric(credentialId).then((result) => {
      if (result === 'ok') {
        resetAttempts()
        setRemaining(MAX_FAILED_ATTEMPTS)
        setLockout(0)
        verified()
      } else if (result === 'failed') {
        setError(t('appLock.lock_screen.biometric_failed'))
      }
    })
  }

  const onCreate = (pin: string): boolean => {
    firstPin.current = pin
    goTo('confirm')
    return true
  }

  const onConfirm = async (pin: string): Promise<boolean> => {
    if (pin !== firstPin.current) {
      firstPin.current = ''
      goTo('create', t('appLock.settings.dialog.mismatch'))
      return false
    }
    await onComplete(pin)
    return true
  }

  const title =
    step === 'verify'
      ? t('appLock.settings.dialog.current_title')
      : step === 'confirm'
        ? t('appLock.settings.dialog.confirm_title')
        : flow === 'change'
          ? t('appLock.settings.dialog.new_title')
          : t('appLock.settings.dialog.create_title')

  const shownError = lockedOut ? t('appLock.lock_screen.try_again_in', { time: formatCountdown(lockout) }) : error

  return (
    <ResponsiveDialog open={open} onOpenChange={(value) => !value && onClose()} title={title}>
      <div className="flex flex-col items-center gap-2 pb-2">
        <p className="min-h-5 text-center text-sm text-muted-foreground">
          {step === 'create'
            ? t('appLock.settings.dialog.pin_hint')
            : step === 'verify' && remaining < WARN_REMAINING
              ? pluralPick(t('appLock.lock_screen.attempts_left'), remaining, i18n.language)
              : ''}
        </p>
        <PinPad
          key={`${step}-${round}`}
          mode={step === 'verify' ? 'verify' : 'create'}
          length={pinLength}
          onSubmit={step === 'verify' ? onVerify : step === 'create' ? onCreate : onConfirm}
          disabled={lockedOut}
          error={shownError}
          keyboard={open}
          submitLabel={step === 'confirm' ? t('appLock.settings.dialog.save') : undefined}
          biometric={
            step === 'verify' &&
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
      </div>
    </ResponsiveDialog>
  )
}
