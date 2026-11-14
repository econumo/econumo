import { useTranslation } from 'react-i18next'
import type { ElementDebtDto } from '@/api/dto/budget'
import { debtMonth } from '@/features/debts/debtDates'
import { pluralPick } from '@/lib/plural'

/** "24 payments left · Paid off by Oct 2028", "Paid off", or null for a debt not set up */
export function useDebtSummary() {
  const { t, i18n } = useTranslation()
  return (debt: ElementDebtDto): string | null => {
    if (debt.isSetUp === 0) {
      return null
    }
    if (debt.isPaidOff === 1) {
      return t('debts.list.paid_off')
    }
    const left = pluralPick(t('budgets.page.debts.payments_left'), debt.paymentsLeft, i18n.language)
    return debt.payoffDate === '' ? left : `${left} · ${t('budgets.page.debts.payoff_by', { date: debtMonth(debt.payoffDate, i18n.language) })}`
  }
}

/** whether ElementDebtLine renders anything */
export const showsDebtLine = (debt: ElementDebtDto | undefined, canPlan: boolean): debt is ElementDebtDto => !!debt && (debt.isSetUp === 1 || canPlan)
