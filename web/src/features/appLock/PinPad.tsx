import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Delete } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export const PIN_MIN_LENGTH = 4
export const PIN_MAX_LENGTH = 8
// How long a rejected entry stays on screen, red, before it clears.
export const REJECT_FLASH_MS = 450

export interface PinPadProps {
  /** verify: a known number of digits, submitted as soon as the last one is in. create: 4-8 digits and a Continue button. */
  mode: 'verify' | 'create'
  /** verify mode: the stored PIN's length. */
  length?: number
  /** Resolve false to reject the PIN: the dots shake and clear. Either way the entry is cleared. */
  onSubmit: (pin: string) => boolean | Promise<boolean>
  disabled?: boolean
  /** Shown under the dots in the error colour and announced; wins over the hint. */
  error?: string
  /** A quiet line under the dots ("4 to 8 digits", attempts left). */
  hint?: string
  /** Rendered in the keypad's bottom-left slot (the biometric button). */
  biometric?: ReactNode
  submitLabel?: string
  /** Physical keyboard input; turn it off while another surface (a dialog) is on top. */
  keyboard?: boolean
  className?: string
}

const KEYS = ['1', '2', '3', '4', '5', '6', '7', '8', '9'] as const

const KEY_CLASS =
  'flex size-[4.25rem] touch-manipulation items-center justify-center rounded-full text-foreground outline-none select-none [-webkit-tap-highlight-color:transparent] transition-[transform,background-color,opacity] duration-100 active:scale-[0.94] focus-visible:ring-3 focus-visible:ring-ring/60 disabled:pointer-events-none disabled:opacity-40'

interface PadKeyProps {
  /** digit: a filled round key; icon: a bare key-sized target (backspace, biometrics). */
  variant?: 'digit' | 'icon'
  label?: string
  disabled?: boolean
  hidden?: boolean
  onPress: () => void
  children: ReactNode
}

// A tap must not leave the focus ring behind, so a key never takes focus from
// a pointer; the keyboard still reaches it with Tab.
export function PadKey({ variant = 'digit', label, disabled, hidden, onPress, children }: PadKeyProps) {
  return (
    <button
      type="button"
      aria-label={label}
      disabled={disabled}
      onMouseDown={(event) => event.preventDefault()}
      onClick={onPress}
      className={cn(
        KEY_CLASS,
        variant === 'digit'
          ? 'bg-secondary text-[1.75rem] leading-none font-light tabular-nums hover:bg-econumo-hover active:bg-econumo-hover'
          : 'text-muted-foreground hover:bg-secondary hover:text-foreground active:bg-secondary',
        hidden && 'invisible',
      )}
    >
      {children}
    </button>
  )
}

function typingInField(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false
  }
  return target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
}

const INTERACTIVE = 'button, a[href], input, select, textarea, summary, [role="button"], [role="link"]'

export function PinPad({
  mode,
  length = PIN_MIN_LENGTH,
  onSubmit,
  disabled = false,
  error,
  hint,
  biometric,
  submitLabel,
  keyboard = true,
  className,
}: PinPadProps) {
  const { t } = useTranslation()
  const [pin, setPin] = useState('')
  const [busy, setBusy] = useState(false)
  const [shakes, setShakes] = useState(0)
  // The digit count of a rejected entry, shown red while the dots shake.
  const [flash, setFlash] = useState<number | null>(null)
  const pinRef = useRef('')
  const busyRef = useRef(false)
  const mounted = useRef(true)
  const flashTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const root = useRef<HTMLDivElement>(null)
  const max = mode === 'verify' ? length : PIN_MAX_LENGTH
  const blocked = disabled || busy

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      if (flashTimer.current !== null) {
        clearTimeout(flashTimer.current)
      }
    }
  }, [])

  const stopFlash = () => {
    if (flashTimer.current !== null) {
      clearTimeout(flashTimer.current)
      flashTimer.current = null
    }
    setFlash(null)
  }

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
          setFlash(value.length)
          flashTimer.current = setTimeout(() => {
            flashTimer.current = null
            setFlash(null)
          }, REJECT_FLASH_MS)
        }
      })
  }

  const actions = {
    digit(d: string) {
      if (disabled || busyRef.current || pinRef.current.length >= max) {
        return
      }
      stopFlash()
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
        // Enter belongs to whatever else has focus ("Forgot PIN?", a dialog's close button).
        const target = event.target
        const elsewhere =
          target instanceof Element && target.closest(INTERACTIVE) !== null && !root.current?.contains(target)
        if (mode !== 'create' || elsewhere) {
          return
        }
        actionsRef.current.confirm()
      } else {
        return
      }
      event.preventDefault()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [keyboard, mode])

  const slots = mode === 'verify' ? length : Math.max(PIN_MIN_LENGTH, pin.length, flash ?? 0)
  const shown = flash ?? pin.length
  const message = error || hint

  return (
    <div ref={root} className={cn('flex flex-col items-center', className)}>
      <div
        key={shakes}
        role="img"
        aria-label={t('appLock.pin_pad.entered', { count: pin.length })}
        data-testid="pin-dots"
        data-filled={pin.length}
        className={cn('flex h-4 items-center justify-center gap-4', shakes > 0 && 'pin-shake')}
      >
        {Array.from({ length: slots }, (_, i) => (
          <span
            key={i}
            className={cn(
              'size-3.5 rounded-full border-[1.5px] transition-colors duration-150',
              i < shown
                ? flash !== null
                  ? 'border-destructive bg-destructive'
                  : 'pin-dot-fill border-primary bg-primary'
                : flash !== null
                  ? 'border-destructive/50'
                  : 'border-foreground/25',
            )}
          />
        ))}
      </div>
      <p
        aria-live="assertive"
        className={cn('mt-3 min-h-5 max-w-64 text-center text-sm', error ? 'text-destructive' : 'text-muted-foreground')}
      >
        {message}
      </p>
      <div className="mt-5 grid grid-cols-3 gap-x-6 gap-y-3">
        {KEYS.map((d) => (
          <PadKey key={d} disabled={blocked} onPress={() => actions.digit(d)}>
            {d}
          </PadKey>
        ))}
        <div className="flex size-[4.25rem] items-center justify-center">{biometric}</div>
        <PadKey disabled={blocked} onPress={() => actions.digit('0')}>
          0
        </PadKey>
        <PadKey
          variant="icon"
          label={t('appLock.pin_pad.delete')}
          disabled={blocked || pin.length === 0}
          hidden={pin.length === 0}
          onPress={actions.erase}
        >
          <Delete className="size-7" strokeWidth={1.5} aria-hidden="true" />
        </PadKey>
      </div>
      {mode === 'create' && (
        <Button
          type="button"
          className="mt-6 h-11 w-full max-w-[15.75rem] text-[0.9375rem] normal-case! tracking-normal!"
          disabled={blocked || pin.length < PIN_MIN_LENGTH}
          onClick={actions.confirm}
        >
          {submitLabel ?? t('appLock.pin_pad.continue')}
        </Button>
      )}
    </div>
  )
}
