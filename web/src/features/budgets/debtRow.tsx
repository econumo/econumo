import { useTranslation } from 'react-i18next'
import type { ElementDebtDto } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import { useUiStore } from '@/app/uiStore'
import { debtMonth } from '@/features/debts/debtDates'
import { isZero } from '@/lib/decimal'
import { moneyFormat } from '@/lib/money'
import { pluralPick } from '@/lib/plural'
import { debtExtraEffect } from './budgetMath'
import type { PayoffFigures } from './budgetMath'
import { useDebtSummary } from './debtSummary'

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
export function DebtPayoff({ debt }: { debt: Pick<ElementDebtDto, 'payoffDate' | 'isPaidOff'> }) {
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

/** The quiet line under a row carrying a debt: what is left and when it is paid
 *  off, opening the debt's review. A debt not set up offers whoever plans in the
 *  budget Finish setup (the review on Settings) and shows nothing to anyone else. */
export function ElementDebtLine({ debt, canPlan, className = '' }: { debt: ElementDebtDto; canPlan: boolean; className?: string }) {
  const { t } = useTranslation()
  const openDebtDetails = useUiStore((s) => s.openDebtDetails)
  const summary = useDebtSummary()(debt)
  if (summary === null && !canPlan) {
    return null
  }
  return (
    <div className={`text-[11px] leading-tight text-muted-foreground ${className}`} data-testid="element-debt-line">
      {summary === null ? (
        <button type="button" className="text-econumo-link hover:underline" onClick={() => openDebtDetails(debt.id, 'settings')}>
          {t('debts.terms.finish_setup')}
        </button>
      ) : (
        <button
          type="button"
          className="max-w-full truncate text-left underline-offset-2 hover:text-foreground hover:underline"
          title={t('budgets.page.debts.details')}
          onClick={() => openDebtDetails(debt.id, 'overview')}
        >
          {summary}
        </button>
      )}
    </div>
  )
}
