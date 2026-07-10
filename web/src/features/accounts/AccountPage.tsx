import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { ChevronLeft, MoreVertical, Plus, Settings2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { UserAvatar } from '@/components/UserAvatar'
import { moneyFormat } from '@/lib/money'
import { useIsCompact } from '@/hooks/useIsCompact'
import { useUiStore } from '@/app/uiStore'
import { RouterPage } from '@/app/router-pages'
import { useAccounts } from './queries'
import { useUserData } from '@/features/user/queries'
import { useDeleteTransaction } from '@/features/transactions/queries'
import { separatorText, useAccountTransactions } from '@/features/transactions/useAccountTransactions'
import type { ViewTransaction } from '@/features/transactions/useAccountTransactions'
import type { DailyListEntry } from '@/features/transactions/useAccountTransactions'
import { TransactionRow } from '@/features/transactions/TransactionRow'
import { ViewTransactionDialog } from '@/features/transactions/ViewTransactionDialog'

// Accounts hold thousands of transactions; mounting them all at once makes
// switching accounts visibly slow. Render a chunk and grow it as the scroll
// sentinel comes into range — search still matches the full dataset because
// filtering happens on the data, not the DOM.
const LIST_CHUNK = 100

function WindowedEntries({ entries, children }: { entries: DailyListEntry[]; children: (entry: DailyListEntry) => ReactNode }) {
  const [visibleCount, setVisibleCount] = useState(LIST_CHUNK)
  const sentinelRef = useRef<HTMLDivElement | null>(null)
  const hasMore = visibleCount < entries.length

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (!sentinel) {
      return
    }
    // The parent is the scroll container; it must be the observer root —
    // rootMargin on the default (viewport) root does not expand the clip
    // rect of a scrollable ancestor, so prefetch would never trigger.
    const observer = new IntersectionObserver(
      (hits) => {
        if (hits.some((hit) => hit.isIntersecting)) {
          setVisibleCount((count) => count + LIST_CHUNK)
        }
      },
      { root: sentinel.parentElement, rootMargin: '600px' },
    )
    observer.observe(sentinel)
    return () => observer.disconnect()
    // re-observe after each growth: the sentinel may still be in range and the
    // observer only fires on intersection *changes*
  }, [hasMore, visibleCount])

  return (
    <>
      {entries.slice(0, visibleCount).map(children)}
      {hasMore ? <div ref={sentinelRef} aria-hidden="true" className="h-px" /> : null}
    </>
  )
}

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
  const [openMenuId, setOpenMenuId] = useState<string | null>(null)
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
        <span title={account.owner.name}>
          <UserAvatar avatar={account.owner.avatar} size="xs" />
        </span>
        {account.sharedAccess.map((access) => (
          <span key={access.user.id} title={access.user.name}>
            <UserAvatar avatar={access.user.avatar} size="xs" />
          </span>
        ))}
      </span>
    ) : null

  return (
    <div className="flex h-full flex-col gap-3 p-4">
      {isCompact ? (
        <>
          <header className="flex items-center gap-3">
            <Button type="button" variant="ghost" size="icon" aria-label="back" title={t('elements.button.back.label')} onClick={() => navigate(RouterPage.HOME)}>
              <ChevronLeft className="size-5" />
            </Button>
            <EntityIcon name={account.icon} className="text-2xl text-muted-foreground" />
            <div className="min-w-0 flex-1">
              <h1 className="truncate text-lg uppercase" title={account.name}>
                {account.name}
              </h1>
              <span className="flex items-center gap-2">
                <p className="text-sm text-muted-foreground" data-testid="account-balance">
                  {moneyFormat(account.balance, account.currency, { useNativePrecision: false })}
                </p>
                {sharedAvatars}
              </span>
            </div>
            {canUpdateSettings ? (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={t('pages.account.toolbar.settings')}
                title={t('pages.account.toolbar.settings')}
                onClick={() => openAccountModal({ account })}
              >
                <Settings2 className="size-5" />
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
                // the budget page's Configure button, minus the dropdown
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="uppercase tracking-wide text-muted-foreground"
                  aria-label={t('pages.account.toolbar.settings')}
                  title={t('pages.account.toolbar.settings')}
                  onClick={() => openAccountModal({ account })}
                >
                  <Settings2 className="size-4" />
                  <span className="hidden sm:inline">{t('pages.account.toolbar.settings')}</span>
                </Button>
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
        <WindowedEntries key={account.id} entries={entries}>
          {(entry) =>
            entry.kind === 'separator' ? (
            <div key={`sep-${entry.day}`} className="px-2 pb-1 pt-4 text-xs font-medium uppercase text-muted-foreground">
              {separatorText(entry, t)}
            </div>
          ) : (
            // The whole row is one hover/tap surface (like the settings lists):
            // click opens the transaction preview; desktop keeps the kebab menu
            // as an edit/delete shortcut.
            <div
              key={entry.transaction.id}
              className={`flex items-start rounded-md ${isCompact ? 'active:bg-accent' : 'hover:bg-accent cursor-pointer'}`}
              onClick={() => setPreview(entry.transaction)}
            >
              <div className="min-w-0 flex-1">
                <TransactionRow transaction={entry.transaction} pageAccount={account} />
              </div>
              {!isCompact && canTouchRow(entry.transaction) ? (
                <DropdownMenu
                  open={openMenuId === entry.transaction.id}
                  onOpenChange={(open) => setOpenMenuId(open ? entry.transaction.id : null)}
                >
                  <DropdownMenuTrigger asChild>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="mt-0.5"
                      aria-label={`actions ${entry.transaction.id}`}
                      onClick={(e) => e.stopPropagation()}
                    >
                      <MoreVertical className="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  {/* portaled content still bubbles React clicks to the row — don't reopen the menu */}
                  <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
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
          )
          }
        </WindowedEntries>
      </div>

      {isCompact && canChangeTransaction ? (
        <footer className="shrink-0 pb-[env(safe-area-inset-bottom)]">
          <Button
            type="button"
            className="h-11 w-full"
            title={t('pages.account.transaction_list.action.add_transaction')}
            onClick={() => openTransactionModal({ type: 'expense' })}
          >
            <Plus className="size-4" />
            {t('pages.account.transaction_list.action.add_transaction')}
          </Button>
        </footer>
      ) : null}

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
          isShared={account.sharedAccess.length > 0}
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
        destructive
      />
    </div>
  )
}
