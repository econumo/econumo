import { useEffect, useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { isAxiosError } from 'axios'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { UserCard } from '@/components/UserCard'
import { isNotEmpty, isValidName } from '@/lib/validation'
import { RouterPage } from '@/app/router-pages'
import { useUserData, useUpdateName } from '@/features/user/queries'
import { SettingsShell } from './SettingsShell'

export function ProfilePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data: user } = useUserData()
  const updateName = useUpdateName()

  const [name, setName] = useState('')
  const [nameError, setNameError] = useState<string | null>(null)
  const [logoutOpen, setLogoutOpen] = useState(false)

  useEffect(() => {
    if (user) {
      setName(user.name)
    }
  }, [user])

  const saveName = () => {
    if (!user || name === user.name) {
      return
    }
    if (!isNotEmpty(name)) {
      setNameError(t('modules.user.form.user.name.validation.required_field'))
      return
    }
    if (!isValidName(name)) {
      setNameError(t('modules.user.form.user.name.validation.invalid_name'))
      return
    }
    setNameError(null)
    updateName.mutate(name, {
      onError: (error) => {
        // surface the envelope's field errors (Vue silently swallows these)
        if (isAxiosError(error)) {
          const fieldErrors = (error.response?.data as { errors?: Record<string, string[]> } | undefined)?.errors?.name
          if (fieldErrors?.length) {
            setNameError(fieldErrors.join(' '))
            return
          }
        }
        setNameError(t('modules.user.form.user.name.validation.invalid_name'))
      },
    })
  }

  return (
    <SettingsShell title={t('modules.user.page.settings.profile.header')} backTo={RouterPage.SETTINGS}>
      {user ? (
        <div className="px-1 py-3">
          <UserCard user={user} size="lg">
            <button type="button" className="self-start text-sm text-econumo-magenta underline hover:text-econumo-magenta-dark" onClick={() => setLogoutOpen(true)}>
              {t('pages.settings.settings.logout')}
            </button>
          </UserCard>
        </div>
      ) : null}

      <form
        className="flex max-w-md flex-col gap-4 py-2"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          saveName()
        }}
      >
        <div className="flex flex-col gap-2">
          <Label htmlFor="profile-name">{t('modules.user.form.user.name.label')}</Label>
          <Input
            id="profile-name"
            placeholder={t('modules.user.form.user.name.placeholder')}
            value={name}
            onChange={(e) => setName(e.target.value)}
            onBlur={saveName}
          />
          {nameError ? <p className="text-sm text-destructive">{nameError}</p> : null}
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="profile-email">{t('modules.user.form.user.email.label')}</Label>
          <Input id="profile-email" type="email" disabled readOnly placeholder={t('modules.user.form.user.email.placeholder')} value={user?.email ?? ''} />
        </div>
      </form>

      <p className="px-1 pb-1 pt-4 text-xs font-medium uppercase text-muted-foreground">
        {t('modules.user.page.settings.profile.groups.security')}
      </p>
      <Link
        to={RouterPage.SETTINGS_CHANGE_PASSWORD}
        className="flex max-w-md items-center justify-between gap-2 rounded-lg bg-econumo-card px-4 py-3.5 text-sm hover:bg-econumo-hover"
      >
        {t('modules.user.page.settings.profile.change_password.menu_item')}
        <ChevronRight className="size-4 text-muted-foreground" />
      </Link>

      <ConfirmDialog
        open={logoutOpen}
        onClose={() => setLogoutOpen(false)}
        onConfirm={() => navigate(RouterPage.LOGOUT)}
        title={t('modules.user.modal.sign_out.title')}
        question={t('modules.user.modal.sign_out.question')}
        confirmLabel={t('modules.user.modal.sign_out.action.logout')}
        cancelLabel={t('modules.user.modal.sign_out.action.cancel')}
      />
    </SettingsShell>
  )
}
