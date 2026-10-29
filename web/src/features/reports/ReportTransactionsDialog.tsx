import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueries } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import * as budgetApi from '@/api/budget'
import type { ReportScope } from '@/api/dto/report'
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
import { add, cmp } from '@/lib/decimal'
import { usePeriodLabels } from './usePeriodLabels'
import { budgetSelector, matches } from './reportRows'
import type { DrillRow } from './reportRows'

function monthList(from: string, to: string): string[] {
  return Array.from({ length: Math.max(0, monthDiff(from, to) + 1) }, (_, i) => addMonths(from, i))
}

function useBudgetRows(budgetId: Id | null, row: DrillRow, from: string, to: string) {
  const selector = budgetId ? budgetSelector(row) : null
  const results = useQueries({
    queries: (selector && budgetId ? monthList(from, to) : []).map((periodStart) => {
      const params: budgetApi.BudgetTransactionsParams = { budgetId: budgetId!, periodStart, ...selector }
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
    supported: selector !== null,
    isLoading: results.some((r) => r.isLoading),
    rows,
  }
}

function useMyRows(enabled: boolean, row: DrillRow, from: string, to: string) {
  const { data: transactions, isLoading } = useTransactions()
  const { data: user } = useUserData()
  const lookups = useTransactionLookups()
  const rows = useMemo(() => {
    if (!enabled || !user) return []
    const owned = new Set((lookups.accounts ?? []).filter((a) => a.owner.id === user.id).map((a) => a.id))
    const first = from.slice(0, 7)
    const last = to.slice(0, 7)
    return (transactions ?? [])
      .filter((tx) => {
        const month = tx.date.slice(0, 7)
        if (month < first || month > last) return false
        // an account's own list: every transaction on either side, whoever owns it
        if (row.kind === 'account') return tx.accountId === row.id || tx.accountRecipientId === row.id
        if (tx.type !== row.side || !owned.has(tx.accountId)) return false
        switch (row.kind) {
          case 'category':
          case 'income_category':
            return matches(tx.categoryId, row.id)
          case 'tag':
            return matches(tx.tagId, row.id)
          case 'payee':
            return matches(tx.payeeId, row.id)
          default:
            return false
        }
      })
      // the account page's order: day, then the account's own position within the day
      .sort((a, b) => compareTransactions(a, b, row.kind === 'account' ? row.id : undefined))
      .map((tx) => ({ view: enrichTransaction(tx, lookups), currencyId: undefined as Id | undefined }))
  }, [enabled, user, transactions, lookups, row, from, to])
  return { supported: true, isLoading: enabled && isLoading, rows }
}

// My-accounts rows come from the local transaction list, which holds no
// deleted account, while the report still counts them. The two only compare
// when every listed row is in the report's currency.
function missesDeleted(rows: { view: ViewTransaction }[], expected: Expected | undefined): boolean {
  if (!expected) return false
  if (rows.some((r) => r.view.account?.currency.id !== expected.currencyId)) return false
  return cmp(rows.reduce((acc, r) => add(acc, r.view.amount), '0'), expected.amount) !== 0
}

export interface Expected {
  /** the report's figure for the row over the range */
  amount: string
  currencyId: Id
}

export function ReportTransactionsDialog({ scope, row, from, to, expected, onClose }: {
  scope: ReportScope
  row: DrillRow
  from: string
  to: string
  expected?: Expected
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const labels = usePeriodLabels()
  // an account's transactions come from the local list in either scope
  const local = scope.budgetId === null || row.kind === 'account'
  const budgetRows = useBudgetRows(local ? null : scope.budgetId, row, from, to)
  const myRows = useMyRows(local, row, from, to)
  const { supported, isLoading, rows } = local ? myRows : budgetRows
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
  const deletedNote = local && !isLoading && missesDeleted(rows, expected)

  return (
    <>
      <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={title} dismissible={!preview && !deleteTarget}>
        {!supported ? (
          <p className="py-2 text-sm text-muted-foreground">{t('reports.drill.unsupported')}</p>
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
        {deletedNote ? <p className="pt-2 text-xs text-muted-foreground">{t('reports.drill.deleted_accounts')}</p> : null}
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
