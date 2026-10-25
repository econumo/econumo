import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2, Repeat } from 'lucide-react'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { METRICS, trackEvent } from '@/lib/metrics'
import { moneyFormat } from '@/lib/money'
import { dayKey, formatDayHeading, isFuture, isToday, isYesterday, parseDateTime } from '@/lib/datetime'
import type { BudgetDto, BudgetTransactionDto } from '@/api/dto/budget'
import { BudgetElementType } from '@/api/dto/budget'
import type { CategoryDto } from '@/api/dto/category'
import type { Id } from '@/api/types'
import type { PayeeDto } from '@/api/dto/payee'
import type { TagDto } from '@/api/dto/tag'
import { useUiStore } from '@/app/uiStore'
import { useAccounts } from '@/features/accounts/queries'
import { canWriteToAccount } from '@/features/connections/shared'
import { canTouchTransaction } from '@/features/transactions/canTouchTransaction'
import { useCategories, usePayees, useTags } from '@/features/classifications/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData } from '@/features/user/queries'
import { useDeleteTransaction, useTransactions } from '@/features/transactions/queries'
import type { ViewTransaction } from '@/features/transactions/useAccountTransactions'
import { ViewTransactionDialog } from '@/features/transactions/ViewTransactionDialog'
import { elementDisplayName } from './budgetMath'
import { useBudgetTransactions } from './queries'
import { budgetTransactionsParams } from './budgetTransactionsParams'
import { useBudgetPeriodStore } from './budgetStore'

/** enough of an element (or a nested child category) to list its transactions.
 *  'label' and 'transfers' are client-side discriminants only -- neither is a
 *  real BudgetElementType, neither appears in structure.elements or on the wire
 *  as an element type. 'transfers' is the plan sheet's Transfers line: the
 *  transfers that crossed the budget boundary in the period. */
export interface BudgetTransactionsTarget {
  id: Id
  type: BudgetElementType | 'label' | 'transfers'
  name: string
  icon: string
  /** null = the budget base currency */
  currencyId: Id | null
  /** set on a nested child row: the row it is listed under */
  parent?: { id: Id; type: BudgetElementType | 'label' }
}

/** the id a 'transfers' target carries; never a real element id */
export const TRANSFERS_TARGET_ID = 'transfers'

interface BudgetTransactionsDialogProps {
  budget: BudgetDto
  element: BudgetTransactionsTarget | null
  onClose: () => void
  /** the month to list (Y-m-d, first of month). Defaults to the budget page's
   *  selected period; the plan sheet passes the clicked column's month. */
  periodStart?: string
}

function transactionsKind(type: BudgetTransactionsTarget['type']): string {
  switch (type) {
    case 'transfers':
    case 'label':
      return type
    case BudgetElementType.SAVINGS:
      return 'savings'
    case BudgetElementType.INCOME_CATEGORY:
    case BudgetElementType.INCOME_ENVELOPE:
      return 'income'
    default:
      return 'expense'
  }
}

