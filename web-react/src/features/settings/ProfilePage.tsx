import { useEffect, useState } from 'react'
import { ChevronRight, Power } from 'lucide-react'
import { isAxiosError } from 'axios'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ConfirmDialog } from '@/components/ConfirmDialog'
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
    <SettingsShell
      title={t('modules.user.page.settings.profile.header')}
      backTo={RouterPage.SETTINGS}
      actions={
        <Button type="button" variant="ghost" size="icon" aria-label={t('pages.settings.settings.logout')} onClick={() => setLogoutOpen(true)}>
          <Power className="size-5" />
        </Button>
      }
    >
      {user ? (
        <div className="flex items-center gap-3 px-1 py-3">
          <img src={`${user.avatar}?s=100`} alt={user.name} className="size-16 rounded-full" />
          <span className="flex min-w-0 flex-col">
            <span className="truncate text-sm font-medium">{user.name}</span>
            <span className="truncate text-xs text-muted-foreground">{user.email}</span>
            <button type="button" className="mt-1 self-start text-xs text-muted-foreground underline" onClick={() => setLogoutOpen(true)}>
              {t('pages.settings.settings.logout')}
            </button>
          </span>
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
        className="flex items-center justify-between gap-2 rounded-md px-1 py-2.5 text-sm hover:bg-accent"
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
