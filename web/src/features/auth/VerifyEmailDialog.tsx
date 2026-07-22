import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { apiErrorMessage } from '@/lib/apiError'
import { isNotEmpty, isValidRecoveryCode } from '@/lib/validation'
import { useLogin, useResendVerification } from './queries'

interface VerifyEmailForm {
  code: string
}

export function VerifyEmailDialog({ open, onClose, username, password }: {
  open: boolean
  onClose: () => void
  username: string
  password: string
}) {
  const { t } = useTranslation()
  const login = useLogin()
  const resend = useResendVerification()
  const [serverError, setServerError] = useState('')
  const [resent, setResent] = useState(false)
  const { register, handleSubmit, formState: { errors } } = useForm<VerifyEmailForm>({ mode: 'onTouched', defaultValues: { code: '' } })

  const onVerify = handleSubmit(async ({ code }) => {
    setServerError('')
    setResent(false)
    try {
      await login.mutateAsync({ username, password, code: code.trim() })
      window.location.assign('/')
    } catch (err) {
      setServerError(apiErrorMessage(err))
    }
  })

  const onResend = async () => {
    setServerError('')
    setResent(false)
    try {
      await resend.mutateAsync({ username, password })
      setResent(true)
    } catch (err) {
      setServerError(apiErrorMessage(err))
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

        <Button type="button" variant="secondary" className="w-full h-11" onClick={onResend} disabled={resend.isPending}>
          {t('auth.verify_email.action.resend')}
        </Button>

        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" disabled={login.isPending}>
            {t('auth.verify_email.action.verify')}
          </Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
