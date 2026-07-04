import { useEffect, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { FailDialog } from '@/components/FailDialog'
import * as config from '@/lib/config'
import { econumoPackage } from '@/lib/package'
import { getToken, isTokenExpired } from '@/lib/storage'
import { isNotEmpty, isValidEmail, isValidHttpUrl, isValidName, isValidPassword } from '@/lib/validation'
import { RouterPage } from '@/app/router-pages'
import { SelfHostedInfoDialog } from './SelfHostedInfoDialog'
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
  const [infoOpen, setInfoOpen] = useState(false)
  const customApiAllowed = config.isCustomApiAllowed()
  const pkg = econumoPackage()

  const { register, handleSubmit, control, watch, formState: { errors } } = useForm<RegistrationForm>({
    mode: 'onTouched',
    defaultValues: {
      name: '',
      email: '',
      password: '',
      passwordRetry: '',
      selfHosted: config.selfHosted(),
      host: config.backendHost() || '',
    },
  })
  const selfHostedChecked = watch('selfHosted')

  useEffect(() => {
    const token = getToken()
    if (token && !isTokenExpired(token)) {
      window.location.assign('/')
    }
  }, [])

  const onSubmit = handleSubmit(async ({ name, email, password, selfHosted, host }) => {
    if (customApiAllowed) {
      config.selfHosted(selfHosted)
      if (selfHosted && host) {
        config.backendHost(host)
      }
    }
    try {
      await registerMutation.mutateAsync({ email, password, name })
      navigate(RouterPage.LOGIN)
    } catch {
      setFailOpen(true)
    }
  })

  if (pkg.isPaywallEnabled) {
    return (
      <div className="flex flex-col items-center gap-4 text-center">
        <div dangerouslySetInnerHTML={{ __html: t('modules.user.page.sign_up.paywall.header') }} />
        <div dangerouslySetInnerHTML={{ __html: t('modules.user.page.sign_up.paywall.text') }} />
        <Button asChild size="lg">
          <a href={pkg.paywallUrl} target="_blank" rel="noopener noreferrer">
            {t('modules.user.page.sign_up.paywall.action')}
          </a>
        </Button>
        <p className="text-sm text-muted-foreground">{t('modules.user.page.sign_up.paywall.next_steps')}</p>
      </div>
    )
  }

  return (
    <div className="flex w-full flex-col gap-4">
      <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
        <div className="flex flex-col gap-2">
          <Label htmlFor="reg-name">{t('modules.user.form.user.name.label')}</Label>
          <Input
            id="reg-name"
            placeholder={t('modules.user.form.user.name.placeholder')}
            {...register('name', {
              validate: {
                required: (v) => isNotEmpty(v) || t('modules.user.form.user.name.validation.required_field'),
                name: (v) => isValidName(v) || t('modules.user.form.user.name.validation.invalid_name'),
              },
            })}
          />
          {errors.name ? <p className="text-sm text-destructive">{errors.name.message}</p> : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="reg-email">{t('modules.user.form.user.email.label')}</Label>
          <Input
            id="reg-email"
            type="email"
            placeholder={t('modules.user.form.user.email.placeholder')}
            {...register('email', {
              validate: {
                required: (v) => isNotEmpty(v) || t('modules.user.form.user.email.validation.required_field'),
                email: (v) => isValidEmail(v) || t('modules.user.form.user.email.validation.invalid_email'),
              },
            })}
          />
          {errors.email ? <p className="text-sm text-destructive">{errors.email.message}</p> : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="reg-password">{t('modules.user.form.user.password.label')}</Label>
          <Input
            id="reg-password"
            type="password"
            placeholder={t('modules.user.form.user.password.placeholder')}
            {...register('password', {
              validate: {
                required: (v) => isNotEmpty(v) || t('modules.user.form.user.password.validation.required_field'),
                password: (v) => isValidPassword(v) || t('modules.user.form.user.password.validation.invalid_password'),
              },
            })}
          />
          {errors.password ? <p className="text-sm text-destructive">{errors.password.message}</p> : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="reg-password-retry">{t('modules.user.form.user.password_retry.label')}</Label>
          <Input
            id="reg-password-retry"
            type="password"
            placeholder={t('modules.user.form.user.password_retry.placeholder')}
            {...register('passwordRetry', {
              validate: {
                required: (v) => isNotEmpty(v) || t('modules.user.form.user.password_retry.validation.invalid_password'),
                equals: (v, values) => v === values.password || t('modules.user.form.user.password_retry.validation.not_equals'),
              },
            })}
          />
          {errors.passwordRetry ? <p className="text-sm text-destructive">{errors.passwordRetry.message}</p> : null}
        </div>

        {customApiAllowed ? (
          <div className="flex flex-col gap-2">
            <div className="flex items-center gap-2">
              <Controller
                control={control}
                name="selfHosted"
                render={({ field }) => (
                  <Checkbox id="reg-self-hosted" checked={field.value} onCheckedChange={field.onChange} />
                )}
              />
              <Label htmlFor="reg-self-hosted">{t('modules.user.form.user.server_host.self_hosted')}</Label>
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
                <Label htmlFor="reg-host">{t('modules.user.form.user.server_host.label')}</Label>
                <Input
                  id="reg-host"
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

        <Button type="submit" className="w-full" disabled={registerMutation.isPending}>
          {t('modules.user.form.sign_up.action.sign_up')}
        </Button>

        <div className="text-xs text-muted-foreground" dangerouslySetInnerHTML={{ __html: t('modules.user.page.sign_up.privacy.text') }} />
      </form>

      <FailDialog
        open={failOpen}
        onClose={() => setFailOpen(false)}
        title={t('modules.user.modal.sign_up_failed.header')}
        description={t('modules.user.modal.sign_up_failed.information')}
      />
      <SelfHostedInfoDialog open={infoOpen} onClose={() => setInfoOpen(false)} />
    </div>
  )
}
