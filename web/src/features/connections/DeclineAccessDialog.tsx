import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { UserAvatar } from '@/components/UserAvatar'
import type { UserDto } from '@/api/dto/user'

interface DeclineAccessDialogProps {
  open: boolean
  owner: UserDto | null
  itemName: string
  onDecline: () => void
  onClose: () => void
}

export function DeclineAccessDialog({ open, owner, itemName, onDecline, onClose }: DeclineAccessDialogProps) {
  const { t } = useTranslation()
  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={owner?.name ?? ''}>
      <div className="flex flex-col gap-3">
        {owner ? (
          <div className="flex items-center gap-3">
            <UserAvatar avatar={owner.avatar} size="md" />
            <span className="flex min-w-0 flex-col">
              <span className="truncate text-sm font-medium">{owner.name}</span>
              <span className="truncate text-xs text-muted-foreground">{itemName}</span>
            </span>
          </div>
        ) : null}
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('elements.button.cancel.label')}
          </Button>
          <Button type="button" variant="destructive" onClick={onDecline}>
            {t('modules.connections.modals.decline_access.decline_access')}
          </Button>
        </div>
      </div>
    </ResponsiveDialog>
  )
}
