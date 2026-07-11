import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, Server } from 'lucide-react'
import { cn } from '@/lib/utils'

// Collapsible advanced option shared by the login and registration pages.
// The open state IS the form's selfHosted value, so the host input mounts
// (and its validation registers) only while the section is expanded.
export function CustomServerSection({
  open,
  onToggle,
  children,
}: {
  open: boolean
  onToggle: () => void
  children: ReactNode
}) {
  const { t } = useTranslation()
  const label = t('modules.user.form.user.server_host.connect')

  return (
    <div className="flex flex-col gap-2">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        title={label}
        className="flex items-center gap-1.5 self-start text-sm text-muted-foreground hover:text-foreground"
      >
        <Server className="size-4" />
        {label}
        <ChevronDown className={cn('size-4 transition-transform', open && 'rotate-180')} />
      </button>
      {open ? (
        <div className="flex flex-col gap-2 border-l-2 border-border pl-3">
          {children}
          <p className="text-xs text-muted-foreground">{t('modules.app.modal.self_hosted.information')}</p>
        </div>
      ) : null}
    </div>
  )
}
