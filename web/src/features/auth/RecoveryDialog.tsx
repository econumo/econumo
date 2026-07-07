import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
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
    await remind.mutateAsync({ username: email })
    setIsCodeSent(true)
  })

  const changePassword = handleSubmit(async ({ email, code, password }) => {
    await reset.mutateAsync({ username: email, code, password })
    onClose()
  })

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={t('modules.user.modal.access_recovery.header')}
      description={t('modules.user.modal.access_recovery.information')}
      dismissible={false}
    >
      <form onSubmit={isCodeSent ? changePassword : sendCode} className="flex flex-col gap-4" noValidate>
        <div className="flex flex-col gap-2">
          <Label htmlFor="recovery-email">{t('modules.user.form.user.email.placeholder')}</Label>
          <Input
            id="recovery-email"
            type="email"
            disabled={isCodeSent}
            autoFocus={!isCodeSent}
            {...register('email', {
              validate: {
                required: (v) => isNotEmpty(v) || t('modules.user.form.user.email.validation.required_field'),
                email: (v) => isValidEmail(v) || t('modules.user.form.user.email.validation.invalid_email'),
              },
            })}
          />
          {errors.email ? <p className="text-sm text-destructive">{errors.email.message}</p> : null}
        </div>

        {isCodeSent ? (
          <>
            <p className="text-sm text-muted-foreground">{t('modules.user.modal.access_recovery.instruction')}</p>
            <div className="flex flex-col gap-2">
              <Label htmlFor="recovery-code">{t('modules.user.form.user.code.placeholder')}</Label>
              <Input
                id="recovery-code"
                autoFocus
                {...register('code', {
                  validate: {
                    required: (v) => isNotEmpty(v) || t('modules.user.form.user.code.validation.required_field'),
                    code: (v) => isValidRecoveryCode(v) || t('modules.user.form.user.code.validation.invalid_code'),
                  },
                })}
              />
              {errors.code ? <p className="text-sm text-destructive">{errors.code.message}</p> : null}
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="recovery-password">{t('modules.user.form.user.password.placeholder')}</Label>
              <Input
                id="recovery-password"
                type="password"
                {...register('password', {
                  validate: {
                    required: (v) => isNotEmpty(v) || t('modules.user.form.user.password.validation.required_field'),
                    password: (v) => isValidPassword(v) || t('modules.user.form.user.password.validation.invalid_password'),
                  },
                })}
              />
              {errors.password ? <p className="text-sm text-destructive">{errors.password.message}</p> : null}
            </div>
            <Button type="submit" className="w-full max-md:h-11" disabled={reset.isPending}>
              {t('modules.user.form.access_recovery.action.change_password.label')}
            </Button>
          </>
        ) : (
          <Button type="submit" className="w-full max-md:h-11" disabled={remind.isPending}>
            {t('modules.user.form.access_recovery.action.recover.label')}
          </Button>
        )}
      </form>
    </ResponsiveDialog>
  )
}
