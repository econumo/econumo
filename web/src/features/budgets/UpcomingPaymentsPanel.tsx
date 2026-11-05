import type { KeyboardEvent, MouseEvent, ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import { Repeat } from 'lucide-react'
import type { BudgetDto, BudgetTransactionDto, BudgetUpcomingDto } from '@/api/dto/budget'
import { BudgetElementType, isIncomeType } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { cmp, isZero, sub } from '@/lib/decimal'
import { dayKey, isFuture, parseDateTime } from '@/lib/datetime'
import { METRICS, trackEvent } from '@/lib/metrics'
import { moneyFormat, normalizeNumber } from '@/lib/money'
import { useCurrencies } from '@/features/currencies/queries'
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
  /** the cell's real money up to today (cell currency) */
  spent: string
  /** the cell's money still to come: recurring payments due and rows dated after today (cell currency); absent = 0 */
  upcoming?: string
}

// a savings row's figure is a net change, so a withdrawal counts against it
const signedAmount = (amount: string, row: BudgetTransactionDto, savings: boolean): string =>
  savings && row.direction === 'out' ? sub('0', amount) : amount

interface Row {
  key: string
  spentAt: string
  name: string
  /** what the row shows: its own currency, with that currency's symbol when it differs from the cell's */
  shown: string
  recurring?: BudgetUpcomingDto
}

/** What is still to come in the cell: due recurring payments and real rows dated later this month.
 *  Every figure carries a currency symbol: Spent and Total in the cell's currency, each row in its
 *  own (a row in another currency keeps its own amount). Pressing anywhere on it puts spent + still due (the total)
 *  into the editor. */
export function UpcomingPaymentsPanel({ query, onUse }: { query: UpcomingQuery; onUse?: (amount: string) => void }): ReactElement | null {
  const { t, i18n } = useTranslation()
  const { data } = useBudgetTransactions(budgetTransactionsParams(query.budget.meta.id, query.target, query.periodStart))
  const { data: currencies = [] } = useCurrencies()
  const currency = query.currency
  const savings = query.target.type === BudgetElementType.SAVINGS
  const nameOf = (row: BudgetTransactionDto) => row.description || row.payee?.name || row.category?.name || ''
  // without the cell's currency the rows could not be put side by side
  if (!currency) {
    return null
  }
  const cellAmount = (row: BudgetTransactionDto): string => {
    const amount = row.currencyId !== currency.id ? query.exchange(row.currencyId, currency.id, row.amount) : row.amount
    return signedAmount(amount, row, savings)
  }
  const numberOpts = { useNativePrecision: false, maxPrecision: currency.fractionDigits }
  const fmt = (v: string) => moneyFormat(v, currency, { ...numberOpts, showCurrency: true })
  const shownAmount = (row: BudgetTransactionDto): string => {
    const own = row.currencyId === currency.id ? undefined : currencies.find((c) => c.id === row.currencyId)
    if (!own) {
      return fmt(cellAmount(row))
    }
    return moneyFormat(signedAmount(row.amount, row, savings), own, { useNativePrecision: false, maxPrecision: own.fractionDigits, showCurrency: true })
  }
  const later = (data?.items ?? []).filter((r) => isFuture(r.spentAt))
  const rows: Row[] = [
    ...(data?.upcoming ?? []).map((r) => ({ key: `r:${r.templateId}:${r.spentAt}`, spentAt: r.spentAt, name: nameOf(r), shown: shownAmount(r), recurring: r })),
    ...later.map((r) => ({ key: `t:${r.id}`, spentAt: r.spentAt, name: nameOf(r), shown: shownAmount(r) })),
  ].sort((a, b) => a.spentAt.localeCompare(b.spentAt))
  if (rows.length === 0) {
    return null
  }
  const dateFmt = new Intl.DateTimeFormat(i18n.language, { day: 'numeric', month: 'short' })
  const need = planNeed(query.spent, query.upcoming)
  const totalLabel = t('budgets.page.upcoming.total')
  // spent holds what is behind us and upcoming the rows listed above, so the column adds up to the total
  const already = query.spent
  const alreadyLabel = savings
    ? t('budgets.page.upcoming.saved')
    : typeof query.target.type === 'number' && isIncomeType(query.target.type)
      ? t('budgets.page.upcoming.received')
      : t('budgets.page.upcoming.spent')
  const fill =
    onUse && cmp(need, '0') > 0
      ? () => {
          onUse(normalizeNumber(need))
          trackEvent(METRICS.BUDGET_PLAN_USE_UPCOMING)
        }
      : null
  const amountCell = 'text-right tabular-nums whitespace-nowrap'

  return (
    <div
      data-testid="upcoming-payments"
      className={`grid grid-cols-[auto_minmax(0,1fr)_auto] items-baseline gap-x-2 rounded text-xs text-muted-foreground ${fill ? 'cursor-pointer hover:bg-muted/60 focus-visible:outline focus-visible:outline-ring' : ''}`}
      {...(fill
        ? {
            role: 'button',
            tabIndex: 0,
            'aria-label': `${totalLabel} ${fmt(need)}`,
            // the editor's input keeps focus, so the value can be adjusted right away
            onMouseDown: (e: MouseEvent) => e.preventDefault(),
            onClick: fill,
            onKeyDown: (e: KeyboardEvent) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                fill()
              }
            },
          }
        : {})}
    >
      <p className="col-span-3 text-foreground">{t('budgets.page.upcoming.title')}</p>
      <ul className="col-span-3 grid grid-cols-subgrid">
        {rows.map((row) => {
          const date = dateFmt.format(parseDateTime(dayKey(row.spentAt)))
          const overdue = row.recurring?.isOverdue ?? false
          return (
            <li key={row.key} className="col-span-3 grid grid-cols-subgrid items-baseline">
              <span className={overdue ? 'text-destructive' : ''}>{overdue ? t('inbox.due.overdue', { date }) : date}</span>
              <span className="flex min-w-0 items-center gap-1">
                <span className="truncate">{row.name}</span>
                {row.recurring ? <Repeat className="size-3 shrink-0" aria-hidden /> : null}
              </span>
              <span className={amountCell}>{row.shown}</span>
            </li>
          )
        })}
      </ul>
      {!isZero(already) ? (
        <p className="col-span-3 grid grid-cols-subgrid text-foreground" data-testid="upcoming-spent">
          <span className="col-span-2">{alreadyLabel}</span>
          <span className={amountCell}>{fmt(already)}</span>
        </p>
      ) : null}
      <p className="col-span-3 grid grid-cols-subgrid mt-0.5 border-t border-border pt-0.5 text-foreground" data-testid="upcoming-total">
        <span className="col-span-2 text-right">{totalLabel}</span>
        <span className={amountCell}>{fmt(need)}</span>
      </p>
    </div>
  )
}
