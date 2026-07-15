import { useState } from 'react'
import { useTranslation } from 'react-i18next'
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

  const [expandedId, setExpandedId] = useState<Id | null>(null)
  const [folderId, setFolderId] = useState<string>('')
  const [declineTarget, setDeclineTarget] = useState<PendingInvite | null>(null)

  const startAccept = (invite: PendingInvite) => {
    if (invite.kind === 'budget') {
      acceptBudget.mutate(invite.id)
      return
    }
    setExpandedId(invite.id)
    const last = folders && folders.length > 0 ? folders[folders.length - 1] : undefined
    setFolderId(last ? last.id : '')
  }

  const confirmAccept = (invite: PendingInvite) => {
    acceptAccount.mutate({ accountId: invite.id, folderId: folderId || undefined })
    setExpandedId(null)
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
      title={t('modules.connections.sharing_requests.title')}
    >
      <div className="flex flex-col gap-4">
        {invites.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('modules.connections.sharing_requests.empty')}</p>
        ) : (
          invites.map((invite) => {
            const isExpanded = invite.kind === 'account' && expandedId === invite.id
            return (
              <div key={`${invite.kind}-${invite.id}`} className="flex flex-col gap-3 rounded-lg bg-econumo-card p-3">
                <div className="flex items-center gap-3">
                  <UserAvatar avatar={invite.owner.avatar} size="md" />
                  <div className="flex min-w-0 flex-col">
                    <span className="truncate text-sm font-medium">
                      {t('modules.connections.sharing_requests.invited_you', { name: invite.owner.name })}
                    </span>
                    <span className="flex flex-wrap items-center gap-x-1 text-xs text-muted-foreground">
                      <span>{t(`modules.connections.sharing_requests.${invite.kind}`)}</span>
                      <span>·</span>
                      <span className="truncate">{invite.name}</span>
                      <span>·</span>
                      <span>{t(`modules.connections.${invite.kind}s.roles.${invite.role}`)}</span>
                    </span>
                  </div>
                </div>

                {isExpanded ? (
                  <div className="flex flex-col gap-2">
                    <Label htmlFor={`sharing-request-folder-${invite.id}`}>
                      {t('modules.connections.sharing_requests.choose_folder')}
                    </Label>
                    <Select value={folderId} onValueChange={setFolderId}>
                      <SelectTrigger id={`sharing-request-folder-${invite.id}`} className="w-full">
                        <SelectValue placeholder={t('modules.connections.sharing_requests.general_folder_hint')} />
                      </SelectTrigger>
                      <SelectContent>
                        {folders && folders.length > 0 ? (
                          folders.map((folder) => (
                            <SelectItem key={folder.id} value={folder.id}>
                              {folder.name}
                            </SelectItem>
                          ))
                        ) : (
                          <SelectItem value={NO_FOLDER_OPTION} disabled>
                            {t('modules.connections.sharing_requests.general_folder_hint')}
                          </SelectItem>
                        )}
                      </SelectContent>
                    </Select>
                    <div className={dialogActionsClass}>
                      <Button type="button" variant="secondary" onClick={() => setExpandedId(null)}>
                        {t('elements.button.cancel.label')}
                      </Button>
                      <Button type="button" onClick={() => confirmAccept(invite)}>
                        {t('elements.button.accept.label')}
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className={dialogActionsClass}>
                    <Button type="button" variant="destructive" onClick={() => setDeclineTarget(invite)}>
                      {t('elements.button.decline.label')}
                    </Button>
                    <Button type="button" onClick={() => startAccept(invite)}>
                      {t('elements.button.accept.label')}
                    </Button>
                  </div>
                )}
              </div>
            )
          })
        )}
      </div>

      <ConfirmDialog
        open={declineTarget !== null}
        onClose={() => setDeclineTarget(null)}
        onConfirm={confirmDecline}
        question={t('modules.connections.sharing_requests.decline_question', { name: declineTarget?.name ?? '' })}
        confirmLabel={t('elements.button.decline.label')}
        cancelLabel={t('elements.button.cancel.label')}
        destructive
      />
    </ResponsiveDialog>
  )
}
