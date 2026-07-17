import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2 } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { moneyFormat } from '@/lib/money'
import { dayKey, formatDayHeading, isFuture, isToday, isYesterday } from '@/lib/datetime'
import type { BudgetDto, BudgetTransactionDto } from '@/api/dto/budget'
import { BudgetElementType } from '@/api/dto/budget'
import type { CategoryDto } from '@/api/dto/category'
import type { Id } from '@/api/types'
import type { PayeeDto } from '@/api/dto/payee'
import type { TagDto } from '@/api/dto/tag'
import { useUiStore } from '@/app/uiStore'
import { queryKeys, TEN_MINUTES } from '@/app/queryKeys'
import * as transactionApi from '@/api/transaction'
import { useAccounts } from '@/features/accounts/queries'
import { useCategories, usePayees, useTags } from '@/features/classifications/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData } from '@/features/user/queries'
import { useDeleteTransaction, useTransactions } from '@/features/transactions/queries'
import type { ViewTransaction } from '@/features/transactions/useAccountTransactions'
import { ViewTransactionDialog } from '@/features/transactions/ViewTransactionDialog'
import { useBudgetTransactions } from './queries'
import { useBudgetPeriodStore } from './budgetStore'

/** enough of an element (or a nested child category) to list its transactions */
export interface BudgetTransactionsTarget {
  id: Id
  type: BudgetElementType
  name: string
  icon: string
  /** null = the budget base currency */
  currencyId: Id | null
}

// [monthStart, nextMonthStart) in the strict wire datetime format
function monthBounds(periodStart: string): { periodStart: string; periodEnd: string } {
  const [y, m] = periodStart.split('-').map(Number)
  const nextY = m === 12 ? y + 1 : y
  const nextM = m === 12 ? 1 : m + 1
  const pad = (n: number) => String(n).padStart(2, '0')
  return {
    periodStart: `${y}-${pad(m)}-01 00:00:00`,
    periodEnd: `${nextY}-${pad(nextM)}-01 00:00:00`,
  }
}

interface BudgetTransactionsDialogProps {
  budget: BudgetDto
  element: BudgetTransactionsTarget | null
  onClose: () => void
}

export function BudgetTransactionsDialog({ budget, element, onClose }: BudgetTransactionsDialogProps) {
  const { t, i18n } = useTranslation()
  const selectedDate = useBudgetPeriodStore((s) => s.selectedDate)
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

  const params = element
    ? {
        budgetId: budget.meta.id,
        periodStart: selectedDate,
        ...(element.type === BudgetElementType.CATEGORY ? { categoryId: element.id } : {}),
        ...(element.type === BudgetElementType.TAG ? { tagId: element.id } : {}),
        ...(element.type === BudgetElementType.ENVELOPE ? { envelopeId: element.id } : {}),
      }
    : null
  const { data: transactions, isLoading } = useBudgetTransactions(params)

  // The flat cache holds only windows now; older own rows in this budget month
  // may be outside them. Fetch the month once so editability detection keeps
  // working (not persisted — see queryPersist).
  const { data: monthTransactions } = useQuery({
    queryKey: queryKeys.transactionPeriod(selectedDate),
    queryFn: () => transactionApi.getTransactionList(monthBounds(selectedDate)).then((r) => r.items),
    enabled: element !== null,
    staleTime: TEN_MINUTES,
    gcTime: TEN_MINUTES,
  })

  if (!element) {
    return null
  }

  // full ViewTransaction when the caller can see the transaction in their own
  // list; otherwise (a partner's row in a shared budget) a read-only shape is
  // synthesized from the budget wire — enough for the preview, never editable
  const toViewTransaction = (wireTx: BudgetTransactionDto): ViewTransaction => {
    const tx = allTransactions?.find((item) => item.id === wireTx.id) ?? monthTransactions?.find((item) => item.id === wireTx.id)
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
      type: 'expense',
      accountId: '',
      accountRecipientId: null,
      amount: wireTx.amount,
      amountRecipient: null,
      categoryId: wireTx.category?.id ?? null,
      description: wireTx.description,
      payeeId: wireTx.payee?.id ?? null,
      tagId: wireTx.tag?.id ?? null,
      date: wireTx.spentAt,
      category: wireTx.category ? (wireTx.category as CategoryDto) : undefined,
      payee: wireTx.payee ? (wireTx.payee as PayeeDto) : undefined,
      tag: wireTx.tag ? (wireTx.tag as TagDto) : undefined,
      isInFuture: isFuture(wireTx.spentAt),
    }
  }

  const canChange = (tx: ViewTransaction): boolean => {
    const account = tx.account
    if (!account) {
      return false
    }
    const isOwner = account.owner.id === user?.id
    const myRole = account.sharedAccess.find((access) => access.user.id === user?.id)?.role
    if (!(isOwner || myRole === 'admin' || myRole === 'user')) {
      return false
    }
    if (tx.type === 'transfer') {
      return !!tx.account && !!tx.accountRecipient
    }
    return true
  }

  let currentDay: string | null = null
  const rows: { kind: 'sep' | 'tx'; key: string; label?: string; tx?: NonNullable<typeof transactions>[number] }[] = []
  for (const tx of transactions ?? []) {
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
      <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={element.name} dismissible={!preview && !deleteTarget}>
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
                  const currency = currencies.find((c) => c.id === tx.currencyId)
                  return (
                    <button
                      key={row.key}
                      type="button"
                      title={t('accounts.page.preview_transaction_modal.header')}
                      className="flex w-full items-center gap-2 rounded-md px-1 py-1.5 text-sm hover:bg-accent/50"
                      data-testid={`budget-tx-${row.key}`}
                      onClick={() => setPreview(toViewTransaction(tx))}
                    >
                      <EntityIcon name={tx.category?.icon || element.icon} className="text-base text-muted-foreground" />
                      <span className="flex min-w-0 flex-1 flex-col text-left">
                        <span className="truncate">{tx.description || tx.category?.name || element.name}</span>
                        {tx.description && (tx.category || tx.payee) ? (
                          <span className="truncate text-xs text-muted-foreground">
                            {[tx.category?.name, tx.payee?.name].filter(Boolean).join(' · ')}
                          </span>
                        ) : null}
                      </span>
                      <span className="tabular-nums text-muted-foreground">
                        {moneyFormat(-tx.amount, currency, { useNativePrecision: false, maxPrecision: currency?.fractionDigits ?? 2 })}
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
