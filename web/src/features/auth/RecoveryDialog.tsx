import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { isNotEmpty, isValidEmail, isValidPassword, isValidRecoveryCode } from '@/lib/validation'
import { useRemindPassword, useResetPassword } from './queries'

interface RecoveryForm {
  email: string
  code: string
  password: string
}

export function RecoveryDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  const [isCodeSent, setIsCodeSent] = useState(false)
  const remind = useRemindPassword()
  const reset = useResetPassword()
  const form = useForm<RecoveryForm>({ mode: 'onTouched', defaultValues: { email: '', code: '', password: '' } })
  const { register, handleSubmit, formState: { errors } } = form

  const sendCode = handleSubmit(async ({ email }) => {
    try {
      await remind.mutateAsync({ username: email })
      setIsCodeSent(true)
    } catch {
      // stay on the step — the inline error below explains
    }
  })

  const changePassword = handleSubmit(async ({ email, code, password }) => {
    try {
      await reset.mutateAsync({ username: email, code, password })
      onClose()
    } catch {
      // stay on the step — the inline error below explains
    }
  })

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={t('auth.access_recovery_modal.header')}
      description={t('auth.access_recovery_modal.information')}
    >
      <form onSubmit={isCodeSent ? changePassword : sendCode} className="flex flex-col gap-4" noValidate>
        <div className="flex flex-col gap-2">
          <Label htmlFor="recovery-email">{t('user.form.email.placeholder')}</Label>
          <Input
            className="h-11"
            id="recovery-email"
            type="email"
            disabled={isCodeSent}
            autoFocus={!isCodeSent}
            {...register('email', {
              validate: {
                required: (v) => isNotEmpty(v) || t('user.form.email.validation.required_field'),
                email: (v) => isValidEmail(v) || t('user.form.email.validation.invalid_email'),
              },
            })}
          />
          {errors.email ? <p className="text-sm text-destructive">{errors.email.message}</p> : null}
        </div>

        {isCodeSent ? (
          <>
            <p className="text-sm text-muted-foreground">{t('auth.access_recovery_modal.instruction')}</p>
            <div className="flex flex-col gap-2">
              <Label htmlFor="recovery-code">{t('user.form.code.placeholder')}</Label>
              <Input
                className="h-11"
                id="recovery-code"
                autoFocus
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={6}
                {...register('code', {
                  validate: {
                    required: (v) => isNotEmpty(v) || t('user.form.code.validation.required_field'),
                    code: (v) => isValidRecoveryCode(v) || t('user.form.code.validation.invalid_code'),
                  },
                })}
              />
              {errors.code ? <p className="text-sm text-destructive">{errors.code.message}</p> : null}
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="recovery-password">{t('user.form.password.placeholder')}</Label>
              <Input
                className="h-11"
                id="recovery-password"
                type="password"
                {...register('password', {
                  validate: {
                    required: (v) => isNotEmpty(v) || t('user.form.password.validation.required_field'),
                    password: (v) => isValidPassword(v) || t('user.form.password.validation.invalid_password'),
                  },
                })}
              />
              {errors.password ? <p className="text-sm text-destructive">{errors.password.message}</p> : null}
            </div>
            {reset.isError ? (
              <p className="text-sm text-destructive">{t('auth.access_recovery_modal.reset_failed')}</p>
            ) : null}
            <div className={dialogActionsClass}>
              <Button type="button" variant="secondary" onClick={onClose}>
                {t('common.button.cancel.label')}
              </Button>
              <Button type="submit" disabled={reset.isPending}>
                {t('auth.form.access_recovery.action.change_password.label')}
              </Button>
            </div>
          </>
        ) : (
          <>
            {remind.isError ? (
              <p className="text-sm text-destructive">{t('auth.access_recovery_modal.send_failed')}</p>
            ) : null}
            <div className={dialogActionsClass}>
              <Button type="button" variant="secondary" onClick={onClose}>
                {t('common.button.cancel.label')}
              </Button>
              <Button type="submit" disabled={remind.isPending}>
                {t('auth.form.access_recovery.action.recover.label')}
              </Button>
            </div>
          </>
        )}
      </form>
    </ResponsiveDialog>
  )
}
