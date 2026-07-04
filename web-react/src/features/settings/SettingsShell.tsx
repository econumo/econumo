import type { ReactNode } from 'react'
import { ChevronLeft } from 'lucide-react'
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
  /** where the mobile back button goes */
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

  return (
    <div className="flex h-full flex-col gap-3 p-4">
      {isCompact ? (
        <header className="flex items-center gap-2">
          <Button type="button" variant="ghost" size="icon" aria-label="back" onClick={() => navigate(backTo)}>
            <ChevronLeft className="size-5" />
          </Button>
          <h1 className="flex-1 truncate text-lg font-semibold">{title}</h1>
          {actions}
        </header>
      ) : (
        <header className="flex flex-col gap-1">
          <nav className="flex gap-1 text-xs text-muted-foreground">
            {(crumbs ?? [{ label: t('pages.settings.settings.header_desktop'), to: '/settings' }]).map((crumb) => (
              <Link key={crumb.to} to={crumb.to} className="hover:text-foreground">
                {crumb.label} /
              </Link>
            ))}
          </nav>
          <div className="flex items-center gap-3">
            <h1 className="flex-1 truncate text-xl font-semibold">{heading ?? title}</h1>
            {actions}
          </div>
        </header>
      )}
      <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
    </div>
  )
}
