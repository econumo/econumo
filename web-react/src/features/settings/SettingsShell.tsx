import type { ReactNode } from 'react'
import { ChevronLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useLocation, useNavigate } from 'react-router'
import { Button } from '@/components/ui/button'
import { useIsCompact } from '@/hooks/useIsCompact'

interface Crumb {
  label: string
  to: string
}

interface SettingsShellProps {
  /** mobile toolbar title */
  title: string
  /** desktop heading (defaults to title) */
  heading?: string
  /** mobile back-button fallback for deep links; with in-app history it goes back instead */
  backTo: string
  /** desktop breadcrumbs shown above the heading */
  crumbs?: Crumb[]
  /** extra toolbar actions (both modes) */
  actions?: ReactNode
  children: ReactNode
}

export function SettingsShell({ title, heading, backTo, crumbs, actions, children }: SettingsShellProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const isCompact = useIsCompact()

  // Back returns to wherever the user came from; only a deep link (the initial
  // document load keeps location.key === 'default') falls back to backTo.
  const goBack = () => {
    if (location.key === 'default') {
      void navigate(backTo)
    } else {
      void navigate(-1)
    }
  }

  return (
    <div className="flex h-full flex-col gap-3 p-4">
      {isCompact ? (
        <header className="flex items-center gap-2">
          <Button type="button" variant="ghost" size="icon" aria-label="back" title={t('elements.button.back.label')} onClick={goBack}>
            <ChevronLeft className="size-5" />
          </Button>
          <h1 className="flex-1 truncate text-center text-lg">{title}</h1>
          <div className="flex min-w-9 justify-end">{actions}</div>
        </header>
      ) : (
        // Vue anatomy: breadcrumb chip, large light title, wide action button UNDER the title
        <header className="flex flex-col gap-1">
          <nav className="flex gap-1">
            {(crumbs ?? [{ label: t('pages.settings.settings.header_desktop'), to: '/settings' }]).map((crumb) => (
              <Link
                key={crumb.to}
                to={crumb.to}
                className="rounded bg-econumo-card px-2 py-0.5 text-xs text-muted-foreground hover:bg-econumo-hover"
              >
                {crumb.label}
              </Link>
            ))}
          </nav>
          <h1 className="truncate text-[26px] font-normal leading-snug">{heading ?? title}</h1>
          {actions ? <div className="mt-2 flex items-center gap-2 [&_button]:min-w-44">{actions}</div> : null}
        </header>
      )}
      <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
    </div>
  )
}
