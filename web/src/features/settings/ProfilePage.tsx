import { useEffect, useRef, useState } from 'react'
import { Check, ChevronRight, Lock } from 'lucide-react'
import { isAxiosError } from 'axios'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { CurrencyPickerDialog } from '@/components/CurrencyPickerDialog'
import { AvatarPickerDialog } from '@/components/AvatarPickerDialog'
import { UserCard } from '@/components/UserCard'
import { apiErrorMessage } from '@/lib/apiError'
import { isNotEmpty, isValidName } from '@/lib/validation'
import { RouterPage } from '@/app/router-pages'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData, useUpdateName, useUpdateCurrency, userCurrencyId } from '@/features/user/queries'
import { SettingsShell } from './SettingsShell'

export function ProfilePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data: user } = useUserData()
  const updateName = useUpdateName()

  const { data: currencies } = useCurrencies()
  const updateCurrency = useUpdateCurrency()
  const currentCurrencyId = userCurrencyId(user)

  const [name, setName] = useState('')
  const [nameError, setNameError] = useState<string | null>(null)
  const [logoutOpen, setLogoutOpen] = useState(false)
  const [currencyOpen, setCurrencyOpen] = useState(false)
  const [avatarOpen, setAvatarOpen] = useState(false)
  const [savedVisible, setSavedVisible] = useState(false)
  const savedTimer = useRef<number | null>(null)

  useEffect(() => {
    if (user) {
      setName(user.name)
    }
  }, [user])

  useEffect(() => () => {
    if (savedTimer.current !== null) {
      window.clearTimeout(savedTimer.current)
    }
  }, [])

  const flashSaved = () => {
    setSavedVisible(true)
    if (savedTimer.current !== null) {
      window.clearTimeout(savedTimer.current)
    }
    savedTimer.current = window.setTimeout(() => setSavedVisible(false), 2500)
  }

  const saveName = () => {
    if (!user || name === user.name) {
      return
    }
    if (!isNotEmpty(name)) {
      setNameError(t('user.form.name.validation.required_field'))
      return
    }
    if (!isValidName(name)) {
      setNameError(t('user.form.name.validation.invalid_name'))
      return
    }
    setNameError(null)
    updateName.mutate(name, {
      onSuccess: flashSaved,
      onError: (error) => {
        // surface the envelope's field errors (Vue silently swallows these)
        if (isAxiosError(error)) {
          const fieldErrors = (error.response?.data as { errors?: Record<string, string[]> } | undefined)?.errors?.name
          if (fieldErrors?.length) {
            setNameError(apiErrorMessage(error))
            return
          }
        }
        setNameError(t('user.form.name.validation.invalid_name'))
      },
    })
  }

  return (
    <SettingsShell title={t('user.page.settings.profile.header')} backTo={RouterPage.SETTINGS}>
      {user ? (
        <div className="px-1 py-3">
          <UserCard user={user} size="lg" onAvatarClick={() => setAvatarOpen(true)} avatarLabel={t('user.avatar_picker.change')}>
            <button type="button" className="self-start text-sm text-econumo-magenta underline hover:text-econumo-magenta-dark" onClick={() => setLogoutOpen(true)}>
              {t('settings.page.logout')}
            </button>
          </UserCard>
        </div>
      ) : null}

      <p className="px-1 pb-1 pt-2 text-xs font-medium uppercase text-muted-foreground">
        {t('user.page.settings.profile.groups.personal_details')}
      </p>
      <form
        className="flex max-w-md flex-col gap-4 py-2"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          saveName()
        }}
      >
        <CardField label={t('user.form.name.label')} htmlFor="profile-name" error={nameError}>
          <div className="relative">
            <Input
              id="profile-name"
              className={`${cardFieldControlClass} pr-9`}
              placeholder={t('user.form.name.placeholder')}
              value={name}
              onChange={(e) => {
                setName(e.target.value)
                setSavedVisible(false)
              }}
              onBlur={saveName}
            />
            {/* unobtrusive save confirmation: fades in on success, fades out on its own */}
            <span
              data-testid="name-saved"
              aria-hidden={!savedVisible}
              className={`pointer-events-none absolute inset-y-0 right-3 flex items-center text-income transition-opacity duration-500 ${
                savedVisible ? 'opacity-100' : 'opacity-0'
              }`}
            >
              <Check className="size-4" />
            </span>
          </div>
        </CardField>
        {/* read-only: dashed border instead of a fill, muted value, lock mark */}
        <div className="flex w-full items-center gap-3 rounded-lg border border-dashed px-4 py-2.5" title={t('user.form.email.label')}>
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <Label htmlFor="profile-email" className="text-[11px] font-normal text-muted-foreground">
              {t('user.form.email.label')}
            </Label>
            <Input
              id="profile-email"
              type="email"
              disabled
              readOnly
              className="h-auto rounded-none border-0 bg-transparent p-0 text-sm text-muted-foreground shadow-none disabled:bg-transparent disabled:opacity-100 dark:bg-transparent dark:disabled:bg-transparent"
              placeholder={t('user.form.email.placeholder')}
              value={user?.email ?? ''}
            />
          </div>
          <Lock className="size-4 shrink-0 text-muted-foreground/60" aria-hidden="true" />
        </div>
      </form>

      <p className="px-1 pb-1 pt-4 text-xs font-medium uppercase text-muted-foreground">
        {t('user.page.settings.profile.groups.preferences')}
      </p>
      <button
        type="button"
        className="flex w-full max-w-md items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-sm hover:bg-econumo-hover"
        onClick={() => setCurrencyOpen(true)}
      >
        {t('user.page.settings.profile.currency.label')}
        <span className="flex items-center gap-2 text-xs text-muted-foreground">
          {currencies?.find((c) => c.id === currentCurrencyId)?.code ?? ''}
          <ChevronRight className="size-4" />
        </span>
      </button>

      <p className="px-1 pb-1 pt-4 text-xs font-medium uppercase text-muted-foreground">
        {t('user.page.settings.profile.groups.security')}
      </p>
      <div className="flex max-w-md flex-col gap-2">
        <Link
          to={RouterPage.SETTINGS_CHANGE_PASSWORD}
          className="flex items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-sm hover:bg-econumo-hover"
        >
          {t('user.page.settings.profile.change_password.menu_item')}
          <ChevronRight className="size-4 text-muted-foreground" />
        </Link>
        <Link
          to={RouterPage.SETTINGS_SESSIONS}
          className="flex items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-sm hover:bg-econumo-hover"
        >
          {t('user.page.settings.profile.sessions.menu_item')}
          <ChevronRight className="size-4 text-muted-foreground" />
        </Link>
        <Link
          to={RouterPage.SETTINGS_TOKENS}
          className="flex items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-sm hover:bg-econumo-hover"
        >
          {t('user.page.settings.profile.tokens.menu_item')}
          <ChevronRight className="size-4 text-muted-foreground" />
        </Link>
      </div>

      <CurrencyPickerDialog
        open={currencyOpen}
        title={t('user.page.settings.profile.currency.label')}
        value={currentCurrencyId}
        onClose={() => setCurrencyOpen(false)}
        onPick={(id) => {
          const currency = currencies?.find((c) => c.id === id)
          if (currency) {
            updateCurrency.mutate({ currency: currency.code })
          }
        }}
      />

      <AvatarPickerDialog open={avatarOpen} onClose={() => setAvatarOpen(false)} />

      <ConfirmDialog
        open={logoutOpen}
        onClose={() => setLogoutOpen(false)}
        onConfirm={() => navigate(RouterPage.LOGOUT)}
        title={t('auth.sign_out.title')}
        question={t('auth.sign_out.question')}
        confirmLabel={t('auth.sign_out.action.logout')}
        cancelLabel={t('auth.sign_out.action.cancel')}
      />
    </SettingsShell>
  )
}
