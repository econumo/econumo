import { useEffect, useState, type ChangeEvent } from 'react'
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
import { CustomServerSection } from './CustomServerSection'
import { RecoveryDialog } from './RecoveryDialog'
import { useLogin } from './queries'

interface LoginForm {
  username: string
  password: string
  rememberEmail: boolean
  selfHosted: boolean
  host: string
}

export function LoginPage() {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const login = useLogin()
  const [failOpen, setFailOpen] = useState(false)
  const [recoveryOpen, setRecoveryOpen] = useState(false)
  const sessionExpired = searchParams.get('reason') === 'expired'
  const customApiAllowed = config.isCustomApiAllowed()

  const { register, handleSubmit, setValue, getValues, watch, control, formState: { errors } } = useForm<LoginForm>({
    mode: 'onTouched',
    defaultValues: {
      username: config.rememberedEmail(),
      password: '',
      rememberEmail: config.rememberedEmail() !== '',
      selfHosted: config.selfHosted(),
      host: config.selfHosted() ? config.backendHost() : '',
    },
  })
  const selfHostedChecked = watch('selfHosted')

  // The disclosure state persists immediately (not on submit), and collapsing
  // forgets the previously configured server address.
  const toggleCustomServer = () => {
    const next = !selfHostedChecked
    config.selfHosted(next)
    if (next) {
      if (!getValues('host')) {
        setValue('host', window.location.origin)
      }
    } else {
      config.clearBackendHost()
      setValue('host', '')
    }
    setValue('selfHosted', next)
  }

  useEffect(() => {
    if (getToken()) {
      window.location.assign('/')
    }
  }, [])

  const onSubmit = handleSubmit(async ({ username, password }) => {
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
              onChange: (e: ChangeEvent<HTMLInputElement>) => {
                if (getValues('rememberEmail')) {
                  config.rememberedEmail(e.target.value)
                }
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

        <div className="flex items-center gap-2">
          <Controller
            control={control}
            name="rememberEmail"
            render={({ field }) => (
              <Checkbox
                id="login-remember-email"
                checked={field.value}
                onCheckedChange={(checked) => {
                  field.onChange(checked)
                  if (checked) {
                    config.rememberedEmail(getValues('username'))
                  } else {
                    config.clearRememberedEmail()
                  }
                }}
              />
            )}
          />
          <Label htmlFor="login-remember-email">{t('modules.user.form.sign_in.remember_me')}</Label>
        </div>

        <Button type="submit" className="w-full bg-econumo-yellow text-econumo-yellow-text hover:bg-econumo-yellow/85 h-11" disabled={login.isPending}>
          {t('modules.user.form.sign_in.action.sign_in')}
        </Button>
        <Button type="button" variant="secondary" className="w-full h-11" onClick={() => setRecoveryOpen(true)}>
          {t('modules.user.form.sign_in.action.forget_password')}
        </Button>

        {customApiAllowed ? (
          <CustomServerSection open={selfHostedChecked} onToggle={toggleCustomServer}>
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
                onChange: (e: ChangeEvent<HTMLInputElement>) => config.backendHost(e.target.value),
              })}
            />
            {errors.host ? <p className="text-sm text-destructive">{errors.host.message}</p> : null}
          </CustomServerSection>
        ) : null}
      </form>

      <FailDialog
        open={failOpen}
        onClose={() => setFailOpen(false)}
        title={t('modules.user.modal.sign_in_failed.header')}
        description={t('modules.user.modal.sign_in_failed.information')}
      />
      {recoveryOpen ? <RecoveryDialog open onClose={() => setRecoveryOpen(false)} /> : null}
    </div>
  )
}
