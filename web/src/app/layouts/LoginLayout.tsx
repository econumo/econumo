import { NavLink, Outlet } from 'react-router'
import { useTranslation } from 'react-i18next'
import { isRegistrationAllowed } from '@/lib/config'
import { useServerConfig } from '@/lib/appConfig'
import { RouterPage } from '@/app/router-pages'
import { AuthFrame } from './AuthFrame'

export function LoginLayout() {
  const { t } = useTranslation()
  // ALLOW_REGISTRATION may arrive after first paint in the app.
  useServerConfig((s) => s.revision)
  const registerEnabled = isRegistrationAllowed()
  // segmented control: the inactive tab sits on the gray track, so it reads as
  // "the other option" instead of blending into the form below
  const tabClass = ({ isActive }: { isActive: boolean }) =>
    `flex-1 rounded-md py-2 text-center text-sm font-medium ${isActive ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'}`
  return (
    <AuthFrame>
      <div className="mb-6 flex rounded-lg bg-econumo-card p-1">
        <NavLink to={RouterPage.LOGIN} className={tabClass}>
          {t('auth.page.sign_in.header')}
        </NavLink>
        {registerEnabled ? (
          <NavLink to={RouterPage.REGISTER} className={tabClass}>
            {t('auth.page.sign_up.header')}
          </NavLink>
        ) : (
          <span className="flex-1 rounded-md py-2 text-center text-sm font-medium text-muted-foreground/50">
            {t('auth.page.sign_up.header')}
          </span>
        )}
      </div>
      <Outlet />
    </AuthFrame>
  )
}
