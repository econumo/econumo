import type { ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import type { BudgetDto, BudgetUpcomingDto } from '@/api/dto/budget'
import { BudgetElementType } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { add, sub } from '@/lib/decimal'
import { dayKey, parseDateTime } from '@/lib/datetime'
import { moneyFormat } from '@/lib/money'
import type { BudgetTransactionsTarget } from './BudgetTransactionsDialog'
import { budgetTransactionsParams } from './budgetTransactionsParams'
import { useBudgetTransactions } from './queries'

/** the cell whose still-due recurring payments a plan editor lists */
export interface UpcomingQuery {
  budget: BudgetDto | { meta: { id: Id } }
  target: BudgetTransactionsTarget
  periodStart: string
  currency: CurrencyDto | undefined
  /** at the month's budget rates; each row is in its account's currency, the cell in `currency` */
  exchange: (fromCurrencyId: Id, toCurrencyId: Id, amount: string) => string
}

// a savings row's figure is a net change, so a withdrawal counts against it
const signedAmount = (amount: string, row: BudgetUpcomingDto, savings: boolean): string =>
  savings && row.direction === 'out' ? sub('0', amount) : amount

/** Read-only: the payments the grey plan suggestion is made of. */
export function UpcomingPaymentsPanel({ query }: { query: UpcomingQuery }): ReactElement | null {
  const { t, i18n } = useTranslation()
  const { data } = useBudgetTransactions(budgetTransactionsParams(query.budget.meta.id, query.target, query.periodStart))
  const rows = data?.upcoming ?? []
  if (rows.length === 0) {
    return null
  }
  const savings = query.target.type === BudgetElementType.SAVINGS
  const fmt = (v: string) =>
    moneyFormat(v, query.currency, { showCurrency: false, useNativePrecision: false, maxPrecision: query.currency?.fractionDigits ?? 2 })
  const dateFmt = new Intl.DateTimeFormat(i18n.language, { day: 'numeric', month: 'short' })
  const cellAmount = (row: BudgetUpcomingDto): string => {
    const amount = query.currency && row.currencyId !== query.currency.id ? query.exchange(row.currencyId, query.currency.id, row.amount) : row.amount
    return signedAmount(amount, row, savings)
  }
  const total = rows.reduce((sum, row) => add(sum, cellAmount(row)), '0')

  return (
    <div data-testid="upcoming-payments" className="flex flex-col gap-1 text-sm">
      <p className="text-xs text-muted-foreground">{t('budgets.page.upcoming.title')}</p>
      <ul className="flex flex-col gap-0.5">
        {rows.map((row) => {
          const date = dateFmt.format(parseDateTime(dayKey(row.spentAt)))
          return (
            <li key={`${row.templateId}:${row.spentAt}`} className="flex items-baseline gap-2">
              <span className={`shrink-0 text-xs ${row.isOverdue ? 'text-destructive' : 'text-muted-foreground'}`}>
                {row.isOverdue ? t('inbox.due.overdue', { date }) : date}
              </span>
              <span className="min-w-0 flex-1 truncate">{row.description || row.payee?.name || row.category?.name}</span>
              <span className="tabular-nums">{fmt(cellAmount(row))}</span>
            </li>
          )
        })}
      </ul>
      <p className="text-right text-xs text-muted-foreground tabular-nums">{t('budgets.page.upcoming.total', { amount: fmt(total) })}</p>
    </div>
  )
}
