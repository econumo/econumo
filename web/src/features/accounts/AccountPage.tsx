import { useState } from 'react'
import { formatDate } from '@/lib/datetime'
import { ChevronLeft, GripVertical, MoreVertical, Plus, Settings2 } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from 'react-router'
import type { RecurringDto } from '@/api/dto/recurring'
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
import { TransactionHistory } from './TransactionHistory'
import { HistoryDragProvider, SortableHistoryEntry } from './HistoryDrag'
import { entryKey } from './historyDrop'
import type { HistoryDrop } from './historyDrop'
import { useUserData } from '@/features/user/queries'
import { useDeleteTransaction, useMoveTransaction } from '@/features/transactions/queries'
import { useDeleteRecurring, usePostRecurring, useRecurring, useSkipRecurring } from '@/features/recurring/queries'
import { ViewRecurringDialog } from '@/features/recurring/ViewRecurringDialog'
import { recurringPostPayload } from '@/features/recurring/postPayload'
import { useExchange } from '@/features/currencies/useExchange'
import { separatorText, useAccountTransactions } from '@/features/transactions/useAccountTransactions'
import type { DailyListEntry, ViewTransaction } from '@/features/transactions/useAccountTransactions'
import { TransactionRow } from '@/features/transactions/TransactionRow'
import { ViewTransactionDialog } from '@/features/transactions/ViewTransactionDialog'
import { canTouchTransaction } from '@/features/transactions/canTouchTransaction'
import { canWriteToAccount } from '@/features/connections/shared'

