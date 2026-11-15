import { useCurrencies } from '@/features/currencies/queries'
import { moneyFormat } from '@/lib/money'
import { pluralPick } from '@/lib/plural'
import { useTranslation } from 'react-i18next'
import type { DebtDto } from '@/api/dto/debt'

/** a debt's amounts, always in its own currency with the symbol */
export function useDebtMoney() {
  const { data: currencies } = useCurrencies()
  return (amount: string, debt: Pick<DebtDto, 'currencyId'>): string =>
    moneyFormat(amount, currencies?.find((c) => c.id === debt.currencyId))
}

export function usePaymentsLeft() {
  const { t, i18n } = useTranslation()
  return (count: number): string => pluralPick(t('debts.list.payments_left'), count, i18n.language)
}
