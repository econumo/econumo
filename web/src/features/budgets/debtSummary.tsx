import { useTranslation } from 'react-i18next'
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

// every summary joins its parts with " · " in every language: as lines, each part on its own
function SummaryText({ text, lines }: { text: string | null; lines: boolean }) {
  if (!lines || text === null) {
    return <>{text}</>
  }
  return (
    <>
      {text.split(' · ').map((part, i) => (
        <span key={i} className={`block ${i > 0 ? 'text-muted-foreground' : ''}`}>
          {part}
        </span>
      ))}
    </>
  )
}

/** "3,000.00 $ left · return by Dec 2027" for a loan with no fixed payments, from the debt list */
function NoFixedText({ debt, fallback, lines }: { debt: ElementDebtDto; fallback: string | null; lines: boolean }) {
  const { t, i18n } = useTranslation()
  const { data: currencies } = useCurrencies()
  const { data } = useDebts()
  // until the list answers the schedule is unknown: show nothing rather than "0 payments left"
  if (!data) {
    return null
  }
  const listed: DebtDto | undefined = data.find((d) => d.id === debt.id)
  if (listed?.schedule !== 'none') {
    return <SummaryText text={fallback} lines={lines} />
  }
  const amount = moneyFormat(listed.currentBalance, currencies?.find((c) => c.id === listed.currencyId))
  const text =
    listed.lastPaymentDate === ''
      ? t('budgets.page.debts.left', { amount })
      : t('budgets.page.debts.left_return_by', { amount, date: debtMonth(listed.lastPaymentDate, i18n.language) })
  return <SummaryText text={text} lines={lines} />
}

/** a debt's summary line (`lines`: each part on its own line, the first in the foreground);
 *  a set-up debt still owed with no payments counted may be a loan with no fixed payments,
 *  which the element's figures cannot tell from the debt list (asked only then) */
export function useDebtSummaryText({ lines = false }: { lines?: boolean } = {}) {
  const summary = useDebtSummary()
  return (debt: ElementDebtDto): ReactNode => {
    const plain = summary(debt)
    return debt.isSetUp === 1 && debt.isPaidOff === 0 && debt.paymentsLeft === 0 ? (
      <NoFixedText debt={debt} fallback={plain} lines={lines} />
    ) : (
      <SummaryText text={plain} lines={lines} />
    )
  }
}

/** whether ElementDebtLine renders anything */
export const showsDebtLine = (debt: ElementDebtDto | undefined, canPlan: boolean): debt is ElementDebtDto => !!debt && (debt.isSetUp === 1 || canPlan)
