import { useTranslation } from 'react-i18next'
import type { BudgetDebtElementDto } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import { isZero } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'
import { pluralPick } from '@/lib/plural'
import { debtExtraEffect } from './budgetMath'
import type { PayoffFigures } from './budgetMath'

/** "Paid off 14 months earlier, $21,000.00 less overpayment", or nothing */
export function DebtExtraNote({ row, currency }: { row: PayoffFigures; currency: CurrencyDto | undefined }) {
  const { t, i18n } = useTranslation()
  const effect = debtExtraEffect(row)
  if (!effect) {
    return null
  }
  const text = isZero(effect.saved)
    ? t('budgets.page.debts.extra_effect_earlier', { months: effect.months })
    : t('budgets.page.debts.extra_effect', { months: effect.months, amount: moneyFormat(effect.saved, currency) })
  return (
    <p className="text-xs text-muted-foreground" data-testid="debt-extra-effect">
      {pluralPick(text, effect.months, i18n.language)}
    </p>
  )
}

/** a debt row's Left: what is owed at the month's end, and how many payments remain */
export function DebtLeft({ row, amount }: { row: Pick<BudgetDebtElementDto, 'paymentsLeft' | 'isPaidOff'>; amount: string }) {
  const { t, i18n } = useTranslation()
  return (
    <span className="flex flex-col items-end leading-tight" title={t('budgets.page.debts.left_hint')}>
      <span>{amount}</span>
      <span className="text-[11px] text-muted-foreground" data-testid="debt-payments-left">
        {row.isPaidOff === 1 ? t('debts.list.paid_off') : pluralPick(t('budgets.page.debts.payments_left'), row.paymentsLeft, i18n.language)}
      </span>
    </span>
  )
}
