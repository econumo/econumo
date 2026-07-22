import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { apiErrorMessage, retryAfterSeconds } from '@/lib/apiError'
import { isNotEmpty, isValidRecoveryCode } from '@/lib/validation'
import { useConfirmEmail, useLogin, useResendVerification } from './queries'

interface VerifyEmailForm {
  code: string
}

// Fallback only, for a server that sent no Retry-After. The authoritative
// value is the server's: the login 403 carries it (a code is already on its way
// when the dialog opens) and every resend response refreshes it.
const RESEND_COOLDOWN_SECONDS = 60

export function VerifyEmailDialog({ open, onClose, username, password, cooldownSeconds }: {
  open: boolean
  onClose: () => void
  username: string
  password: string
  cooldownSeconds?: number
}) {
  const { t } = useTranslation()
  const confirm = useConfirmEmail()
  const login = useLogin()
  const resend = useResendVerification()
  const [serverError, setServerError] = useState('')
  const [resent, setResent] = useState(false)
  // Tracked as a deadline rather than a decrementing counter so a backgrounded
  // tab (where timers are throttled) still lifts the block at the right moment.
  const initialWait = cooldownSeconds ?? RESEND_COOLDOWN_SECONDS
  const [resendAt, setResendAt] = useState(() => Date.now() + initialWait * 1000)
  const [cooldown, setCooldown] = useState(initialWait)
  const { register, handleSubmit, formState: { errors } } = useForm<VerifyEmailForm>({ mode: 'onTouched', defaultValues: { code: '' } })

  useEffect(() => {
    if (open) setResendAt(Date.now() + initialWait * 1000)
  }, [open, initialWait])

  useEffect(() => {
    if (!open) return
    const tick = () => setCooldown(Math.max(0, Math.ceil((resendAt - Date.now()) / 1000)))
    tick()
    const timer = setInterval(tick, 250)
    return () => clearInterval(timer)
  }, [open, resendAt])

  const onVerify = handleSubmit(async ({ code }) => {
    setServerError('')
    setResent(false)
    try {
      await confirm.mutateAsync({ username, code: code.trim() })
      // The code proved ownership; the silent re-login uses the credentials
      // still held by the login form, so the user lands in the app in one step.
      await login.mutateAsync({ username, password })
      window.location.assign('/')
    } catch (err) {
      setServerError(apiErrorMessage(err))
    }
  })

  const onResend = async () => {
    setServerError('')
    setResent(false)
    try {
      // The server decides the next allowed send and enforces it; whatever it
      // returns is the countdown, so the UI can never promise a shorter wait.
      const wait = await resend.mutateAsync({ username })
      setResent(true)
      setResendAt(Date.now() + (wait || RESEND_COOLDOWN_SECONDS) * 1000)
    } catch (err) {
      setServerError(apiErrorMessage(err))
      // A 429 (the attempt cap — a longer wait than the send gap) also reports
      // Retry-After; honour it so the button stays locked for as long as the
      // server will actually keep refusing.
      const blocked = retryAfterSeconds(err)
      if (blocked > 0) setResendAt(Date.now() + blocked * 1000)
    }
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={t('auth.verify_email.header')}
      description={t('auth.verify_email.information', { email: username })}
    >
      <form onSubmit={onVerify} className="flex flex-col gap-4" noValidate>
        <div className="flex flex-col gap-2">
          <Label htmlFor="verify-email-code">{t('user.form.code.placeholder')}</Label>
          <Input
            className="h-11"
            id="verify-email-code"
            autoFocus
            inputMode="numeric"
            autoComplete="one-time-code"
            maxLength={6}
            {...register('code', {
              validate: {
                required: (v) => isNotEmpty(v) || t('user.form.code.validation.required_field'),
                code: (v) => isValidRecoveryCode(v.trim()) || t('user.form.code.validation.invalid_code'),
              },
            })}
          />
          {errors.code ? <p className="text-sm text-destructive">{errors.code.message}</p> : null}
        </div>

        {serverError ? <p className="text-sm text-destructive">{serverError}</p> : null}
        {resent ? <p className="text-sm text-muted-foreground">{t('auth.verify_email.resent')}</p> : null}

        <Button
          type="button"
          variant="secondary"
          className="w-full h-11"
          onClick={onResend}
          disabled={resend.isPending || cooldown > 0}
          title={t('auth.verify_email.action.resend')}
        >
          {cooldown > 0
            ? t('auth.verify_email.action.resend_in', { seconds: cooldown })
            : t('auth.verify_email.action.resend')}
        </Button>

        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" disabled={confirm.isPending || login.isPending}>
            {t('auth.verify_email.action.verify')}
          </Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
