import { useTranslation } from 'react-i18next'
import { createElement } from 'react'
import type { ReactNode } from 'react'
import type { ElementDebtDto } from '@/api/dto/budget'
import type { DebtDto } from '@/api/dto/debt'
import { useCurrencies } from '@/features/currencies/queries'
import { debtMonth } from '@/features/debts/debtDates'
import { useDebts } from '@/features/debts/queries'
import { moneyFormat } from '@/lib/money'
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

/** "3,000.00 $ left · return by Dec 2027" for a loan with no fixed payments, from the debt list */
function NoFixedText({ debt, fallback }: { debt: ElementDebtDto; fallback: string | null }): ReactNode {
  const { t, i18n } = useTranslation()
  const { data: currencies } = useCurrencies()
  const { data } = useDebts()
  const listed: DebtDto | undefined = data?.find((d) => d.id === debt.id)
  if (listed?.schedule !== 'none') {
    return fallback
  }
  const amount = moneyFormat(listed.currentBalance, currencies?.find((c) => c.id === listed.currencyId))
  return listed.lastPaymentDate === ''
    ? t('budgets.page.debts.left', { amount })
    : t('budgets.page.debts.left_return_by', { amount, date: debtMonth(listed.lastPaymentDate, i18n.language) })
}

/** a debt's summary line; a set-up debt still owed with no payments counted may be a loan with
 *  no fixed payments, which the element's figures cannot tell from the debt list (asked only then) */
export function useDebtSummaryText() {
  const summary = useDebtSummary()
  return (debt: ElementDebtDto): ReactNode => {
    const plain = summary(debt)
    return debt.isSetUp === 1 && debt.isPaidOff === 0 && debt.paymentsLeft === 0 ? createElement(NoFixedText, { debt, fallback: plain }) : plain
  }
}

/** whether ElementDebtLine renders anything */
export const showsDebtLine = (debt: ElementDebtDto | undefined, canPlan: boolean): debt is ElementDebtDto => !!debt && (debt.isSetUp === 1 || canPlan)
