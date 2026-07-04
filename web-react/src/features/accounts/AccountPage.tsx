import { useState } from 'react'
import { ChevronLeft, MoreVertical, Plus, Settings2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { moneyFormat } from '@/lib/money'
import { useIsCompact } from '@/hooks/useIsCompact'
import { useUiStore } from '@/app/uiStore'
import { RouterPage } from '@/app/router-pages'
import { useAccounts } from './queries'
import { useUserData } from '@/features/user/queries'
import { useDeleteTransaction } from '@/features/transactions/queries'
import { separatorText, useAccountTransactions } from '@/features/transactions/useAccountTransactions'
import type { ViewTransaction } from '@/features/transactions/useAccountTransactions'
import { TransactionRow } from '@/features/transactions/TransactionRow'
import { ViewTransactionDialog } from '@/features/transactions/ViewTransactionDialog'

export function AccountPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isCompact = useIsCompact()
  const { id } = useParams()
  const { data: accounts } = useAccounts()
  const { data: user } = useUserData()
  const deleteTransaction = useDeleteTransaction()
  const openTransactionModal = useUiStore((s) => s.openTransactionModal)
  const openAccountModal = useUiStore((s) => s.openAccountModal)

  const [search, setSearch] = useState('')
  const [preview, setPreview] = useState<ViewTransaction | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ViewTransaction | null>(null)
  const entries = useAccountTransactions(id, search)

  const account = accounts?.find((a) => a.id === id)
  if (!account) {
    return null
  }

  const myRole = account.sharedAccess.find((access) => access.user.id === user?.id)?.role
  const isOwner = account.owner.id === user?.id
  const canUpdateSettings = isOwner || myRole === 'admin'
  const canChangeTransaction = isOwner || myRole === 'admin' || myRole === 'user'

  const canTouchRow = (tx: ViewTransaction): boolean => {
    if (!canChangeTransaction) {
      return false
    }
    if (tx.type === 'transfer') {
      return !!tx.account && !!tx.accountRecipient
    }
    return true
  }

  const editTransaction = (tx: ViewTransaction) => {
    setPreview(null)
    openTransactionModal({ transaction: tx })
  }

  const sharedAvatars =
    account.sharedAccess.length > 0 ? (
      <span className="flex items-center gap-0.5">
        <img src={`${account.owner.avatar}?s=30`} alt={account.owner.name} className="size-5 rounded-full" />
        {account.sharedAccess.map((access) => (
          <img key={access.user.id} src={`${access.user.avatar}?s=30`} alt={access.user.name} className="size-5 rounded-full" />
        ))}
      </span>
    ) : null

  return (
    <div className="flex h-full flex-col gap-3 p-4">
      {isCompact ? (
        <>
          <header className="flex items-center gap-3">
            <Button type="button" variant="ghost" size="icon" aria-label="back" onClick={() => navigate(RouterPage.HOME)}>
              <ChevronLeft className="size-5" />
            </Button>
            <EntityIcon name={account.icon} className="text-2xl text-muted-foreground" />
            <div className="min-w-0 flex-1">
              <h1 className="truncate text-lg uppercase" title={account.name}>
                {account.name}
              </h1>
              <p className="text-sm text-muted-foreground" data-testid="account-balance">
                {moneyFormat(account.balance, account.currency, { useNativePrecision: false })}
              </p>
            </div>
            {sharedAvatars}
            {canUpdateSettings ? (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={t('pages.account.toolbar.settings')}
                onClick={() => openAccountModal({ account })}
              >
                <Settings2 className="size-5" />
              </Button>
            ) : null}
            {canChangeTransaction ? (
              <Button
                type="button"
                size="icon"
                aria-label={t('pages.account.transaction_list.action.add_transaction')}
                onClick={() => openTransactionModal({ type: 'expense' })}
              >
                <Plus className="size-4" />
              </Button>
            ) : null}
          </header>
          <Input
            aria-label={t('pages.account.toolbar.search')}
            placeholder={t('pages.account.toolbar.search')}
            className="border-0 bg-econumo-card shadow-none"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </>
      ) : (
        <>
          {/* Vue workspace header: uppercase name + icon, bold balance, hairline */}
          <header className="flex flex-col gap-1">
            <div className="flex items-center gap-3">
              <h1 className="truncate text-[22px] uppercase tracking-wide" title={account.name}>
                {account.name}
              </h1>
              <span className="grid size-8 place-items-center rounded-lg bg-econumo-card">
                <EntityIcon name={account.icon} className="text-lg text-[#666666]" />
              </span>
              {sharedAvatars}
            </div>
            <div className="flex items-end justify-between gap-3 border-b pb-2">
              <p className="text-[15px] font-medium" data-testid="account-balance">
                {moneyFormat(account.balance, account.currency, { useNativePrecision: false })}
              </p>
              {canUpdateSettings ? (
                <button
                  type="button"
                  className="text-sm text-muted-foreground normal-case hover:text-foreground"
                  aria-label={t('pages.account.toolbar.settings')}
                  onClick={() => openAccountModal({ account })}
                >
                  {t('pages.account.toolbar.settings')}
                </button>
              ) : null}
            </div>
          </header>
          <div className="flex items-center justify-between gap-3">
            {canChangeTransaction ? (
              <Button
                type="button"
                aria-label={t('pages.account.transaction_list.action.add_transaction')}
                onClick={() => openTransactionModal({ type: 'expense' })}
              >
                {t('pages.account.transaction_list.action.add_transaction')}
              </Button>
            ) : (
              <span />
            )}
            <Input
              aria-label={t('pages.account.toolbar.search')}
              placeholder={t('pages.account.toolbar.search')}
              className="w-60 border-0 bg-econumo-card shadow-none"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
        </>
      )}

      <div className="flex-1 overflow-y-auto">
        {entries.map((entry) =>
          entry.kind === 'separator' ? (
            <div key={`sep-${entry.day}`} className="px-2 pb-1 pt-4 text-xs font-medium uppercase text-muted-foreground">
              {separatorText(entry, t)}
            </div>
          ) : (
            <div key={entry.transaction.id} className="group flex items-center">
              <div className="min-w-0 flex-1">
                <TransactionRow
                  transaction={entry.transaction}
                  pageAccount={account}
                  onClick={() => (isCompact ? setPreview(entry.transaction) : undefined)}
                />
              </div>
              {!isCompact && canTouchRow(entry.transaction) ? (
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button type="button" variant="ghost" size="icon" aria-label={`actions ${entry.transaction.id}`}>
                      <MoreVertical className="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem onSelect={() => editTransaction(entry.transaction)}>
                      {t('elements.button.edit.label')}
                    </DropdownMenuItem>
                    <DropdownMenuItem variant="destructive" onSelect={() => setDeleteTarget(entry.transaction)}>
                      {t('elements.button.delete.label')}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              ) : null}
            </div>
          ),
        )}
      </div>

      {preview ? (
        <ViewTransactionDialog
          transaction={preview}
          onClose={() => setPreview(null)}
          onEdit={() => editTransaction(preview)}
          onDelete={() => {
            setDeleteTarget(preview)
            setPreview(null)
          }}
          canChange={canTouchRow(preview)}
        />
      ) : null}

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) {
            deleteTransaction.mutate(deleteTarget.id, { onSettled: () => setDeleteTarget(null) })
          }
        }}
        question={t('pages.account.delete_transaction_modal.question')}
        confirmLabel={t('elements.button.delete.label')}
        cancelLabel={t('elements.button.cancel.label')}
      />
    </div>
  )
}
