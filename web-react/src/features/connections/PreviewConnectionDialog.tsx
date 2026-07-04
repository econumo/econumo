import { useState } from 'react'
import { BookOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import type { ConnectionDto } from '@/api/dto/connection'
import type { AccountRole } from '@/api/dto/account'
import type { Id } from '@/api/types'
import { useAccounts, useDeleteAccount } from '@/features/accounts/queries'
import { useBudgets, useDeclineBudgetAccess, useGrantBudgetAccess, useRevokeBudgetAccess } from '@/features/budgets/queries'
import { useUserData } from '@/features/user/queries'
import { AccessLevelDialog } from './AccessLevelDialog'
import { DeclineAccessDialog } from './DeclineAccessDialog'
import { useRevokeAccountAccess, useSetAccountAccess } from './queries'
import { sharedAccountsFor, sharedBudgetsFor } from './shared'
import type { SharedItem } from './shared'

interface PreviewConnectionDialogProps {
  open: boolean
  connection: ConnectionDto | null
  onDelete: (userId: Id) => void
  onClose: () => void
}

type Managed = { kind: 'accounts' | 'budgets'; item: SharedItem }

export function PreviewConnectionDialog({ open, connection, onDelete, onClose }: PreviewConnectionDialogProps) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const { data: budgets = [] } = useBudgets()
  const setAccountAccess = useSetAccountAccess()
  const revokeAccountAccess = useRevokeAccountAccess()
  const grantBudgetAccess = useGrantBudgetAccess()
  const revokeBudgetAccess = useRevokeBudgetAccess()
  const declineBudgetAccess = useDeclineBudgetAccess()
  const deleteAccount = useDeleteAccount()

  const [level, setLevel] = useState<Managed | null>(null)
  const [decline, setDecline] = useState<Managed | null>(null)

  if (!connection) {
    return null
  }
  const other = connection.user
  const meId = user?.id ?? ''
  const sharedBudgets = sharedBudgetsFor(budgets, meId, other.id)
  const sharedAccounts = sharedAccountsFor(accounts, meId, other.id)

  const handleRowClick = (kind: 'accounts' | 'budgets', item: SharedItem) => {
    if (item.ownedByMe) {
      if (item.role !== 'owner') {
        setLevel({ kind, item })
      }
    } else {
      setDecline({ kind, item })
    }
  }

  const section = (kind: 'accounts' | 'budgets', items: SharedItem[]) => (
    <section className="flex flex-col gap-1">
      <p className="text-xs font-medium uppercase text-muted-foreground">
        {t(`modules.connections.modals.preview_connection.${kind}`)}
      </p>
      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t(`modules.connections.modals.preview_connection.${kind}_empty`)}</p>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">{t('modules.connections.modals.preview_connection.tap_to_manage')}</p>
          <ul className="flex flex-col">
            {items.map((item) => (
              <li key={item.id}>
                <button
                  type="button"
                  className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-left hover:bg-accent"
                  onClick={() => handleRowClick(kind, item)}
                >
                  {kind === 'budgets' ? (
                    <BookOpen className="size-4 text-muted-foreground" />
                  ) : (
                    <EntityIcon name={item.icon ?? 'wallet'} className="text-base text-muted-foreground" />
                  )}
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="truncate text-sm">{item.name}</span>
                    <span className="text-xs text-muted-foreground">
                      {item.ownedByMe
                        ? t(`modules.connections.modals.preview_connection.your_${kind === 'budgets' ? 'budget' : 'account'}`)
                        : t('modules.connections.modals.preview_connection.shared_with_you')}
                      {' · '}
                      {t(`modules.connections.${kind}.roles.${item.role}`)}
                    </span>
                  </span>
                  <img src={`${item.owner.avatar}?s=30`} alt={item.owner.name} className="size-5 rounded-full" />
                </button>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  )

  return (
    <>
      <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={other.name}>
        <div className="flex flex-col gap-4">
          {section('budgets', sharedBudgets)}
          {section('accounts', sharedAccounts)}
          <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
            <Button type="button" variant="destructive" onClick={() => onDelete(other.id)}>
              {t('elements.button.delete.label')}
            </Button>
            <Button type="button" variant="secondary" onClick={onClose}>
              {t('elements.button.ok.label')}
            </Button>
          </div>
        </div>
      </ResponsiveDialog>

      <AccessLevelDialog
        open={level !== null}
        kind={level?.kind ?? 'accounts'}
        user={other}
        role={level?.item.role ?? null}
        onSelect={(role) => {
          if (!level) return
          if (level.kind === 'accounts') {
            setAccountAccess.mutate({ accountId: level.item.id, userId: other.id, role: role as AccountRole })
          } else {
            grantBudgetAccess.mutate({ budgetId: level.item.id, userId: other.id, role })
          }
          setLevel(null)
        }}
        onRevoke={() => {
          if (!level) return
          if (level.kind === 'accounts') {
            revokeAccountAccess.mutate({ accountId: level.item.id, userId: other.id })
          } else {
            revokeBudgetAccess.mutate({ budgetId: level.item.id, userId: other.id })
          }
          setLevel(null)
        }}
        onClose={() => setLevel(null)}
      />

      <DeclineAccessDialog
        open={decline !== null}
        owner={decline?.item.owner ?? null}
        itemName={decline?.item.name ?? ''}
        onDecline={() => {
          if (!decline) return
          if (decline.kind === 'accounts') {
            // no decline-account endpoint: dropping a shared account = deleting it from my list (Vue parity)
            deleteAccount.mutate(decline.item.id)
          } else {
            declineBudgetAccess.mutate(decline.item.id)
          }
          setDecline(null)
        }}
        onClose={() => setDecline(null)}
      />
    </>
  )
}
