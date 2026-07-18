import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { EyeOff } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { UserAvatar } from '@/components/UserAvatar'
import type { Id } from '@/api/types'
import { useAcceptAccountAccess, useDeclineAccountAccess, useFolders } from '@/features/accounts/queries'
import { useAcceptBudgetAccess, useDeclineBudgetAccess } from '@/features/budgets/queries'
import { usePendingInvites, type PendingInvite } from './pendingInvites'

const NO_FOLDER_OPTION = '__general__'

interface SharingRequestsDialogProps {
  open: boolean
  onClose: () => void
}

export function SharingRequestsDialog({ open, onClose }: SharingRequestsDialogProps) {
  const { t } = useTranslation()
  const { invites } = usePendingInvites()
  const { data: folders } = useFolders()
  const acceptAccount = useAcceptAccountAccess()
  const declineAccount = useDeclineAccountAccess()
  const acceptBudget = useAcceptBudgetAccess()
  const declineBudget = useDeclineBudgetAccess()

  const [folderChoices, setFolderChoices] = useState<Record<Id, string>>({})
  const [declineTarget, setDeclineTarget] = useState<PendingInvite | null>(null)

  const defaultFolderId = folders && folders.length > 0 ? folders[0].id : ''
  const chosenFolder = (invite: PendingInvite) => folderChoices[invite.id] ?? defaultFolderId

  const accept = (invite: PendingInvite) => {
    if (invite.kind === 'budget') {
      acceptBudget.mutate(invite.id)
      return
    }
    acceptAccount.mutate({ accountId: invite.id, folderId: chosenFolder(invite) || undefined })
  }

  const confirmDecline = () => {
    if (!declineTarget) return
    const target = declineTarget
    if (target.kind === 'account') {
      declineAccount.mutate(target.id, { onSettled: () => setDeclineTarget(null) })
    } else {
      declineBudget.mutate(target.id, { onSettled: () => setDeclineTarget(null) })
    }
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={t('connections.sharing_requests.title')}
    >
      <div className="flex flex-col gap-4">
        {invites.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('connections.sharing_requests.empty')}</p>
        ) : (
          invites.map((invite) => (
            <div key={`${invite.kind}-${invite.id}`} className="flex flex-col gap-3 rounded-lg bg-econumo-card p-3">
              <div className="flex items-center gap-3">
                <UserAvatar avatar={invite.owner.avatar} size="md" />
                <div className="flex min-w-0 flex-col">
                  <span className="truncate text-sm font-medium">
                    {t('connections.sharing_requests.invited_you', { name: invite.owner.name })}
                  </span>
                  <span className="flex flex-wrap items-center gap-x-1 text-xs text-muted-foreground">
                    <span>{t(`connections.sharing_requests.${invite.kind}`)}</span>
                    <span>·</span>
                    <span className="truncate">{invite.name}</span>
                    <span>·</span>
                    <span>{t(`connections.${invite.kind}s.roles.${invite.role}`)}</span>
                  </span>
                </div>
              </div>

              {invite.kind === 'account' ? (
                <div className="flex flex-col gap-2">
                  <Label htmlFor={`sharing-request-folder-${invite.id}`}>
                    {t('connections.sharing_requests.choose_folder')}
                  </Label>
                  <Select
                    value={chosenFolder(invite)}
                    onValueChange={(value) => setFolderChoices((prev) => ({ ...prev, [invite.id]: value }))}
                  >
                    <SelectTrigger id={`sharing-request-folder-${invite.id}`} className="w-full">
                      <SelectValue placeholder={t('connections.sharing_requests.general_folder_hint')} />
                    </SelectTrigger>
                    <SelectContent>
                      {folders && folders.length > 0 ? (
                        folders.map((folder) => (
                          <SelectItem key={folder.id} value={folder.id}>
                            <span className={`flex items-center gap-1.5 ${folder.isVisible === 0 ? 'text-muted-foreground' : ''}`}>
                              {folder.name}
                              {folder.isVisible === 0 ? <EyeOff className="size-3.5" aria-label="hidden" /> : null}
                            </span>
                          </SelectItem>
                        ))
                      ) : (
                        <SelectItem value={NO_FOLDER_OPTION} disabled>
                          {t('connections.sharing_requests.general_folder_hint')}
                        </SelectItem>
                      )}
                    </SelectContent>
                  </Select>
                </div>
              ) : null}

              <div className={dialogActionsClass}>
                <Button type="button" variant="destructive" onClick={() => setDeclineTarget(invite)}>
                  {t('common.button.decline.label')}
                </Button>
                <Button type="button" onClick={() => accept(invite)}>
                  {t('common.button.accept.label')}
                </Button>
              </div>
            </div>
          ))
        )}
      </div>

      <ConfirmDialog
        open={declineTarget !== null}
        onClose={() => setDeclineTarget(null)}
        onConfirm={confirmDecline}
        question={t('connections.sharing_requests.decline_question', { name: declineTarget?.name ?? '' })}
        confirmLabel={t('common.button.decline.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </ResponsiveDialog>
  )
}
