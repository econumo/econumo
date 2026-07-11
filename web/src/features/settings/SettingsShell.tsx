import { Fragment } from 'react'
import type { ReactNode } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate } from 'react-router'
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
  /** where the mobile back button navigates (hierarchical "up", not history back) */
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
  const isCompact = useIsCompact()

  // Deterministic "up" navigation: on compact viewports this chevron is the
  // ONLY exit (no sidebar), so it must not depend on the history stack —
  // history-back can trap the user when the stack is odd (restored tab,
  // login/logout redirect chains) with no way back to the account list.
  const goBack = () => {
    void navigate(backTo)
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
          <nav className="flex items-center gap-1">
            {(crumbs ?? [{ label: t('pages.settings.settings.header_desktop'), to: '/settings' }]).map((crumb, i) => (
              <Fragment key={crumb.to}>
                {i > 0 ? <ChevronRight className="size-3 text-muted-foreground" aria-hidden="true" /> : null}
                <Link
                  to={crumb.to}
                  className="rounded bg-econumo-card px-2 py-0.5 text-xs text-muted-foreground hover:bg-econumo-hover"
                >
                  {crumb.label}
                </Link>
              </Fragment>
            ))}
          </nav>
          <h1 className="truncate text-[26px] font-normal leading-snug">{heading ?? title}</h1>
          {actions ? (
            <div className="mt-2 flex items-center gap-2 lg:max-w-xl [&_button:not([data-slot=switch])]:min-w-44">{actions}</div>
          ) : null}
        </header>
      )}
      {/* -mx/px pair: 4px of in-scroller breathing room so focus rings at the
          content edge aren't clipped (overflow-y:auto also clips the x axis).
          On desktop the content column is capped like the settings hub, so
          row actions don't drift to the far right of the screen. */}
      {/* overflow-x-hidden: invisible tap-target halos (e.g. the Switch ::after)
          must not turn into a few px of horizontal scroll on mobile. */}
      <div className="-mx-1 min-h-0 flex-1 overflow-x-hidden overflow-y-auto px-1">
        <div className="lg:max-w-xl">{children}</div>
      </div>
    </div>
  )
}