export function BudgetTransactionsDialog({ budget, element, onClose, periodStart }: BudgetTransactionsDialogProps) {
  const { t, i18n } = useTranslation()
  const storeSelectedDate = useBudgetPeriodStore((s) => s.selectedDate)
  const selectedDate = periodStart ?? storeSelectedDate
  const { data: currencies = [] } = useCurrencies()
  const { data: user } = useUserData()
  const { data: allTransactions } = useTransactions()
  const { data: accounts } = useAccounts()
  const { data: categories } = useCategories()
  const { data: payees } = usePayees()
  const { data: tags } = useTags()
  const deleteTransaction = useDeleteTransaction()
  const openTransactionModal = useUiStore((s) => s.openTransactionModal)

  const [preview, setPreview] = useState<ViewTransaction | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ViewTransaction | null>(null)

  const params = element ? budgetTransactionsParams(budget.meta.id, element, selectedDate) : null
  const { data, isLoading } = useBudgetTransactions(params)
  const transactions = data?.items

  const openedKind = element ? transactionsKind(element.type) : null
  const openedId = element?.id
  useEffect(() => {
    if (openedKind) {
      trackEvent(METRICS.BUDGET_TRANSACTIONS_OPEN, { kind: openedKind })
    }
  }, [openedKind, openedId])

  if (!element) {
    return null
  }

  const displayName = elementDisplayName(element.id, element.name, t)

  // full ViewTransaction when the caller can see the transaction in their own
  // list; otherwise (a partner's row in a shared budget) a read-only shape is
  // synthesized from the budget wire — enough for the preview, never editable
  const toViewTransaction = (wireTx: BudgetTransactionDto): ViewTransaction => {
    const tx = allTransactions?.find((item) => item.id === wireTx.id)
    if (tx) {
      return {
        ...tx,
        account: accounts?.find((a) => a.id === tx.accountId),
        accountRecipient: tx.accountRecipientId ? accounts?.find((a) => a.id === tx.accountRecipientId) : undefined,
        category: tx.categoryId ? categories?.find((c) => c.id === tx.categoryId) : undefined,
        payee: tx.payeeId ? payees?.find((p) => p.id === tx.payeeId) : undefined,
        tag: tx.tagId ? tags?.find((tg) => tg.id === tx.tagId) : undefined,
        isInFuture: isFuture(tx.date),
      }
    }
    return {
      id: wireTx.id,
      author: wireTx.author,
      type: wireTx.type ?? (wireTx.direction ? 'transfer' : 'expense'),
      accountId: '',
      accountRecipientId: null,
      amount: wireTx.amount,
      amountRecipient: null,
      categoryId: wireTx.category?.id ?? null,
      description: wireTx.description,
      payeeId: wireTx.payee?.id ?? null,
      tagId: wireTx.tag?.id ?? null,
      // the budget wire carries the row's reporting labels; the preview
      // resolves them itself from the owner's label list
      labelIds: wireTx.labelIds ?? [],
      date: wireTx.spentAt,
      category: wireTx.category ? (wireTx.category as CategoryDto) : undefined,
      payee: wireTx.payee ? (wireTx.payee as PayeeDto) : undefined,
      tag: wireTx.tag ? (wireTx.tag as TagDto) : undefined,
      isInFuture: isFuture(wireTx.spentAt),
      // the budget wire carries no provenance; this synthesized shape is
      // read-only anyway, so no recurring action is offered on it
      recurringId: null,
      isImported: 0,
    }
  }

  const canChange = (tx: ViewTransaction): boolean =>
    canTouchTransaction(tx, !!tx.account && canWriteToAccount(tx.account, user?.id))

  let currentDay: string | null = null
  const rows: { kind: 'sep' | 'tx'; key: string; label?: string; tx?: BudgetTransactionDto }[] = []
  // What is still to come leads the list, laid out like the plan editor's "Coming up" list:
  // rows dated after today and recurring payments still due (neither is in the figure; shown for
  // reference only: the budget offers no Post/Skip), soonest first.
  const later = [
    ...(transactions ?? []).filter((tx) => isFuture(tx.spentAt)).map((tx) => ({ tx, recurring: false })),
    ...(data?.upcoming ?? []).map((tx) => ({ tx, recurring: true })),
  ].sort((a, b) => a.tx.spentAt.localeCompare(b.tx.spentAt))
  const shortDate = new Intl.DateTimeFormat(i18n.language, { day: 'numeric', month: 'short' })
  const signedMoney = (tx: BudgetTransactionDto) => {
    const currency = currencies.find((c) => c.id === tx.currencyId)
    // spend is negative; money arriving (direction 'in') is positive
    return moneyFormat(tx.direction === 'in' ? tx.amount : -tx.amount, currency, {
      useNativePrecision: false,
      maxPrecision: currency?.fractionDigits ?? 2,
    })
  }
  for (const tx of (transactions ?? []).filter((tx) => !isFuture(tx.spentAt))) {
    const day = dayKey(tx.spentAt)
    if (day !== currentDay) {
      currentDay = day
      const label = isToday(day)
        ? t('accounts.page.transaction_list.today')
        : isYesterday(day)
          ? t('accounts.page.transaction_list.yesterday')
          : formatDayHeading(day, i18n.language)
      rows.push({ kind: 'sep', key: `sep-${day}`, label })
    }
    rows.push({ kind: 'tx', key: tx.id, tx })
  }

  return (
    <>
      {/* interactions inside the stacked preview/confirm must not dismiss the list */}
      <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={displayName} dismissible={!preview && !deleteTarget}>
        {isLoading ? (
          <div className="flex justify-center py-6">
            <Loader2 className="size-6 animate-spin text-muted-foreground" aria-label="loading" />
          </div>
        ) : rows.length === 0 && later.length === 0 ? (
          <p className="py-2 text-sm text-muted-foreground">{t('common.list.list_empty')}</p>
        ) : (
          <div className="max-h-80 overflow-y-auto scrollbar-slim">
            {later.length > 0 ? (
              <div data-testid="budget-tx-coming-up" className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-baseline gap-x-2 px-1 pt-1 text-sm">
                <p className="col-span-3 text-foreground">{t('budgets.page.upcoming.title')}</p>
                {later.map(({ tx, recurring }) => {
                  const line = (
                    <>
                      <span>{shortDate.format(parseDateTime(dayKey(tx.spentAt)))}</span>
                      <span className="flex min-w-0 items-center gap-1">
                        <span className="truncate">{tx.description || tx.payee?.name || tx.category?.name || displayName}</span>
                        {recurring ? <Repeat className="size-3.5 shrink-0" aria-label={t('recurring.preview.header')} /> : null}
                      </span>
                      <span className="text-right tabular-nums whitespace-nowrap">{signedMoney(tx)}</span>
                    </>
                  )
                  // display only: nothing still to come is opened from here
                  return (
                    <div
                      key={recurring ? `r-${tx.id}-${tx.spentAt}` : tx.id}
                      className="col-span-3 grid grid-cols-subgrid items-baseline py-0.5 text-muted-foreground"
                      data-testid={recurring ? `budget-upcoming-${tx.id}` : `budget-tx-${tx.id}`}
                    >
                      {line}
                    </div>
                  )
                })}
              </div>
            ) : null}
            {later.length > 0 && rows.length > 0 ? (
              <p className="px-1 pt-3 text-sm text-foreground" data-testid="budget-tx-posted">
                {t('budgets.page.upcoming.posted')}
              </p>
            ) : null}
            {rows.map((row) =>
              row.kind === 'sep' ? (
                <p key={row.key} className="px-1 pb-1 pt-3 text-xs font-medium uppercase text-muted-foreground">
                  {row.label}
                </p>
              ) : (
                (() => {
                  const tx = row.tx!
                  const classification = [tx.category?.name, tx.payee?.name].filter(Boolean).join(' · ')
                  const content = (
                    <>
                      <EntityIcon name={tx.category?.icon || element.icon} className="text-base text-muted-foreground" />
                      <span className="flex min-w-0 flex-1 flex-col text-left">
                        {tx.description ? (
                          <>
                            <span className="truncate">{tx.description}</span>
                            {classification ? (
                              <span className="truncate text-xs text-muted-foreground">{classification}</span>
                            ) : null}
                          </>
                        ) : (
                          // no note: category and payee share the one line
                          <span className="truncate">
                            {tx.category?.name || (tx.payee ? null : displayName)}
                            {tx.payee ? (
                              <span className="text-muted-foreground">
                                {tx.category ? ` · ${tx.payee.name}` : tx.payee.name}
                              </span>
                            ) : null}
                          </span>
                        )}
                      </span>
                      <span className="tabular-nums text-muted-foreground">{signedMoney(tx)}</span>
                    </>
                  )
                  return (
                    <button
                      key={row.key}
                      type="button"
                      title={t('accounts.page.preview_transaction_modal.header')}
                      className="flex w-full items-center gap-2 rounded-md px-1 py-1.5 text-sm hover:bg-accent/50"
                      data-testid={`budget-tx-${row.key}`}
                      onClick={() => setPreview(toViewTransaction(tx))}
                    >
                      {content}
                    </button>
                  )
                })()
              ),
            )}
          </div>
        )}
      </ResponsiveDialog>

      {preview ? (
        <ViewTransactionDialog
          transaction={preview}
          onClose={() => setPreview(null)}
          onEdit={() => {
            openTransactionModal({ transaction: preview })
            setPreview(null)
          }}
          // the preview stays open under the confirm: unmount+mount in one tick
          // races Radix's aria-hidden bookkeeping, and cancel returns to the preview
          onDelete={() => setDeleteTarget(preview)}
          canChange={canChange(preview)}
          // no visible account = someone else's transaction — the author matters then
          isShared={(preview.account?.sharedAccess.length ?? 0) > 0 || !preview.account}
          dismissible={deleteTarget === null}
          fallbackCurrency={currencies.find((c) => c.id === transactions?.find((tx) => tx.id === preview.id)?.currencyId)}
        />
      ) : null}

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) {
            // the mutation invalidates the budget + this list, so the numbers refresh
            deleteTransaction.mutate(deleteTarget.id, {
              onSuccess: () => setPreview(null),
              onSettled: () => setDeleteTarget(null),
            })
          }
        }}
        question={t('accounts.page.delete_transaction_modal.question')}
        confirmLabel={t('common.button.delete.label')}
        cancelLabel={t('common.button.cancel.label')}
        destructive
      />
    </>
  )
}
