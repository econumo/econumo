import { NavLink, Outlet } from 'react-router'
import { useTranslation } from 'react-i18next'
import { isRegistrationAllowed } from '@/lib/config'
import { econumoPackage } from '@/lib/package'
import { RouterPage } from '@/app/router-pages'
import logo from '@/assets/econumo.svg'

export function LoginLayout() {
  const { t } = useTranslation()
  const registerEnabled = econumoPackage().isPaywallEnabled || isRegistrationAllowed()
  const tabClass = ({ isActive }: { isActive: boolean }) =>
    `flex-1 border-b-2 pb-2 text-center text-sm font-medium ${isActive ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground'}`
  return (
    <div className="flex min-h-svh flex-col items-center justify-center gap-6 p-4">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex justify-center">
          <img src={logo} width={194} height={20} alt={t('elements.econumo.label')} />
        </div>
        <div className="mb-6 flex">
          <NavLink to={RouterPage.LOGIN} className={tabClass}>
            {t('modules.user.page.sign_in.header')}
          </NavLink>
          {registerEnabled ? (
            <NavLink to={RouterPage.REGISTER} className={tabClass}>
              {t('modules.user.page.sign_up.header')}
            </NavLink>
          ) : (
            <span className="flex-1 border-b-2 border-transparent pb-2 text-center text-sm font-medium text-muted-foreground/50">
              {t('modules.user.page.sign_up.header')}
            </span>
          )}
        </div>
        <Outlet />
      </div>
      <div className="flex gap-4 text-sm text-muted-foreground">
        <a target="_blank" rel="nofollow" href="https://github.com/econumo/" aria-label="GitHub">GitHub</a>
        <a target="_blank" rel="nofollow" href="https://x.com/econumo" aria-label="Twitter">X</a>
      </div>
      <a target="_blank" rel="nofollow" href={t('blocks.help.url')} className="text-sm text-muted-foreground">
        {t('blocks.help.label')}
      </a>
    </div>
  )
}
