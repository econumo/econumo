import { useEffect, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { FailDialog } from '@/components/FailDialog'
import { PasswordInput } from '@/components/PasswordInput'
import * as config from '@/lib/config'
import { getToken } from '@/lib/storage'
import { isNotEmpty, isValidEmail, isValidHttpUrl } from '@/lib/validation'
import { RecoveryDialog } from './RecoveryDialog'
import { SelfHostedInfoDialog } from './SelfHostedInfoDialog'
import { useLogin } from './queries'

interface LoginForm {
  username: string
  password: string
  selfHosted: boolean
  host: string
}

export function LoginPage() {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const login = useLogin()
  const [failOpen, setFailOpen] = useState(false)
  const [recoveryOpen, setRecoveryOpen] = useState(false)
  const [infoOpen, setInfoOpen] = useState(false)
  const sessionExpired = searchParams.get('reason') === 'expired'
  const customApiAllowed = config.isCustomApiAllowed()

  const { register, handleSubmit, control, watch, formState: { errors } } = useForm<LoginForm>({
    mode: 'onTouched',
    defaultValues: {
      username: '',
      password: '',
      selfHosted: config.selfHosted(),
      host: config.backendHost() || '',
    },
  })
  const selfHostedChecked = watch('selfHosted')

  useEffect(() => {
    if (getToken()) {
      window.location.assign('/')
    }
  }, [])

  const onSubmit = handleSubmit(async ({ username, password, selfHosted, host }) => {
    if (customApiAllowed) {
      config.selfHosted(selfHosted)
      if (selfHosted && host) {
        config.backendHost(host)
      }
    }
    try {
      const result = await login.mutateAsync({ username, password })
      if (!result.token) {
        setFailOpen(true)
        return
      }
      window.location.assign('/')
    } catch {
      setFailOpen(true)
    }
  })

  return (
    <div className="flex w-full flex-col gap-4">
      {sessionExpired ? (
        <Alert variant="destructive">
          <AlertDescription>{t('modules.user.page.sign_in.session_expired')}</AlertDescription>
        </Alert>
      ) : null}

      <form onSubmit={onSubmit} className="flex flex-col gap-4" aria-label="Login form" noValidate>
        <div className="flex flex-col gap-2">
          <Label htmlFor="login-email">{t('modules.user.form.user.email.label')}</Label>
          <Input
            className="h-11"
            id="login-email"
            type="email"
            placeholder={t('modules.user.form.user.email.placeholder')}
            aria-required="true"
            {...register('username', {
              validate: {
                required: (v) => isNotEmpty(v) || t('modules.user.form.user.email.validation.required_field'),
                email: (v) => isValidEmail(v) || t('modules.user.form.user.email.validation.invalid_email'),
              },
            })}
          />
          {errors.username ? <p className="text-sm text-destructive">{errors.username.message}</p> : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="login-password">{t('modules.user.form.user.password.label')}</Label>
          <PasswordInput
            className="h-11"
            id="login-password"
            placeholder={t('modules.user.form.user.password.placeholder')}
            aria-required="true"
            {...register('password', {
              validate: {
                required: (v) => isNotEmpty(v) || t('modules.user.form.user.password.validation.required_field'),
              },
            })}
          />
          {errors.password ? <p className="text-sm text-destructive">{errors.password.message}</p> : null}
        </div>

        {customApiAllowed ? (
          <div className="flex flex-col gap-2">
            <div className="flex items-center gap-2">
              <Controller
                control={control}
                name="selfHosted"
                render={({ field }) => (
                  <Checkbox id="login-self-hosted" checked={field.value} onCheckedChange={field.onChange} />
                )}
              />
              <Label htmlFor="login-self-hosted">{t('modules.user.form.user.server_host.self_hosted')}</Label>
              <button
                type="button"
                className="text-sm text-muted-foreground underline"
                onClick={() => setInfoOpen(true)}
                aria-label={t('modules.app.modal.self_hosted.information')}
              >
                ?
              </button>
            </div>
            {selfHostedChecked ? (
              <div className="flex flex-col gap-2">
                <Label htmlFor="login-host">{t('modules.user.form.user.server_host.label')}</Label>
                <Input
                  className="h-11"
                  id="login-host"
                  type="url"
                  placeholder={t('modules.user.form.user.server_host.placeholder')}
                  {...register('host', {
                    validate: {
                      required: (v) => isNotEmpty(v) || t('modules.user.form.user.server_host.validation.required_field'),
                      url: (v) => isValidHttpUrl(v) || t('modules.user.form.user.server_host.validation.invalid_url'),
                    },
                  })}
                />
                {errors.host ? <p className="text-sm text-destructive">{errors.host.message}</p> : null}
              </div>
            ) : null}
          </div>
        ) : null}

        <Button type="submit" className="w-full bg-econumo-yellow text-econumo-yellow-text hover:bg-econumo-yellow/85 h-11" disabled={login.isPending}>
          {t('modules.user.form.sign_in.action.sign_in')}
        </Button>
        <Button type="button" variant="secondary" className="w-full h-11" onClick={() => setRecoveryOpen(true)}>
          {t('modules.user.form.sign_in.action.forget_password')}
        </Button>
      </form>

      <FailDialog
        open={failOpen}
        onClose={() => setFailOpen(false)}
        title={t('modules.user.modal.sign_in_failed.header')}
        description={t('modules.user.modal.sign_in_failed.information')}
      />
      <SelfHostedInfoDialog open={infoOpen} onClose={() => setInfoOpen(false)} />
      {recoveryOpen ? <RecoveryDialog open onClose={() => setRecoveryOpen(false)} /> : null}
    </div>
  )
}
