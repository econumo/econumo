import { useTranslation } from 'react-i18next'
import type { BudgetDebtElementDto } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { Id } from '@/api/types'
import { useUiStore } from '@/app/uiStore'
import { debtMonth } from '@/features/debts/debtDates'
import { isZero } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'
import { pluralPick } from '@/lib/plural'
import { debtExtraEffect } from './budgetMath'
import type { PayoffFigures } from './budgetMath'
import { Dash } from './monthLines'

/** "Paid off 14 months earlier, $21,000.00 less overpayment", or nothing */
export function DebtExtraNote({ row, currency, className = 'text-xs text-muted-foreground' }: { row: PayoffFigures; currency: CurrencyDto | undefined; className?: string }) {
  const { t, i18n } = useTranslation()
  const effect = debtExtraEffect(row)
  if (!effect) {
    return null
  }
  const text = pluralPick(
    isZero(effect.saved)
      ? t('budgets.page.debts.extra_effect_earlier', { months: effect.months })
      : t('budgets.page.debts.extra_effect', { months: effect.months, amount: moneyFormat(effect.saved, currency) }),
    effect.months,
    i18n.language,
  )
  return (
    <p className={className} data-testid="debt-extra-effect" title={text}>
      {text}
    </p>
  )
}

/** A Plan row's payoff under its name: the projected "Paid off by Mar 2028", or
 *  "Paid off" once it is. What the planned extras buy shows only in the plan editor. */
export function DebtPayoff({ debt }: { debt: PayoffFigures & Pick<BudgetDebtElementDto, 'isPaidOff'> }) {
  const { t, i18n } = useTranslation()
  const payoff =
    debt.isPaidOff === 1
      ? t('debts.list.paid_off')
      : debt.payoffDate !== ''
        ? t('budgets.page.debts.payoff_by', { date: debtMonth(debt.payoffDate, i18n.language) })
        : null
  if (payoff === null) {
    return null
  }
  return (
    <span className="min-w-0 truncate text-[11px] text-muted-foreground" data-testid="debt-payoff" title={payoff}>
      {payoff}
    </span>
  )
}

/** a debt row's Left: what is owed at the month's end, and how many payments remain.
 *  A debt not set up owes nothing it knows of: its owner is offered Finish setup
 *  (the debt's review screen), anyone else a dash. */
export function DebtLeft({
  row,
  amount,
  userId,
}: {
  row: Pick<BudgetDebtElementDto, 'id' | 'paymentsLeft' | 'isPaidOff' | 'isSetUp' | 'ownerUserId'>
  amount: string
  userId?: Id
}) {
  const { t, i18n } = useTranslation()
  const openDebtDetails = useUiStore((s) => s.openDebtDetails)
  if (row.isSetUp === 0) {
    return !!userId && row.ownerUserId === userId ? (
      <button type="button" className="text-econumo-link hover:underline" onClick={() => openDebtDetails(row.id, 'settings')}>
        {t('debts.terms.finish_setup')}
      </button>
    ) : (
      <Dash />
    )
  }
  return (
    <span className="flex flex-col items-end leading-tight" title={t('budgets.page.debts.left_hint')}>
      <span>{amount}</span>
      <span className="text-[11px] text-muted-foreground" data-testid="debt-payments-left">
        {row.isPaidOff === 1 ? t('debts.list.paid_off') : pluralPick(t('budgets.page.debts.payments_left'), row.paymentsLeft, i18n.language)}
      </span>
    </span>
  )
}