export function AccountPage() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const isCompact = useIsCompact()
  const { id } = useParams()
  const { data: accounts } = useAccounts()
  const { data: user } = useUserData()
  const deleteTransaction = useDeleteTransaction()
  const moveTransaction = useMoveTransaction()
  const skipRecurring = useSkipRecurring()
  const postRecurring = usePostRecurring()
  const deleteRecurring = useDeleteRecurring()
  const exchangeFn = useExchange()
  // already cached by useAccountTransactions, so resolving a posted
  // transaction's template costs no extra request
  const { data: recurringList } = useRecurring()
  const openTransactionModal = useUiStore((s) => s.openTransactionModal)
  const openAccountModal = useUiStore((s) => s.openAccountModal)
  const openRecurringModal = useUiStore((s) => s.openRecurringModal)

  const [search, setSearch] = useState('')
  const [historyNavigationTarget, setHistoryNavigationTarget] = useState<HTMLDivElement | null>(null)
  // still reachable from the desktop kebab on an unposted recurring row, even
  // though the preview dialog itself no longer offers delete
  const [recurringDeleteTarget, setRecurringDeleteTarget] = useState<RecurringDto | null>(null)
  const [preview, setPreview] = useState<ViewTransaction | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ViewTransaction | null>(null)
  // the template being previewed: reached either from its own (unposted) list row
  // or from the provenance row inside a posted transaction's preview
  const [recurringPreview, setRecurringPreview] = useState<RecurringDto | null>(null)
  const [openMenuId, setOpenMenuId] = useState<string | null>(null)
  const liveEntries = useAccountTransactions(id, search)
  // A drop shows at once: the list keeps the dropped arrangement until the
  // server answers, then follows the cache again (the new keys, or the old
  // order after a refusal).
  const [droppedEntries, setDroppedEntries] = useState<DailyListEntry[] | null>(null)
  const entries = droppedEntries ?? liveEntries

  // the template behind the previewed transaction. Stays undefined when it was
  // deleted or lives on an account the caller can't see (the list is scoped to
  // visible accounts), in which case the preview shows no provenance row.
  const previewRecurring = preview?.recurringId
    ? recurringList?.find((rt) => rt.id === preview.recurringId)
    : undefined

  // The first entry at or before today; everything above it is the future
  // block (upcoming templates, post-dated transactions) that the initial
  // scroll hides above the fold. Day separators lead every group, so matching
  // on them finds the boundary.
  const todayKey = formatDate(new Date())
  const anchorIndex = entries.findIndex((e) => e.kind === 'separator' && e.day <= todayKey)

  // The interval trailing a recurring row's title, as on the settings list. A
  // virtual row carries its template; a posted one resolves it by recurringId,
  // and shows nothing when the template is gone or not visible.
  const scheduleNote = (tx: ViewTransaction): string | undefined => {
    const schedule =
      tx.recurring?.schedule ?? (tx.recurringId ? recurringList?.find((rt) => rt.id === tx.recurringId)?.schedule : undefined)
    return schedule ? t(`recurring.schedule.${schedule}`) : undefined
  }

  const account = accounts?.find((a) => a.id === id)
  if (!account) {
    return null
  }

  const myRole = account.sharedAccess.find((access) => access.user.id === user?.id)?.role
  const isOwner = account.owner.id === user?.id
  const canUpdateSettings = isOwner || myRole === 'admin'
  // Read-only access does NOT hide write controls: the backend rejects the
  // write with 402 and the global handler explains why. Only sharing roles
  // gate the UI.
  const canChangeTransaction = canWriteToAccount(account, user?.id)

  const canTouchRow = (tx: ViewTransaction): boolean => canTouchTransaction(tx, canChangeTransaction)
  // reordering writes only this account's order, so write access to it is
  // enough; moving to another day edits the transaction (canTouchRow)
  const canDrag = (tx: ViewTransaction): boolean => canChangeTransaction && !tx.recurring && search.trim() === ''

  const dropTransaction = (tx: ViewTransaction, drop: HistoryDrop, dayChanged: boolean) => {
    if (dayChanged && !canTouchRow(tx)) {
      toast.error(t('errors.transaction.cannot_move'))
      return
    }
    setDroppedEntries(drop.entries)
    moveTransaction.mutate(
      { id: tx.id, accountId: account.id, date: drop.date, aboveId: drop.aboveId },
      { onSettled: () => setDroppedEntries(null) },
    )
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
    <div className="flex h-full min-h-0 flex-col gap-3 p-4">
      {isCompact ? (
        <>
          <header className="flex flex-col gap-2">
            <div className="flex items-center gap-3">
              <Button type="button" variant="ghost" size="icon" aria-label="back" title={t('common.button.back.label')} onClick={() => navigate(RouterPage.HOME)}>
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
                  aria-label={t('accounts.page.toolbar.settings')}
                  title={t('accounts.page.toolbar.settings')}
                  onClick={() => openAccountModal({ account })}
                >
                  <Settings2 className="size-5" />
                </Button>
              ) : null}
            </div>
            <div ref={setHistoryNavigationTarget} className="relative h-3 border-b" />
          </header>
          <Input
            aria-label={t('accounts.page.toolbar.search')}
            placeholder={t('accounts.page.toolbar.search')}
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
                <EntityIcon name={account.icon} className="text-lg text-entity-icon" />
              </span>
              {sharedAvatars}
            </div>
            <div className="flex items-end justify-between gap-3">
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
                  aria-label={t('accounts.page.toolbar.settings')}
                  title={t('accounts.page.toolbar.settings')}
                  onClick={() => openAccountModal({ account })}
                >
                  <Settings2 className="size-4" />
                  <span className="hidden sm:inline">{t('accounts.page.toolbar.settings')}</span>
                </Button>
              ) : null}
            </div>
            <div ref={setHistoryNavigationTarget} className="relative h-3 border-b" />
          </header>
          <div className="flex items-center justify-between gap-3">
            {canChangeTransaction ? (
              <Button
                type="button"
                aria-label={t('accounts.page.transaction_list.action.add_transaction')}
                onClick={() => openTransactionModal({ type: 'expense' })}
              >
                {t('accounts.page.transaction_list.action.add_transaction')}
              </Button>
            ) : (
              <span />
            )}
            <Input
              aria-label={t('accounts.page.toolbar.search')}
              placeholder={t('accounts.page.toolbar.search')}
              className="w-60 border-0 bg-econumo-card shadow-none"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
        </>
      )}

      <HistoryDragProvider
        entries={entries}
        touch={isCompact}
        onDrop={dropTransaction}
        renderOverlay={(tx) => <TransactionRow transaction={tx} pageAccount={account} />}
      >
      <TransactionHistory
        key={`${account.id}:${search}`}
        entries={entries}
        anchorIndex={anchorIndex}
        ready={recurringList !== undefined}
        navigationTarget={historyNavigationTarget}
      >
          {(entry) =>
            entry.kind === 'separator' ? (
            <SortableHistoryEntry key={`sep-${entry.day}`} id={entryKey(entry)} draggable={false} touch={isCompact}>
              {({ targetDay }) => (
                <div
                  className={`rounded-md px-2 pb-1 pt-4 text-xs font-medium uppercase ${targetDay === entry.day ? 'bg-accent text-foreground' : 'text-muted-foreground'}`}
                >
                  {separatorText(entry, t, i18n.language)}
                </div>
              )}
            </SortableHistoryEntry>
          ) : (
            // The whole row is one hover/tap surface (like the settings lists):
            // click opens the transaction preview; desktop keeps the kebab menu
            // as an edit/delete shortcut.
            // A virtual row is one occurrence, not the template: posting or
            // skipping advances nextPaymentAt, and a template-id key would MOVE
            // the just-tapped node to the next (later) date — browser scroll
            // anchoring then follows it to the top of the future block.
            <SortableHistoryEntry
              key={entryKey(entry)}
              id={entryKey(entry)}
              draggable={canDrag(entry.transaction)}
              touch={isCompact}
            >
              {({ row, handle, justDragged }) => (
            <div
              {...row}
              className={`group flex items-start rounded-md ${isCompact ? 'active:bg-accent' : 'hover:bg-accent cursor-pointer'} ${isCompact && canDrag(entry.transaction) ? 'select-none [-webkit-touch-callout:none]' : ''}`}
              onClick={() => {
                if (justDragged()) {
                  return
                }
                const rt = entry.transaction.recurring
                if (rt) {
                  setRecurringPreview(rt)
                } else {
                  setPreview(entry.transaction)
                }
              }}
            >
              {handle ? (
                <button
                  type="button"
                  aria-label={t('accounts.page.transaction_list.drag_handle')}
                  title={t('accounts.page.transaction_list.drag_handle')}
                  className="mt-2.5 -mr-1 flex w-4 shrink-0 cursor-grab touch-none justify-center text-muted-foreground opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
                  onClick={(e) => e.stopPropagation()}
                  {...handle}
                >
                  <GripVertical className="size-4" />
                </button>
              ) : !isCompact ? (
                <span className="-mr-1 w-4 shrink-0" aria-hidden="true" />
              ) : null}
              <div className="min-w-0 flex-1">
                <TransactionRow
                  transaction={entry.transaction}
                  pageAccount={account}
                  titleNote={scheduleNote(entry.transaction)}
                  /* the dimming says "not real yet" at a glance; this says why.
                     Red when due — those rows sit interleaved among past days
                     (the future block is above the fold) and need to pop */
                  amountNote={
                    entry.transaction.recurring ? (
                      <span className={entry.transaction.isInFuture ? undefined : 'text-destructive'}>
                        {t('recurring.not_posted')}
                      </span>
                    ) : undefined
                  }
                />
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
                    {entry.transaction.recurring ? (
                      <>
                        <DropdownMenuItem onSelect={() => openRecurringModal({ recurring: entry.transaction.recurring })}>
                          {t('common.button.edit.label')}
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          variant="destructive"
                          onSelect={() => setRecurringDeleteTarget(entry.transaction.recurring ?? null)}
                        >
                          {t('common.button.delete.label')}
                        </DropdownMenuItem>
                      </>
                    ) : (
                      <>
                        <DropdownMenuItem onSelect={() => editTransaction(entry.transaction)}>
                          {t('common.button.edit.label')}
                        </DropdownMenuItem>
                        <DropdownMenuItem variant="destructive" onSelect={() => setDeleteTarget(entry.transaction)}>
                          {t('common.button.delete.label')}
                        </DropdownMenuItem>
                      </>
                    )}
                  </DropdownMenuContent>
                </DropdownMenu>
              ) : null}
            </div>
              )}
            </SortableHistoryEntry>
          )
          }
      </TransactionHistory>
      </HistoryDragProvider>

      {isCompact && canChangeTransaction ? (
        <footer className="shrink-0 pb-[env(safe-area-inset-bottom)]">
          <Button
            type="button"
            className="h-11 w-full"
            title={t('accounts.page.transaction_list.action.add_transaction')}
            onClick={() => openTransactionModal({ type: 'expense' })}
          >
            <Plus className="size-4" />
            {t('accounts.page.transaction_list.action.add_transaction')}
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
          onMakeRecurring={() => {
            const txToMakeRecurring = preview
            setPreview(null)
            openRecurringModal({ fromTransaction: txToMakeRecurring })
          }}
          recurringSchedule={previewRecurring?.schedule}
          onOpenRecurring={
            previewRecurring
              ? () => {
                  // straight to the template editor: the template PREVIEW renders
                  // the same body as this dialog, so hopping through it reads as
                  // the preview merely refreshing
                  setPreview(null)
                  openRecurringModal({ recurring: previewRecurring })
                }
              : undefined
          }
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
        question={t('accounts.page.delete_transaction_modal.question')}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />

      {recurringPreview ? (
        <ViewRecurringDialog
          recurring={recurringPreview}
          onClose={() => setRecurringPreview(null)}
          onPost={() => {
            postRecurring.mutate(recurringPostPayload(recurringPreview, accounts ?? [], exchangeFn), {
              onSuccess: () => setRecurringPreview(null),
            })
          }}
          onSkip={() => {
            skipRecurring.mutate(recurringPreview.id, { onSuccess: () => setRecurringPreview(null) })
          }}
          onEdit={() => {
            setRecurringPreview(null)
            openRecurringModal({ recurring: recurringPreview })
          }}
          canChange={canChangeTransaction}
          skipPending={skipRecurring.isPending}
          postPending={postRecurring.isPending}
        />
      ) : null}

      <ConfirmDialog
        open={recurringDeleteTarget !== null}
        onClose={() => setRecurringDeleteTarget(null)}
        onConfirm={() => {
          if (recurringDeleteTarget) {
            deleteRecurring.mutate(recurringDeleteTarget.id, { onSettled: () => setRecurringDeleteTarget(null) })
          }
        }}
        question={t('settings.recurring.delete_question')}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </div>
  )
}
