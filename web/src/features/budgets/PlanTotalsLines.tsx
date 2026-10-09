import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { CurrencyDto } from '@/api/dto/currency'
import { cmp, isZero } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'
import { PLAN_SECTION_RULE } from './monthLayout'
import { Dash, TotalLine } from './monthLines'
import type { PlanMonthTotals } from './planMath'

interface MonthColumns {
  visibleMonths: string[]
  monthIndex: (m: string) => number
  currency: CurrencyDto | undefined
}

const formatter = (currency: CurrencyDto | undefined) => (v: string) => moneyFormat(v, currency, { showCurrency: false, useNativePrecision: false })

function PlanTotalRow(props: Parameters<typeof TotalLine>[0]) {
  return (
    <div role="row">
      <TotalLine {...props} />
    </div>
  )
}

/** one figure per visible month, a dash for a month the plan has no data for */
function monthValues({ visibleMonths, monthIndex, currency }: MonthColumns, values: (idx: number) => string | undefined, testId: string): ReactNode[] {
  const fmt = formatter(currency)
  return visibleMonths.map((m, i) => {
    const idx = monthIndex(m)
    const value = idx >= 0 ? values(idx) : undefined
    return (
      <span key={m} data-testid={`${testId}-${i}`}>
        {value !== undefined ? fmt(value) : <Dash />}
      </span>
    )
  })
}

// Uncategorized is no totals line: it is a row inside the income and expense
// sections, and Income/Expenses already carry its actual. Net is gone too: the
// Balance line chains on effectiveNet, so it says the same thing.
export function PlanTotals({
  totals,
  showSavings,
  savingsBalance,
  onTransfersClick,
  ...cols
}: MonthColumns & {
  totals: PlanMonthTotals[]
  /** the budget has savings rows: a Savings line */
  showSavings: boolean
  /** the savings side of the balance split, or null when the plan holds no savings money */
  savingsBalance: string[] | null
  /** the month's list of transfers that crossed the budget boundary */
  onTransfersClick: (month: string) => void
}) {
  const { t } = useTranslation()
  const fmt = formatter(cols.currency)
  const at = (idx: number) => totals[idx] as PlanMonthTotals | undefined
  const visibleTotals = cols.visibleMonths.map((m) => at(cols.monthIndex(m))).filter((row) => row !== undefined)
  const showTransfers = visibleTotals.some((row) => !isZero(row.transfersIn) || !isZero(row.transfersOut))
  const transferValues = cols.visibleMonths.map((m, i) => {
    const row = at(cols.monthIndex(m))
    if (!row) {
      return <Dash key={m} />
    }
    // a net that only cancels out (in == out != 0) still has transfers to list
    if (isZero(row.transfersIn) && isZero(row.transfersOut)) {
      return (
        <span key={m} data-testid={`plan-totals-transfers-${i}`}>
          {fmt(row.transfersNet)}
        </span>
      )
    }
    const label = t('budgets.page.plan.totals.transfers')
    return (
      <button
        key={m}
        type="button"
        title={`${t('budgets.page.plan.totals.transfers_tooltip', { in: fmt(row.transfersIn), out: fmt(row.transfersOut) })}. ${t('budgets.page.plan.totals.show_transactions')}`}
        aria-label={`transactions ${label} ${m}`}
        data-testid={`plan-totals-transfers-link-${i}`}
        className="tabular-nums underline-offset-2 hover:underline"
        onClick={() => onTransfersClick(m)}
      >
        {fmt(row.transfersNet)}
      </button>
    )
  })
  return (
    <div role="rowgroup" className={`flex flex-col ${PLAN_SECTION_RULE}`} data-testid="plan-totals">
      <PlanTotalRow
        testId="plan-total-income"
        label={t('budgets.page.plan.totals.income')}
        values={monthValues(cols, (idx) => at(idx)?.effectiveIncome, 'plan-totals-income')}
        actionsColumn={false}
      />
      <PlanTotalRow
        testId="plan-total-expenses"
        label={t('budgets.page.plan.totals.expenses')}
        values={monthValues(cols, (idx) => at(idx)?.effectiveExpense, 'plan-totals-expenses')}
        actionsColumn={false}
      />
      {showTransfers ? (
        <PlanTotalRow testId="plan-total-transfers" label={t('budgets.page.plan.totals.transfers')} values={transferValues} actionsColumn={false} />
      ) : null}
      {showSavings ? (
        <PlanTotalRow
          testId="plan-total-savings"
          label={t('budgets.page.plan.totals.savings')}
          values={monthValues(cols, (idx) => at(idx)?.effectiveSavings, 'plan-totals-savings')}
          actionsColumn={false}
        />
      ) : null}
      {savingsBalance ? (
        <PlanTotalRow
          testId="plan-total-savings-balance"
          label={t('budgets.page.plan.totals.savings_balance')}
          values={monthValues(cols, (idx) => savingsBalance[idx], 'plan-savings-balance')}
          actionsColumn={false}
        />
      ) : null}
    </div>
  )
}

/** The Balance line, pinned to the bottom of the grid. With savings money in the
 *  plan it is the everyday part, and Total savings above holds the rest. */
export function PlanBalanceRow({ balance, ...cols }: MonthColumns & { balance: string[] }) {
  const { t } = useTranslation()
  const negative = cols.visibleMonths.map((m) => {
    const value = balance[cols.monthIndex(m)]
    return value !== undefined && cmp(value, '0') < 0
  })
  return (
    <div role="rowgroup" className="sticky bottom-0 z-10 border-t bg-background" data-testid="plan-balance-row">
      <PlanTotalRow
        testId="plan-total-balance"
        label={t('budgets.page.plan.totals.balance')}
        values={monthValues(cols, (idx) => balance[idx], 'plan-balance')}
        strong
        negative={negative}
        actionsColumn={false}
      />
    </div>
  )
}
