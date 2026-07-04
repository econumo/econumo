import { useTranslation } from 'react-i18next'
import { Loader2 } from 'lucide-react'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { moneyFormat } from '@/lib/money'
import { dayKey, formatDayHeading, isToday, isYesterday } from '@/lib/datetime'
import type { BudgetDto, BudgetElementDto } from '@/api/dto/budget'
import { BudgetElementType } from '@/api/dto/budget'
import { useCurrencies } from '@/features/currencies/queries'
import { useBudgetTransactions } from './queries'
import { useBudgetPeriodStore } from './budgetStore'

interface BudgetTransactionsDialogProps {
  budget: BudgetDto
  element: BudgetElementDto | null
  onClose: () => void
}

export function BudgetTransactionsDialog({ budget, element, onClose }: BudgetTransactionsDialogProps) {
  const { t } = useTranslation()
  const selectedDate = useBudgetPeriodStore((s) => s.selectedDate)
  const { data: currencies = [] } = useCurrencies()

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

  if (!element) {
    return null
  }

  const currency = currencies.find((c) => c.id === (element.currencyId ?? budget.meta.currencyId))

  let currentDay: string | null = null
  const rows: { kind: 'sep' | 'tx'; key: string; label?: string; tx?: NonNullable<typeof transactions>[number] }[] = []
  for (const tx of transactions ?? []) {
    const day = dayKey(tx.date)
    if (day !== currentDay) {
      currentDay = day
      const label = isToday(day)
        ? t('pages.account.transaction_list.today')
        : isYesterday(day)
          ? t('pages.account.transaction_list.yesterday')
          : formatDayHeading(day)
      rows.push({ kind: 'sep', key: `sep-${day}`, label })
    }
    rows.push({ kind: 'tx', key: tx.id, tx })
  }

  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={element.name}>
      {isLoading ? (
        <div className="flex justify-center py-6">
          <Loader2 className="size-6 animate-spin text-muted-foreground" aria-label="loading" />
        </div>
      ) : rows.length === 0 ? (
        <p className="py-2 text-sm text-muted-foreground">{t('blocks.list.list_empty')}</p>
      ) : (
        <div className="max-h-80 overflow-y-auto">
          {rows.map((row) =>
            row.kind === 'sep' ? (
              <p key={row.key} className="px-1 pb-1 pt-3 text-xs font-medium uppercase text-muted-foreground">
                {row.label}
              </p>
            ) : (
              <div key={row.key} className="flex items-center gap-2 px-1 py-1.5 text-sm" data-testid={`budget-tx-${row.key}`}>
                <EntityIcon name={element.icon} className="text-base text-muted-foreground" />
                <span className="min-w-0 flex-1 truncate">{row.tx!.description || element.name}</span>
                <span className="tabular-nums text-muted-foreground">
                  {moneyFormat(Math.abs(row.tx!.amount), currency, { showCurrency: false, useNativePrecision: false })}
                </span>
              </div>
            ),
          )}
        </div>
      )}
    </ResponsiveDialog>
  )
}
