import { useEffect, useState, type ChangeEvent } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { FailDialog } from '@/components/FailDialog'
import { PasswordInput } from '@/components/PasswordInput'
import * as config from '@/lib/config'
import { getToken } from '@/lib/storage'
import { isNotEmpty, isValidEmail, isValidHttpUrl, isValidName, isValidPassword } from '@/lib/validation'
import { RouterPage } from '@/app/router-pages'
import { CustomServerSection } from './CustomServerSection'
import { useRegister } from './queries'

interface RegistrationForm {
  name: string
  email: string
  password: string
  passwordRetry: string
  selfHosted: boolean
  host: string
}

export function RegistrationPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const registerMutation = useRegister()
  const [failOpen, setFailOpen] = useState(false)
  const customApiAllowed = config.isCustomApiAllowed()

  const { register, handleSubmit, setValue, getValues, watch, formState: { errors } } = useForm<RegistrationForm>({
    mode: 'onTouched',
    defaultValues: {
      name: '',
      email: '',
      password: '',
      passwordRetry: '',
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

  const onSubmit = handleSubmit(async ({ name, email, password }) => {
    try {
      await registerMutation.mutateAsync({ email, password, name })
      navigate(RouterPage.LOGIN)
    } catch {
      setFailOpen(true)
    }
  })

  return (
    <>
      <div className="flex w-full flex-col gap-4">
        <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
          <div className="flex flex-col gap-2">
            <Label htmlFor="reg-name">{t('user.form.name.label')}</Label>
            <Input
              className="h-11"
              id="reg-name"
              placeholder={t('user.form.name.placeholder')}
              {...register('name', {
                validate: {
                  required: (v) => isNotEmpty(v) || t('user.form.name.validation.required_field'),
                  name: (v) => isValidName(v) || t('user.form.name.validation.invalid_name'),
                },
              })}
            />
            {errors.name ? <p className="text-sm text-destructive">{errors.name.message}</p> : null}
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="reg-email">{t('user.form.email.label')}</Label>
            <Input
              className="h-11"
              id="reg-email"
              type="email"
              placeholder={t('user.form.email.placeholder')}
              {...register('email', {
                validate: {
                  required: (v) => isNotEmpty(v) || t('user.form.email.validation.required_field'),
                  email: (v) => isValidEmail(v) || t('user.form.email.validation.invalid_email'),
                },
              })}
            />
            {errors.email ? <p className="text-sm text-destructive">{errors.email.message}</p> : null}
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="reg-password">{t('user.form.password.label')}</Label>
            <PasswordInput
              className="h-11"
              id="reg-password"
              placeholder={t('user.form.password.placeholder')}
              {...register('password', {
                validate: {
                  required: (v) => isNotEmpty(v) || t('user.form.password.validation.required_field'),
                  password: (v) => isValidPassword(v) || t('user.form.password.validation.invalid_password'),
                },
              })}
            />
            {errors.password ? <p className="text-sm text-destructive">{errors.password.message}</p> : null}
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="reg-password-retry">{t('user.form.password_retry.label')}</Label>
            <PasswordInput
              className="h-11"
              id="reg-password-retry"
              placeholder={t('user.form.password_retry.placeholder')}
              {...register('passwordRetry', {
                validate: {
                  required: (v) => isNotEmpty(v) || t('user.form.password_retry.validation.invalid_password'),
                  equals: (v, values) => v === values.password || t('user.form.password_retry.validation.not_equals'),
                },
              })}
            />
            {errors.passwordRetry ? <p className="text-sm text-destructive">{errors.passwordRetry.message}</p> : null}
          </div>

          <Button type="submit" className="w-full bg-econumo-yellow text-econumo-yellow-text hover:bg-econumo-yellow/85 h-11" disabled={registerMutation.isPending}>
            {t('auth.form.sign_up.action.sign_up')}
          </Button>

          {customApiAllowed ? (
            <CustomServerSection open={selfHostedChecked} onToggle={toggleCustomServer}>
              <Label htmlFor="reg-host">{t('user.form.server_host.label')}</Label>
              <Input
                className="h-11"
                id="reg-host"
                type="url"
                placeholder={t('user.form.server_host.placeholder')}
                {...register('host', {
                  validate: {
                    required: (v) => isNotEmpty(v) || t('user.form.server_host.validation.required_field'),
                    url: (v) => isValidHttpUrl(v) || t('user.form.server_host.validation.invalid_url'),
                  },
                  onChange: (e: ChangeEvent<HTMLInputElement>) => config.backendHost(e.target.value),
                })}
              />
              {errors.host ? <p className="text-sm text-destructive">{errors.host.message}</p> : null}
            </CustomServerSection>
          ) : null}

          <div className="text-xs text-muted-foreground" dangerouslySetInnerHTML={{ __html: t('auth.page.sign_up.privacy.text') }} />
        </form>

        <FailDialog
          open={failOpen}
          onClose={() => setFailOpen(false)}
          title={t('auth.sign_up_failed.header')}
          description={t('auth.sign_up_failed.information')}
        />
      </div>
    </>
  )
}
