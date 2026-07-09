import { NavLink, Outlet } from 'react-router'
import { useTranslation } from 'react-i18next'
import { isRegistrationAllowed } from '@/lib/config'
import { econumoPackage } from '@/lib/package'
import { RouterPage } from '@/app/router-pages'
import logo from '@/assets/econumo.svg'

export function LoginLayout() {
  const { t } = useTranslation()
  const registerEnabled = econumoPackage().isPaywallEnabled || isRegistrationAllowed()
  // segmented control: the inactive tab sits on the gray track, so it reads as
  // "the other option" instead of blending into the form below
  const tabClass = ({ isActive }: { isActive: boolean }) =>
    `flex-1 rounded-md py-2 text-center text-sm font-medium ${isActive ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'}`
  return (
    // the form lives on a white card over the brand gray so it reads as one
    // object instead of loose fields on a bare page
    <div className="flex min-h-svh flex-col items-center justify-center gap-6 bg-econumo-card p-4 pt-[max(env(safe-area-inset-top),1rem)] pb-[max(env(safe-area-inset-bottom),1rem)]">
      <img src={logo} width={194} height={20} alt={t('elements.econumo.label')} />
      <div className="w-full max-w-sm rounded-xl border bg-background p-6 shadow-sm">
        <div className="mb-6 flex rounded-lg bg-econumo-card p-1">
          <NavLink to={RouterPage.LOGIN} className={tabClass}>
            {t('modules.user.page.sign_in.header')}
          </NavLink>
          {registerEnabled ? (
            <NavLink to={RouterPage.REGISTER} className={tabClass}>
              {t('modules.user.page.sign_up.header')}
            </NavLink>
          ) : (
            <span className="flex-1 rounded-md py-2 text-center text-sm font-medium text-muted-foreground/50">
              {t('modules.user.page.sign_up.header')}
            </span>
          )}
        </div>
        <Outlet />
      </div>
      {/* one quiet credit line: the GitHub mark, then the site */}
      <div className="flex items-center gap-2.5 text-sm text-muted-foreground">
        <a
          target="_blank"
          rel="nofollow"
          href="https://github.com/econumo/"
          aria-label="GitHub"
          title="GitHub"
          className="transition-colors hover:text-foreground"
        >
          {/* octicon mark-github — lucide dropped brand icons */}
          <svg viewBox="0 0 16 16" className="size-4" fill="currentColor" aria-hidden="true">
            <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z" />
          </svg>
        </a>
        <a
          target="_blank"
          rel="nofollow"
          href={t('blocks.help.url')}
          title={t('blocks.help.label')}
          className="transition-colors hover:text-foreground"
        >
          {t('blocks.help.label')}
        </a>
      </div>
    </div>
  )
}
