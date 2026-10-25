import type { ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import { Repeat } from 'lucide-react'
import type { BudgetDto, BudgetTransactionDto, BudgetUpcomingDto } from '@/api/dto/budget'
import { BudgetElementType } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { cmp, sub } from '@/lib/decimal'
import { dayKey, isFuture, parseDateTime } from '@/lib/datetime'
import { METRICS, trackEvent } from '@/lib/metrics'
import { moneyFormat, normalizeNumber } from '@/lib/money'
import type { BudgetTransactionsTarget } from './BudgetTransactionsDialog'
import { budgetTransactionsParams } from './budgetTransactionsParams'
import { planNeed } from './planHint'
import { useBudgetTransactions } from './queries'

/** the cell whose still-to-come money a plan editor lists */
export interface UpcomingQuery {
  budget: BudgetDto | { meta: { id: Id } }
  target: BudgetTransactionsTarget
  periodStart: string
  currency: CurrencyDto | undefined
  /** at the month's budget rates; each row is in its account's currency, the cell in `currency` */
  exchange: (fromCurrencyId: Id, toCurrencyId: Id, amount: string) => string
  /** the cell's real money so far, future-dated rows included (cell currency) */
  spent: string
  /** the cell's still-due recurring money (cell currency); absent = 0 */
  upcoming?: string
}

// a savings row's figure is a net change, so a withdrawal counts against it
const signedAmount = (amount: string, row: BudgetTransactionDto, savings: boolean): string =>
  savings && row.direction === 'out' ? sub('0', amount) : amount

interface Row {
  key: string
  spentAt: string
  name: string
  amount: string
  recurring?: BudgetUpcomingDto
}

/** What is still to come in the cell: due recurring payments and real rows dated later this month.
 *  Pressing "Use" puts spent + still due into the editor. */
export function UpcomingPaymentsPanel({ query, onUse }: { query: UpcomingQuery; onUse?: (amount: string) => void }): ReactElement | null {
  const { t, i18n } = useTranslation()
  const { data } = useBudgetTransactions(budgetTransactionsParams(query.budget.meta.id, query.target, query.periodStart))
  const savings = query.target.type === BudgetElementType.SAVINGS
  const cellAmount = (row: BudgetTransactionDto): string => {
    const amount = query.currency && row.currencyId !== query.currency.id ? query.exchange(row.currencyId, query.currency.id, row.amount) : row.amount
    return signedAmount(amount, row, savings)
  }
  const nameOf = (row: BudgetTransactionDto) => row.description || row.payee?.name || row.category?.name || ''
  const rows: Row[] = [
    ...(data?.upcoming ?? []).map((r) => ({ key: `r:${r.templateId}:${r.spentAt}`, spentAt: r.spentAt, name: nameOf(r), amount: cellAmount(r), recurring: r })),
    ...(data?.items ?? []).filter((r) => isFuture(r.spentAt)).map((r) => ({ key: `t:${r.id}`, spentAt: r.spentAt, name: nameOf(r), amount: cellAmount(r) })),
  ].sort((a, b) => a.spentAt.localeCompare(b.spentAt))
  if (rows.length === 0) {
    return null
  }
  const fmt = (v: string) =>
    moneyFormat(v, query.currency, { showCurrency: false, useNativePrecision: false, maxPrecision: query.currency?.fractionDigits ?? 2 })
  const dateFmt = new Intl.DateTimeFormat(i18n.language, { day: 'numeric', month: 'short' })
  const need = planNeed(query.spent, query.upcoming)
  const useLabel = t('budgets.page.upcoming.use', { amount: fmt(need) })

  return (
    <div data-testid="upcoming-payments" className="flex flex-col gap-0.5 text-xs text-muted-foreground">
      <p>{t('budgets.page.upcoming.title')}</p>
      <ul className="flex flex-col">
        {rows.map((row) => {
          const date = dateFmt.format(parseDateTime(dayKey(row.spentAt)))
          const overdue = row.recurring?.isOverdue ?? false
          return (
            <li key={row.key} className="flex items-baseline gap-2">
              <span className={`shrink-0 ${overdue ? 'text-destructive' : ''}`}>{overdue ? t('inbox.due.overdue', { date }) : date}</span>
              <span className="flex min-w-0 flex-1 items-center gap-1">
                <span className="truncate">{row.name}</span>
                {row.recurring ? <Repeat className="size-3 shrink-0" aria-hidden /> : null}
              </span>
              <span className="tabular-nums">{fmt(row.amount)}</span>
            </li>
          )
        })}
      </ul>
      {cmp(query.spent, '0') > 0 ? (
        <p className="text-right tabular-nums">{t('budgets.page.upcoming.spent', { amount: fmt(query.spent) })}</p>
      ) : null}
      {onUse && cmp(need, '0') > 0 ? (
        <button
          type="button"
          aria-label={useLabel}
          className="self-end rounded px-1 tabular-nums text-foreground/70 underline-offset-2 hover:text-foreground hover:underline"
          // the editor's input keeps focus, so the value can be adjusted right away
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => {
            onUse(normalizeNumber(need))
            trackEvent(METRICS.BUDGET_PLAN_USE_UPCOMING)
          }}
        >
          {useLabel}
        </button>
      ) : null}
    </div>
  )
}
