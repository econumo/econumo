import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { LoadingDialog } from '@/components/LoadingDialog'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { apiErrorMessage, apiFieldErrors, retryAfterSeconds } from '@/lib/apiError'
import { isNotEmpty, isValidEmail, isValidRecoveryCode } from '@/lib/validation'
import { RouterPage } from '@/app/router-pages'
import { useConfirmEmailChange, useRequestEmailChange, useResendEmailChangeCode } from '@/features/user/queries'
import { SettingsShell } from './SettingsShell'

interface RequestFormErrors {
  newEmail?: string
  password?: string
}

// Fallback only, mirroring VerifyEmailDialog: the server's Retry-After on the
// request/resend responses is authoritative and always overrides this.
const RESEND_COOLDOWN_SECONDS = 60

export function ChangeEmailPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const requestChange = useRequestEmailChange()
  const confirmChange = useConfirmEmailChange()
  const resend = useResendEmailChangeCode()

  const [phase, setPhase] = useState<'request' | 'confirm'>('request')

  const [newEmail, setNewEmail] = useState('')
  const [password, setPassword] = useState('')
  const [requestErrors, setRequestErrors] = useState<RequestFormErrors>({})
  const [requestError, setRequestError] = useState('')

  const [code, setCode] = useState('')
  const [codeError, setCodeError] = useState<string | null>(null)
  const [confirmError, setConfirmError] = useState('')
  const [resent, setResent] = useState(false)
  const [resendAt, setResendAt] = useState(() => Date.now() + RESEND_COOLDOWN_SECONDS * 1000)
  const [cooldown, setCooldown] = useState(RESEND_COOLDOWN_SECONDS)

  const [success, setSuccess] = useState(false)

  // Tracked as a deadline rather than a decrementing counter so a backgrounded
  // tab (where timers are throttled) still lifts the block at the right moment.
  useEffect(() => {
    if (phase !== 'confirm') return
    const tick = () => setCooldown(Math.max(0, Math.ceil((resendAt - Date.now()) / 1000)))
    tick()
    const timer = setInterval(tick, 250)
    return () => clearInterval(timer)
  }, [phase, resendAt])

  const validateRequest = (): boolean => {
    const next: RequestFormErrors = {}
    if (!isNotEmpty(newEmail)) {
      next.newEmail = t('user.form.email.validation.required_field')
    } else if (!isValidEmail(newEmail)) {
      next.newEmail = t('user.form.email.validation.invalid_email')
    }
    if (!isNotEmpty(password)) {
      next.password = t('user.change_email.form.password.validation.required_field')
    }
    setRequestErrors(next)
    return Object.keys(next).length === 0
  }

  const submitRequest = () => {
    setRequestError('')
    if (!validateRequest()) {
      return
    }
    requestChange.mutate(
      { newEmail, password },
      {
        onSuccess: () => {
          setPhase('confirm')
          setResendAt(Date.now() + RESEND_COOLDOWN_SECONDS * 1000)
          setCooldown(RESEND_COOLDOWN_SECONDS)
        },
        onError: (err) => {
          setRequestErrors({
            newEmail: apiFieldErrors(err, 'newEmail')?.[0],
            password: apiFieldErrors(err, 'password')?.[0],
          })
          setRequestError(apiErrorMessage(err))
        },
      },
    )
  }

  const submitConfirm = () => {
    setConfirmError('')
    const trimmed = code.trim()
    if (!isNotEmpty(trimmed)) {
      setCodeError(t('user.form.code.validation.required_field'))
      return
    }
    if (!isValidRecoveryCode(trimmed)) {
      setCodeError(t('user.form.code.validation.invalid_code'))
      return
    }
    setCodeError(null)
    confirmChange.mutate(
      { code: trimmed },
      {
        onSuccess: () => setSuccess(true),
        onError: (err) => setConfirmError(apiErrorMessage(err)),
      },
    )
  }

  const onResend = async () => {
    setConfirmError('')
    setResent(false)
    try {
      // The server decides the next allowed send and enforces it; whatever it
      // returns is the countdown, so the UI can never promise a shorter wait.
      const wait = await resend.mutateAsync()
      setResent(true)
      setResendAt(Date.now() + (wait || RESEND_COOLDOWN_SECONDS) * 1000)
    } catch (err) {
      setConfirmError(apiErrorMessage(err))
      const blocked = retryAfterSeconds(err)
      if (blocked > 0) setResendAt(Date.now() + blocked * 1000)
    }
  }

  const closeSuccess = () => {
    setSuccess(false)
    void navigate(RouterPage.SETTINGS_PROFILE)
  }

  return (
    <SettingsShell
      title={t('user.change_email.header')}
      backTo={RouterPage.SETTINGS_PROFILE}
      crumbs={[
        { label: t('settings.page.header_desktop'), to: RouterPage.SETTINGS },
        { label: t('user.page.settings.profile.menu_item'), to: RouterPage.SETTINGS_PROFILE },
      ]}
    >
      {phase === 'request' ? (
        <form
          className="flex max-w-md flex-col gap-4 py-2"
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            submitRequest()
          }}
        >
          <div className="flex flex-col gap-2">
            <Label htmlFor="ce-new-email">{t('user.change_email.form.new_email.label')}</Label>
            <Input
              id="ce-new-email"
              type="email"
              placeholder={t('user.change_email.form.new_email.placeholder')}
              value={newEmail}
              onChange={(e) => setNewEmail(e.target.value)}
            />
            {requestErrors.newEmail ? <p className="text-sm text-destructive">{requestErrors.newEmail}</p> : null}
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="ce-password">{t('user.change_email.form.password.label')}</Label>
            <Input
              id="ce-password"
              type="password"
              placeholder={t('user.change_email.form.password.placeholder')}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            {requestErrors.password ? <p className="text-sm text-destructive">{requestErrors.password}</p> : null}
          </div>
          {requestError ? <p className="text-sm text-destructive">{requestError}</p> : null}
          <Button type="submit" className="h-10 w-full font-normal lg:w-auto lg:min-w-44 lg:self-start" disabled={requestChange.isPending}>
            {t('user.change_email.form.submit.label')}
          </Button>
        </form>
      ) : (
        <form
          className="flex max-w-md flex-col gap-4 py-2"
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            submitConfirm()
          }}
        >
          <p className="text-sm text-muted-foreground">{t('user.change_email.confirm.information', { email: newEmail })}</p>
          <div className="flex flex-col gap-2">
            <Label htmlFor="ce-code">{t('user.form.code.placeholder')}</Label>
            <Input
              id="ce-code"
              autoFocus
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
            {codeError ? <p className="text-sm text-destructive">{codeError}</p> : null}
          </div>
          {confirmError ? <p className="text-sm text-destructive">{confirmError}</p> : null}
          {resent ? <p className="text-sm text-muted-foreground">{t('user.change_email.confirm.resent')}</p> : null}
          <Button
            type="button"
            variant="secondary"
            className="h-11 w-full"
            onClick={onResend}
            disabled={resend.isPending || cooldown > 0}
            title={t('user.change_email.confirm.action.resend')}
          >
            {cooldown > 0
              ? t('user.change_email.confirm.action.resend_in', { seconds: cooldown })
              : t('user.change_email.confirm.action.resend')}
          </Button>
          <Button type="submit" className="h-10 w-full font-normal lg:w-auto lg:min-w-44 lg:self-start" disabled={confirmChange.isPending}>
            {t('user.change_email.confirm.action.verify')}
          </Button>
        </form>
      )}

      <LoadingDialog open={requestChange.isPending} label={t('user.change_email.loading.label')} />

      <ResponsiveDialog
        open={success}
        onOpenChange={(o) => !o && closeSuccess()}
        title={t('user.change_email.success.text')}
      >
        <Button type="button" className="h-11 w-full" onClick={closeSuccess}>
          {t('common.button.close.label')}
        </Button>
      </ResponsiveDialog>
    </SettingsShell>
  )
}
