import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueries } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import * as budgetApi from '@/api/budget'
import type { Id } from '@/api/types'
import { queryKeys } from '@/app/queryKeys'
import { useUiStore } from '@/app/uiStore'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useAccounts } from '@/features/accounts/queries'
import { budgetTransactionView } from '@/features/budgets/budgetTransactionView'
import { addMonths, monthDiff } from '@/features/budgets/planMath'
import { useCategories, usePayees, useTags } from '@/features/classifications/queries'
import { canWriteToAccount } from '@/features/connections/shared'
import { useCurrencies } from '@/features/currencies/queries'
import { canTouchTransaction } from '@/features/transactions/canTouchTransaction'
import { compareTransactions } from '@/features/transactions/order'
import { useDeleteTransaction, useTransactions } from '@/features/transactions/queries'
import { TransactionRow } from '@/features/transactions/TransactionRow'
import { enrichTransaction, groupByDay, separatorText, useTransactionLookups } from '@/features/transactions/useAccountTransactions'
import type { ViewTransaction } from '@/features/transactions/useAccountTransactions'
import { ViewTransactionDialog } from '@/features/transactions/ViewTransactionDialog'
import { useUserData } from '@/features/user/queries'
import { dayKey } from '@/lib/datetime'
import { usePeriodLabels } from './usePeriodLabels'

export interface AccountsDrill {
  /** one account, or `accounts` for every transaction touching accountIds */
  id: Id
  name: string
  /** 'savings': a budget's savings member, listed by the budget, a partner's too */
  kind: 'account' | 'savings' | 'accounts'
  accountIds?: Id[]
}

function monthList(from: string, to: string): string[] {
  return Array.from({ length: Math.max(0, monthDiff(from, to) + 1) }, (_, i) => addMonths(from, i))
}

function useBudgetRows(budgetId: Id | null, accountId: Id | null, from: string, to: string) {
  const results = useQueries({
    queries: (budgetId && accountId ? monthList(from, to) : []).map((periodStart) => {
      const params: budgetApi.BudgetTransactionsParams = { budgetId: budgetId!, periodStart, accountId: accountId! }
      return {
        queryKey: [...queryKeys.budgetTransactions, params],
        queryFn: () => budgetApi.getBudgetTransactions(params),
      }
    }),
  })
  const { data: transactions } = useTransactions()
  const { data: accounts } = useAccounts()
  const { data: categories } = useCategories()
  const { data: payees } = usePayees()
  const { data: tags } = useTags()
  const wire = results.flatMap((r) => r.data?.items ?? [])
  const rows = wire
    .map((w) => ({ view: budgetTransactionView(w, { transactions, accounts, categories, payees, tags }), currencyId: w.currencyId }))
    .sort((a, b) => (a.view.date < b.view.date ? 1 : a.view.date > b.view.date ? -1 : 0))
  return {
    isLoading: results.some((r) => r.isLoading),
    // one month missing would pass for a complete list, offline as much as after an error
    isError: results.some((r) => !r.data && (r.isError || r.fetchStatus === 'paused')),
    rows,
  }
}

function useLocalRows(enabled: boolean, row: AccountsDrill, from: string, to: string) {
  const { data: transactions, isLoading, isError } = useTransactions()
  const lookups = useTransactionLookups()
  const rows = useMemo(() => {
    if (!enabled) return []
    const first = from.slice(0, 7)
    const last = to.slice(0, 7)
    const ids = row.kind === 'accounts' ? (row.accountIds ?? []) : [row.id]
    return (transactions ?? [])
      .filter((tx) => {
        const month = tx.date.slice(0, 7)
        if (month < first || month > last) return false
        // either side, whoever owns the account
        return ids.includes(tx.accountId) || (tx.accountRecipientId !== null && ids.includes(tx.accountRecipientId))
      })
      // the account page's order: day, then the account's own position within the day
      .sort((a, b) => compareTransactions(a, b, row.kind === 'account' ? row.id : undefined))
      .map((tx) => ({ view: enrichTransaction(tx, lookups), currencyId: undefined as Id | undefined }))
  }, [enabled, transactions, lookups, row, from, to])
  return { isLoading: enabled && isLoading, isError: enabled && isError && !transactions, rows }
}

export function ReportTransactionsDialog({ budgetId, row, from, to, onClose }: {
  budgetId: Id | null
  row: AccountsDrill
  from: string
  to: string
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const labels = usePeriodLabels()
  const fromBudget = row.kind === 'savings'
  const budgetRows = useBudgetRows(fromBudget ? budgetId : null, fromBudget ? row.id : null, from, to)
  const localRows = useLocalRows(!fromBudget, row, from, to)
  const { isLoading, isError, rows } = fromBudget ? budgetRows : localRows
  const { data: user } = useUserData()
  const { data: currencies = [] } = useCurrencies()
  const deleteTransaction = useDeleteTransaction()
  const openTransactionModal = useUiStore((s) => s.openTransactionModal)
  const [preview, setPreview] = useState<ViewTransaction | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ViewTransaction | null>(null)

  useEffect(() => {
    // a transaction deleted from the preview leaves the list
    if (preview && !rows.some((r) => r.view.id === preview.id)) setPreview(null)
  }, [rows, preview])

  const entries = groupByDay(rows.map((r) => ({ tx: r.view, groupDay: dayKey(r.view.date) })))
  const canChange = (tx: ViewTransaction) => canTouchTransaction(tx, !!tx.account && canWriteToAccount(tx.account, user?.id))
  const title = `${row.name} · ${labels.range([from, to])}`

  return (
    <>
      <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={title} dismissible={!preview && !deleteTarget}>
        {isError ? (
          <p className="py-2 text-sm text-muted-foreground">{t('common.app.error')}</p>
        ) : isLoading ? (
          <div className="flex justify-center py-6">
            <Loader2 className="size-6 animate-spin text-muted-foreground" aria-label="loading" />
          </div>
        ) : entries.length === 0 ? (
          <p className="py-2 text-sm text-muted-foreground">{t('common.list.list_empty')}</p>
        ) : (
          <div className="max-h-[60vh] overflow-y-auto scrollbar-slim">
            {entries.map((entry) =>
              entry.kind === 'separator' ? (
                <p key={`sep-${entry.day}`} className="px-2 pb-1 pt-3 text-xs uppercase text-muted-foreground">
                  {separatorText(entry, t, i18n.language)}
                </p>
              ) : (
                <button
                  key={entry.transaction.id}
                  type="button"
                  className="block w-full rounded-md text-left hover:bg-econumo-hover"
                  onClick={() => setPreview(entry.transaction)}
                >
                  <TransactionRow transaction={entry.transaction} />
                </button>
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
          onDelete={() => setDeleteTarget(preview)}
          canChange={canChange(preview)}
          isShared={(preview.account?.sharedAccess.length ?? 0) > 0 || !preview.account}
          dismissible={deleteTarget === null}
          fallbackCurrency={currencies.find((c) => c.id === rows.find((r) => r.view.id === preview.id)?.currencyId)}
        />
      ) : null}

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) {
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
