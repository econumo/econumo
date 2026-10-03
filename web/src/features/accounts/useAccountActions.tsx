import { useState } from 'react'
import type { ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { useUiStore } from '@/app/uiStore'
import { queryKeys } from '@/app/queryKeys'
import type { AccountDto } from '@/api/dto/account'
import { AccessLevelDialog } from '@/features/connections/AccessLevelDialog'
import { ShareAccessDialog } from '@/features/connections/ShareAccessDialog'
import type { ShareEntry } from '@/features/connections/shared'
import { buildShareEntries } from '@/features/connections/shared'
import { useConnections } from '@/features/connections/queries'
import { useUserData } from '@/features/user/queries'
import { useAccounts, useDeleteAccount, useGrantAccountAccess, useLeaveSharedAccount, useRevokeAccountAccess } from './queries'

export interface AccountActions {
  edit: (account: AccountDto) => void
  access: (account: AccountDto) => void
  /** owner deletes, a shared member declines — both behind a confirm */
  remove: (account: AccountDto) => void
  pickLevel: (accountId: string, entry: ShareEntry) => void
  dialogs: ReactNode
}

export function useAccountActions(): AccountActions {
  const { t } = useTranslation()
  const { data: accounts = [] } = useAccounts()
  const { data: user } = useUserData()
  const { data: connections = [] } = useConnections()
  const queryClient = useQueryClient()
  const openAccountModal = useUiStore((s) => s.openAccountModal)
  const deleteAccount = useDeleteAccount()
  const declineAccountAccess = useLeaveSharedAccount()
  const grantAccountAccess = useGrantAccountAccess()
  const revokeAccountAccess = useRevokeAccountAccess()

  const [deleteTarget, setDeleteTarget] = useState<AccountDto | null>(null)
  const [declineTarget, setDeclineTarget] = useState<AccountDto | null>(null)
  const [accessAccountId, setAccessAccountId] = useState<string | null>(null)
  const [levelTarget, setLevelTarget] = useState<{ accountId: string; entry: ShareEntry } | null>(null)

  // read the live cache copy so optimistic grant/revoke updates show immediately
  const accessAccount = accessAccountId ? accounts.find((a) => a.id === accessAccountId) ?? null : null

  const dialogs = (
    <>
      <ShareAccessDialog
        open={accessAccount !== null && levelTarget === null}
        title={accessAccount?.name ?? ''}
        kind="accounts"
        entries={accessAccount && user ? buildShareEntries(connections, accessAccount.sharedAccess, user.id, accessAccount.owner.id) : []}
        onPick={(entry) => {
          if (entry.role !== 'owner' && accessAccountId) {
            setLevelTarget({ accountId: accessAccountId, entry })
          }
        }}
        onClose={() => setAccessAccountId(null)}
      />

      <AccessLevelDialog
        open={levelTarget !== null}
        kind="accounts"
        user={levelTarget?.entry.user ?? null}
        role={levelTarget?.entry.role ?? null}
        onSelect={(role) => {
          if (levelTarget) {
            grantAccountAccess.mutate({ accountId: levelTarget.accountId, userId: levelTarget.entry.user.id, role })
          }
          setLevelTarget(null)
        }}
        onRevoke={() => {
          if (levelTarget) {
            revokeAccountAccess.mutate({ accountId: levelTarget.accountId, userId: levelTarget.entry.user.id })
          }
          setLevelTarget(null)
        }}
        onClose={() => setLevelTarget(null)}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) {
            deleteAccount.mutate(deleteTarget.id, { onSettled: () => setDeleteTarget(null) })
          }
        }}
        question={t('settings.accounts.delete_account_modal.question', { account: deleteTarget?.name ?? '' })}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />

      <ConfirmDialog
        open={declineTarget !== null}
        onClose={() => setDeclineTarget(null)}
        onConfirm={() => {
          if (declineTarget) {
            declineAccountAccess.mutate(declineTarget.id, { onSettled: () => setDeclineTarget(null) })
          }
        }}
        title={t('settings.accounts.decline_access_modal.title')}
        question={t('settings.accounts.decline_access_modal.question', { account: declineTarget?.name ?? '' })}
        confirmLabel={t('common.button.decline.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </>
  )

  return {
    edit: (account) => openAccountModal({ account }),
    access: (account) => {
      // grant state changes on the partner's device (accept/decline) — refresh before showing it
      void queryClient.invalidateQueries({ queryKey: queryKeys.accounts })
      setAccessAccountId(account.id)
    },
    remove: (account) => {
      if (account.owner.id === user?.id) {
        setDeleteTarget(account)
      } else {
        setDeclineTarget(account)
      }
    },
    pickLevel: (accountId, entry) => setLevelTarget({ accountId, entry }),
    dialogs,
  }
}
