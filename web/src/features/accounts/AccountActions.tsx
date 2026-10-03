import type { SyntheticEvent } from 'react'
import { MoreVertical } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import type { AccountDto } from '@/api/dto/account'
import { hasAccountAdminAccess } from '@/features/connections/shared'
import { useUserData } from '@/features/user/queries'
import { useAccountActions } from './useAccountActions'

const stop = (e: SyntheticEvent) => e.stopPropagation()

export function AccountActionsMenu({
  account,
  onDone,
  open,
  onOpenChange,
}: {
  account: AccountDto
  onDone?: () => void
  /** a host row that opens the menu on its own click controls it */
  open?: boolean
  onOpenChange?: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const actions = useAccountActions(onDone)
  const isOwner = account.owner.id === user?.id
  return (
    <>
      <DropdownMenu open={open} onOpenChange={onOpenChange}>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={`account actions ${account.name}`}
            /* pointerdown passes on purpose: an enclosing Radix dialog tracks it at
               the document, and swallowing it costs that dialog its next outside click */
            onClick={stop}
            onKeyDown={stop}
          >
            <MoreVertical className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        {/* portaled content still bubbles React events to the host row — don't reselect it */}
        <DropdownMenuContent align="end" onClick={stop} onKeyDown={stop}>
          <DropdownMenuItem onSelect={() => actions.edit(account)}>{t('common.button.edit.label')}</DropdownMenuItem>
          {user && hasAccountAdminAccess(account, user.id) ? (
            <DropdownMenuItem onSelect={() => actions.access(account)}>{t('settings.accounts.list_actions.access')}</DropdownMenuItem>
          ) : null}
          <DropdownMenuItem variant="destructive" onSelect={() => actions.remove(account)}>
            {t(isOwner ? 'common.button.delete.label' : 'common.button.decline.label')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {/* the dialogs portal too; pointerdown must still reach the document so outside clicks dismiss them */}
      <span className="contents" onClick={stop} onKeyDown={stop}>
        {actions.dialogs}
      </span>
    </>
  )
}
