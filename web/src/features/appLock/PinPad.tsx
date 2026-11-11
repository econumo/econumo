import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Delete } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export const PIN_MIN_LENGTH = 4
export const PIN_MAX_LENGTH = 8

export interface PinPadProps {
  /** verify: a known number of digits, submitted as soon as the last one is in. create: 4-8 digits and a Continue button. */
  mode: 'verify' | 'create'
  /** verify mode: the stored PIN's length. */
  length?: number
  /** Resolve false to reject the PIN: the dots shake and clear. Either way the entry is cleared. */
  onSubmit: (pin: string) => boolean | Promise<boolean>
  disabled?: boolean
  /** Announced through the pad's live region. */
  error?: string
  /** Rendered in the keypad's bottom-left slot (the biometric button). */
  biometric?: ReactNode
  submitLabel?: string
  /** Physical keyboard input; turn it off while another surface (a dialog) is on top. */
  keyboard?: boolean
  className?: string
}

const KEYS = ['1', '2', '3', '4', '5', '6', '7', '8', '9'] as const

function typingInField(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false
  }
  return target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
}

export function PinPad({
  mode,
  length = PIN_MIN_LENGTH,
  onSubmit,
  disabled = false,
  error,
  biometric,
  submitLabel,
  keyboard = true,
  className,
}: PinPadProps) {
  const { t } = useTranslation()
  const [pin, setPin] = useState('')
  const [busy, setBusy] = useState(false)
  const [shakes, setShakes] = useState(0)
  const pinRef = useRef('')
  const busyRef = useRef(false)
  const mounted = useRef(true)
  const max = mode === 'verify' ? length : PIN_MAX_LENGTH
  const blocked = disabled || busy

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  const setEntry = (value: string) => {
    pinRef.current = value
    setPin(value)
  }

  const submit = (value: string) => {
    busyRef.current = true
    setBusy(true)
    void Promise.resolve()
      .then(() => onSubmit(value))
      .catch(() => false)
      .then((accepted) => {
        busyRef.current = false
        if (!mounted.current) {
          return
        }
        setBusy(false)
        setEntry('')
        if (!accepted) {
          setShakes((n) => n + 1)
        }
      })
  }

  const actions = {
    digit(d: string) {
      if (disabled || busyRef.current || pinRef.current.length >= max) {
        return
      }
      const next = pinRef.current + d
      setEntry(next)
      if (mode === 'verify' && next.length === length) {
        submit(next)
      }
    },
    erase() {
      if (disabled || busyRef.current) {
        return
      }
      setEntry(pinRef.current.slice(0, -1))
    },
    confirm() {
      if (mode !== 'create' || disabled || busyRef.current || pinRef.current.length < PIN_MIN_LENGTH) {
        return
      }
      submit(pinRef.current)
    },
  }
  const actionsRef = useRef(actions)
  actionsRef.current = actions

  useEffect(() => {
    if (!keyboard) {
      return
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey || typingInField(event.target)) {
        return
      }
      if (/^[0-9]$/.test(event.key)) {
        actionsRef.current.digit(event.key)
      } else if (event.key === 'Backspace') {
        actionsRef.current.erase()
      } else if (event.key === 'Enter') {
        actionsRef.current.confirm()
      } else {
        return
      }
      event.preventDefault()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [keyboard])

  const dots = mode === 'verify' ? length : Math.max(PIN_MIN_LENGTH, pin.length)
  const keyClass = 'size-16 rounded-full text-2xl font-normal'

  return (
    <div className={cn('flex flex-col items-center gap-6', className)}>
      <div
        key={shakes}
        role="img"
        aria-label={t('appLock.pin_pad.entered', { count: pin.length })}
        data-testid="pin-dots"
        data-filled={pin.length}
        className={cn('flex h-4 items-center gap-3', shakes > 0 && 'pin-shake')}
      >
        {Array.from({ length: dots }, (_, i) => (
          <span
            key={i}
            className={cn(
              'size-3 rounded-full border border-foreground/40 transition-colors',
              i < pin.length && 'border-foreground bg-foreground',
            )}
          />
        ))}
      </div>
      <p aria-live="assertive" className="min-h-5 text-center text-sm text-destructive">
        {error}
      </p>
      <div className="grid grid-cols-3 gap-4">
        {KEYS.map((d) => (
          <Button key={d} type="button" variant="secondary" className={keyClass} disabled={blocked} onClick={() => actions.digit(d)}>
            {d}
          </Button>
        ))}
        <div className="flex size-16 items-center justify-center">{biometric}</div>
        <Button type="button" variant="secondary" className={keyClass} disabled={blocked} onClick={() => actions.digit('0')}>
          0
        </Button>
        <Button
          type="button"
          variant="ghost"
          className={keyClass}
          disabled={blocked || pin.length === 0}
          aria-label={t('appLock.pin_pad.delete')}
          onClick={actions.erase}
        >
          <Delete className="size-6" aria-hidden="true" />
        </Button>
      </div>
      {mode === 'create' && (
        <Button type="button" className="w-full max-w-60" disabled={blocked || pin.length < PIN_MIN_LENGTH} onClick={actions.confirm}>
          {submitLabel ?? t('appLock.pin_pad.continue')}
        </Button>
      )}
    </div>
  )
}
