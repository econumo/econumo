import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2, Repeat } from 'lucide-react'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { METRICS, trackEvent } from '@/lib/metrics'
import { moneyFormat } from '@/lib/money'
import { dayKey, formatDayHeading, isFuture, isToday, isYesterday, parseDateTime } from '@/lib/datetime'
import type { BudgetDto, BudgetTransactionDto, BudgetUpcomingDto } from '@/api/dto/budget'
import { BudgetElementType, UNCATEGORIZED_ID } from '@/api/dto/budget'
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
import { useExchange } from '@/features/currencies/useExchange'
import { recurringPostPayload } from '@/features/recurring/postPayload'
import { usePostRecurring, useRecurring, useSkipRecurring } from '@/features/recurring/queries'
import { ViewRecurringDialog } from '@/features/recurring/ViewRecurringDialog'
import { useUserData } from '@/features/user/queries'
import { useDeleteTransaction, useTransactions } from '@/features/transactions/queries'
import type { ViewTransaction } from '@/features/transactions/useAccountTransactions'
import { ViewTransactionDialog } from '@/features/transactions/ViewTransactionDialog'
import { elementDisplayName } from './budgetMath'
import { useBudgetTransactions } from './queries'
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
  const openRecurringModal = useUiStore((s) => s.openRecurringModal)
  const { data: recurringList } = useRecurring()
  const exchangeFn = useExchange()
  const postRecurring = usePostRecurring()
  const skipRecurring = useSkipRecurring()

  const [preview, setPreview] = useState<ViewTransaction | null>(null)
  const [upcomingPreview, setUpcomingPreview] = useState<BudgetUpcomingDto | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ViewTransaction | null>(null)

  // A category listed under a tag displays the tag-and-category intersection.
  // Sending categoryId alone puts the backend on its "tag_id IS NULL" branch,
  // which returns the complement of what the row shows.
  const isUncategorized = element?.id === UNCATEGORIZED_ID
  const parentTagId = element?.parent?.type === BudgetElementType.TAG ? element.parent.id : undefined
  // a category listed under a reporting label narrows the same way; labelId is
  // the one filter the backend accepts alongside categoryId/uncategorized
  const parentLabelId = element?.parent?.type === 'label' ? element.parent.id : undefined
  const params = element
    ? {
        budgetId: budget.meta.id,
        periodStart: selectedDate,
        // the parent tag only ever applies to a CATEGORY-typed element, so it
        // is folded into that branch rather than spread last, which would
        // silently clobber a TAG-typed element's own tagId
        //
        // The Uncategorized element is presentation-only and has no id the
        // backend can look up, so it is selected by flag instead; it composes
        // with the parent tag exactly as a real category does. Never send
        // uncategorized together with categoryId -- the backend rejects that.
        ...(element.type === BudgetElementType.CATEGORY
          ? {
              ...(isUncategorized ? { uncategorized: true } : { categoryId: element.id }),
              ...(parentTagId ? { tagId: parentTagId } : {}),
              ...(parentLabelId ? { labelId: parentLabelId } : {}),
            }
          : {}),
        ...(element.type === BudgetElementType.TAG ? { tagId: element.id } : {}),
        // labelId is mutually exclusive with categoryId/tagId/envelopeId/uncategorized
        // on the backend (400 if combined); 'label' never equals a real
        // BudgetElementType, so this branch can never fire alongside another one
        ...(element.type === 'label' ? { labelId: element.id } : {}),
        ...(element.type === BudgetElementType.ENVELOPE ? { envelopeId: element.id } : {}),
        // transfers composes with nothing on the backend; like 'label' it can
        // never coincide with another branch here
        ...(element.type === 'transfers' ? { transfers: true } : {}),
        // a savings row's id is its account id; composes with nothing
        ...(element.type === BudgetElementType.SAVINGS ? { accountId: element.id } : {}),
        ...(element.type === BudgetElementType.INCOME_CATEGORY ? { income: true, categoryId: element.id } : {}),
        ...(element.type === BudgetElementType.INCOME_ENVELOPE ? { income: true, envelopeId: element.id } : {}),
      }
    : null
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

  // upcoming occurrences join the real rows' day groups, so posting one only
  // changes its style, never its place; within a day real rows come first
  const listed: { tx: BudgetTransactionDto; upcoming?: BudgetUpcomingDto }[] = [
    ...(transactions ?? []).map((tx) => ({ tx })),
    ...(data?.upcoming ?? []).map((u) => ({ tx: u, upcoming: u })),
  ].sort((a, b) => dayKey(b.tx.spentAt).localeCompare(dayKey(a.tx.spentAt)))

  let currentDay: string | null = null
  const rows: { kind: 'sep' | 'tx'; key: string; label?: string; tx?: BudgetTransactionDto; upcoming?: BudgetUpcomingDto }[] = []
  for (const { tx, upcoming } of listed) {
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
    rows.push(
      upcoming
        ? { kind: 'tx', key: `upcoming-${upcoming.templateId}-${day}`, tx, upcoming }
        : { kind: 'tx', key: `tx-${tx.id}`, tx },
    )
  }

  const overdueNote = (spentAt: string) => {
    const date = new Intl.DateTimeFormat(i18n.language, { day: 'numeric', month: 'short' }).format(parseDateTime(dayKey(spentAt)))
    return t('inbox.due.overdue', { date })
  }

  // acts only on the template's next payment, and only when the caller has the
  // template in their own list to act through
  const upcomingTemplate = upcomingPreview
    ? recurringList?.find((rt) => rt.id === upcomingPreview.templateId && upcomingPreview.canEdit)
    : undefined

  return (
    <>
      {/* interactions inside the stacked preview/confirm must not dismiss the list */}
      <ResponsiveDialog
        open
        onOpenChange={(o) => !o && onClose()}
        title={displayName}
        dismissible={!preview && !deleteTarget && !upcomingPreview}
      >
        {isLoading ? (
          <div className="flex justify-center py-6">
            <Loader2 className="size-6 animate-spin text-muted-foreground" aria-label="loading" />
          </div>
        ) : rows.length === 0 ? (
          <p className="py-2 text-sm text-muted-foreground">{t('common.list.list_empty')}</p>
        ) : (
          <div className="max-h-80 overflow-y-auto scrollbar-slim">
            {rows.map((row) =>
              row.kind === 'sep' ? (
                <p key={row.key} className="px-1 pb-1 pt-3 text-xs font-medium uppercase text-muted-foreground">
                  {row.label}
                </p>
              ) : (
                (() => {
                  const tx = row.tx!
                  const upcoming = row.upcoming
                  const currency = currencies.find((c) => c.id === tx.currencyId)
                  const classification = [tx.category?.name, tx.payee?.name].filter(Boolean).join(' · ')
                  return (
                    <button
                      key={row.key}
                      type="button"
                      title={upcoming ? t('budgets.transactions.upcoming_hint') : t('accounts.page.preview_transaction_modal.header')}
                      // money that has not moved yet is dimmed, like the account list's future rows
                      className={`flex w-full items-center gap-2 rounded-md px-1 py-1.5 text-sm hover:bg-accent/50 ${upcoming || isFuture(tx.spentAt) ? 'opacity-50' : ''}`}
                      data-testid={`budget-${row.key}`}
                      onClick={() => (upcoming ? setUpcomingPreview(upcoming) : setPreview(toViewTransaction(tx)))}
                    >
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
                      {upcoming ? (
                        // the same schedule glyph the account list puts on template rows
                        <span title={t('recurring.preview.header')} className="flex shrink-0 items-center">
                          <Repeat className="size-3 text-muted-foreground" aria-label={t('recurring.preview.header')} />
                        </span>
                      ) : null}
                      <span className="flex shrink-0 flex-col items-end">
                        <span className="tabular-nums text-muted-foreground">
                          {/* spend is negative; money arriving (direction 'in') is positive */}
                          {moneyFormat(tx.direction === 'in' ? tx.amount : -tx.amount, currency, {
                            useNativePrecision: false,
                            maxPrecision: currency?.fractionDigits ?? 2,
                          })}
                        </span>
                        {upcoming?.isOverdue ? <span className="text-xs text-destructive">{overdueNote(tx.spentAt)}</span> : null}
                      </span>
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

      {upcomingPreview && upcomingTemplate && upcomingPreview.isNext ? (
        <ViewRecurringDialog
          recurring={upcomingTemplate}
          onClose={() => setUpcomingPreview(null)}
          onPost={() => {
            // a payment due later this month is posted on its own day so it
            // stays where the list shows it; one due today or late is posted now
            const date = isFuture(upcomingPreview.spentAt) ? parseDateTime(upcomingPreview.spentAt) : new Date()
            postRecurring.mutate(recurringPostPayload(upcomingTemplate, accounts ?? [], exchangeFn, { date }), {
              onSuccess: () => setUpcomingPreview(null),
            })
          }}
          onSkip={() => {
            skipRecurring.mutate(upcomingTemplate.id, { onSuccess: () => setUpcomingPreview(null) })
          }}
          onEdit={() => {
            setUpcomingPreview(null)
            openRecurringModal({ recurring: upcomingTemplate })
          }}
          canChange
          skipPending={skipRecurring.isPending}
          postPending={postRecurring.isPending}
        />
      ) : upcomingPreview ? (
        <ViewRecurringDialog
          readOnly
          occurrence={upcomingPreview}
          onClose={() => setUpcomingPreview(null)}
          onEdit={
            upcomingTemplate
              ? () => {
                  setUpcomingPreview(null)
                  openRecurringModal({ recurring: upcomingTemplate })
                }
              : undefined
          }
          note={upcomingTemplate ? t('budgets.transactions.later_cycle') : undefined}
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
