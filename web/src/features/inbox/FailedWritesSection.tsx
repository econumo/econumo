import { skipToken, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { BudgetDto } from '@/api/dto/budget'
import type { TransactionDto } from '@/api/dto/transaction'
import type { Id } from '@/api/types'
import { useUiStore } from '@/app/uiStore'
import { queryKeys } from '@/app/queryKeys'
import { Button } from '@/components/ui/button'
import { discardWrite, retryWrite } from '@/features/pendingWrites/submit'
import type { PendingOp } from '@/features/pendingWrites/types'
import { enrichTransaction, useTransactionLookups } from '@/features/transactions/useAccountTransactions'
import { useTransactions } from '@/features/transactions/queries'
import { TransactionRow } from '@/features/transactions/TransactionRow'
import { useUserData } from '@/features/user/queries'
import { moneyFormat } from '@/lib/money'

function elementName(budgets: [unknown, BudgetDto | undefined][], elementId: Id): string | undefined {
  for (const [, budget] of budgets) {
    const { elements, savings } = budget?.structure ?? { elements: [] }
    for (const el of elements) {
      if (el.id === elementId) return el.name
      const child = el.children.find((c) => c.id === elementId)
      if (child) return child.name
    }
    const saved = savings?.find((s) => s.id === elementId)
    if (saved) return saved.name
  }
  return undefined
}

function periodLabel(period: string, lang: string): string {
  const [year, month] = period.split('-').map(Number)
  return new Intl.DateTimeFormat(lang, { month: 'long', year: 'numeric' }).format(new Date(year, month - 1, 1))
}

function LimitText({ op }: { op: Extract<PendingOp, { kind: 'limit' }> }) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  // read-only subscription to the budget the user was on, so the name appears when it arrives
  const { data: current } = useQuery<BudgetDto>({ queryKey: [...queryKeys.budget, op.budgetId, op.period], queryFn: skipToken })
  const cached = queryClient.getQueriesData<BudgetDto>({ queryKey: [...queryKeys.budget, op.budgetId] })
  const name = elementName([[null, current], ...cached], op.elementId)
  const period = periodLabel(op.period, i18n.language)
  if (op.amount === null) {
    return t(name ? 'inbox.failed_writes.limit_cleared' : 'inbox.failed_writes.limit_cleared_generic', { name, period })
  }
  const amount = moneyFormat(op.amount, null, { showCurrency: false, useNativePrecision: false })
  return t(name ? 'inbox.failed_writes.limit' : 'inbox.failed_writes.limit_generic', { name, amount, period })
}

export function FailedWritesSection({ items }: { items: PendingOp[] }) {
  const { t } = useTranslation()
  const { data: transactions } = useTransactions()
  const lookups = useTransactionLookups()
  const { data: me } = useUserData()
  const openTransactionModal = useUiStore((s) => s.openTransactionModal)

  const kindNote = (op: PendingOp) =>
    op.kind === 'tx-create' ? t('inbox.failed_writes.kind_create') : op.kind === 'tx-update' ? t('inbox.failed_writes.kind_update') : t('inbox.failed_writes.kind_delete')

  const main = (op: PendingOp) => {
    if (op.kind === 'limit') {
      return <span className="block px-4 py-3 text-sm"><LimitText op={op} /></span>
    }
    const stored = op.kind === 'tx-delete' ? op.previous : transactions?.find((tx) => tx.id === op.txId)
    const row: TransactionDto | null =
      stored ?? (op.kind !== 'tx-delete' && me ? { ...op.payload, author: me, recurringId: null, isImported: 0 } : null)
    if (!row) {
      return <span className="block px-4 py-3 text-sm">{op.kind === 'tx-delete' ? t('inbox.failed_writes.delete_generic') : kindNote(op)}</span>
    }
    const view = <TransactionRow transaction={enrichTransaction(row, lookups)} dimmed={false} titleNote={kindNote(op)} />
    if (op.kind === 'tx-delete') {
      return view
    }
    return (
      <button type="button" className="block w-full text-left hover:bg-econumo-hover" onClick={() => openTransactionModal({ transaction: row })}>
        {view}
      </button>
    )
  }

  return (
    <div className="flex flex-col gap-2">
      {items.map((op) => (
        <div key={op.key} data-testid={`failed-write-${op.key}`} className="overflow-hidden rounded-lg bg-econumo-card">
          {main(op)}
          <p className="px-4 pb-2 text-xs text-destructive">{op.error || t('inbox.failed_writes.reason_unknown')}</p>
          <div className="flex justify-end gap-2 px-4 pb-3">
            <Button type="button" size="sm" variant="ghost" onClick={() => discardWrite(op.key)}>{t('inbox.failed_writes.discard')}</Button>
            {op.kind === 'tx-delete' || op.kind === 'limit' ? (
              <Button type="button" size="sm" onClick={() => retryWrite(op.key)}>{t('inbox.failed_writes.retry')}</Button>
            ) : null}
          </div>
        </div>
      ))}
    </div>
  )
}
