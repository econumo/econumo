import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { LoadingDialog } from '@/components/LoadingDialog'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { isNotEmpty, isValidPassword } from '@/lib/validation'
import { RouterPage } from '@/app/router-pages'
import { useUpdatePassword } from '@/features/user/queries'
import { SettingsShell } from './SettingsShell'

interface FormErrors {
  oldPassword?: string
  newPassword?: string
  newPasswordRetry?: string
}

export function ChangePasswordPage() {
  const { t } = useTranslation()
  const updatePassword = useUpdatePassword()

  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [newPasswordRetry, setNewPasswordRetry] = useState('')
  const [errors, setErrors] = useState<FormErrors>({})
  const [outcome, setOutcome] = useState<'success' | 'error' | null>(null)
  const [errorMessage, setErrorMessage] = useState('')

  const validate = (): boolean => {
    const next: FormErrors = {}
    if (!isNotEmpty(oldPassword)) {
      next.oldPassword = t('modules.user.form.change_password.password.validation.invalid_password')
    }
    if (!isValidPassword(newPassword)) {
      next.newPassword = t('modules.user.form.user.password.validation.invalid_password')
    } else if (newPassword === oldPassword) {
      next.newPassword = t('modules.user.form.change_password.new_password.validation.not_equals')
    }
    if (!isNotEmpty(newPasswordRetry)) {
      next.newPasswordRetry = t('modules.user.form.user.password_retry.validation.required_field')
    } else if (!isValidPassword(newPasswordRetry)) {
      next.newPasswordRetry = t('modules.user.form.user.password_retry.validation.invalid_password')
    } else if (newPasswordRetry !== newPassword) {
      next.newPasswordRetry = t('modules.user.form.change_password.new_password_retry.validation.not_equals')
    }
    setErrors(next)
    return Object.keys(next).length === 0
  }

  const submit = () => {
    if (!validate()) {
      return
    }
    updatePassword.mutate(
      { oldPassword, newPassword },
      {
        onSuccess: () => {
          setOldPassword('')
          setNewPassword('')
          setNewPasswordRetry('')
          setErrors({})
          setOutcome('success')
        },
        onError: () => {
          setErrorMessage(t('modules.user.modal.change_password_error.text'))
          setOutcome('error')
        },
      },
    )
  }

  return (
    <SettingsShell
      title={t('modules.user.page.settings.profile.change_password.header')}
      backTo={RouterPage.SETTINGS_PROFILE}
      crumbs={[
        { label: t('pages.settings.settings.header_desktop'), to: RouterPage.SETTINGS },
        { label: t('modules.user.page.settings.profile.menu_item'), to: RouterPage.SETTINGS_PROFILE },
      ]}
    >
      <form
        className="flex max-w-md flex-col gap-4 py-2"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="flex flex-col gap-2">
          <Label htmlFor="cp-old">{t('modules.user.form.change_password.password.label')}</Label>
          <Input
            id="cp-old"
            type="password"
            placeholder={t('modules.user.form.change_password.password.placeholder')}
            value={oldPassword}
            onChange={(e) => setOldPassword(e.target.value)}
          />
          {errors.oldPassword ? <p className="text-sm text-destructive">{errors.oldPassword}</p> : null}
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="cp-new">{t('modules.user.form.change_password.new_password.label')}</Label>
          <Input
            id="cp-new"
            type="password"
            placeholder={t('modules.user.form.change_password.new_password.placeholder')}
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
          />
          {errors.newPassword ? <p className="text-sm text-destructive">{errors.newPassword}</p> : null}
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="cp-retry">{t('modules.user.form.change_password.new_password_retry.label')}</Label>
          <Input
            id="cp-retry"
            type="password"
            placeholder={t('modules.user.form.change_password.new_password_retry.placeholder')}
            value={newPasswordRetry}
            onChange={(e) => setNewPasswordRetry(e.target.value)}
          />
          {errors.newPasswordRetry ? <p className="text-sm text-destructive">{errors.newPasswordRetry}</p> : null}
        </div>
        <Button type="submit" className="h-10 w-full font-normal lg:w-auto lg:min-w-44 lg:self-start" disabled={updatePassword.isPending}>
          {t('modules.user.form.change_password.submit.label')}
        </Button>
      </form>

      <LoadingDialog open={updatePassword.isPending} label={t('modules.user.modal.change_password_loading.label')} />

      <ResponsiveDialog
        open={outcome === 'success'}
        onOpenChange={(o) => !o && setOutcome(null)}
        title={t('modules.user.modal.change_password_success.text')}
      >
        <Button type="button" className="w-full h-11" onClick={() => setOutcome(null)}>
          {t('elements.button.close.label')}
        </Button>
      </ResponsiveDialog>

      <ResponsiveDialog
        open={outcome === 'error'}
        onOpenChange={(o) => !o && setOutcome(null)}
        title={t('modules.user.modal.change_password_error.header')}
        description={errorMessage}
      >
        <Button type="button" className="w-full h-11" onClick={() => setOutcome(null)}>
          {t('elements.button.close.label')}
        </Button>
      </ResponsiveDialog>
    </SettingsShell>
  )
}
