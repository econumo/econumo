import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import type { UserDto } from '@/api/dto/user'

export type GrantableRole = 'guest' | 'user' | 'admin'

interface AccessLevelDialogProps {
  open: boolean
  kind: 'accounts' | 'budgets'
  user: UserDto | null
  role: string | null
  onSelect: (role: GrantableRole) => void
  onRevoke: () => void
  onClose: () => void
}

// Vue offers guest -> user -> admin, and a revoke row only for an existing non-owner grant
const OPTIONS: GrantableRole[] = ['guest', 'user', 'admin']

export function AccessLevelDialog({ open, kind, user, role, onSelect, onRevoke, onClose }: AccessLevelDialogProps) {
  const { t } = useTranslation()
  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={user?.name ?? ''}>
      <div className="flex flex-col gap-2">
        {user ? (
          <div className="flex items-center gap-3">
            <img src={`${user.avatar}?s=50`} alt={user.name} className="size-10 rounded-full" />
            <p className="text-sm text-muted-foreground">{t('modules.connections.modals.share_access.choose_access_level')}</p>
          </div>
        ) : null}
        <div className="flex flex-col">
          {OPTIONS.map((option) => (
            <button
              key={option}
              type="button"
              aria-pressed={role === option}
              className={`rounded-md px-3 py-2.5 text-left text-sm hover:bg-accent ${role === option ? 'bg-accent font-medium' : ''}`}
              onClick={() => onSelect(option)}
            >
              {t(`modules.connections.${kind}.roles.${option}`)}
            </button>
          ))}
          {role && role !== 'owner' ? (
            <button
              type="button"
              className="rounded-md px-3 py-2.5 text-left text-sm text-destructive hover:bg-accent"
              onClick={onRevoke}
            >
              {t('modules.connections.modals.share_access.revoke_access')}
            </button>
          ) : null}
        </div>
        <Button type="button" variant="secondary" className="w-full" onClick={onClose}>
          {t('elements.button.cancel.label')}
        </Button>
      </div>
    </ResponsiveDialog>
  )
}
