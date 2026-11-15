import { useTranslation } from 'react-i18next'
import { ChevronRight } from 'lucide-react'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import type { DebtDto } from '@/api/dto/debt'
import type { Id } from '@/api/types'
import { useCurrencies } from '@/features/currencies/queries'
import { debtDay } from '@/features/debts/debtDates'
import { moneyFormat } from '@/lib/money'

export interface PastDebtsTarget {
  /** the element's name */
  name: string
  /** its paid-off debts, newest first */
  debts: DebtDto[]
}

/** An element's paid-off debts: when each ran and what was borrowed. A pick opens
 *  that debt's review, which is read-only while it stays paid off. */
export function PastDebtsDialog({ target, onClose, onOpen }: { target: PastDebtsTarget | null; onClose: () => void; onOpen: (id: Id) => void }) {
  const { t, i18n } = useTranslation()
  const { data: currencies = [] } = useCurrencies()
  if (!target) {
    return null
  }
  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={t('budgets.page.debts.past_title')} description={target.name}>
      <ul className="flex flex-col" data-testid="past-debts">
        {target.debts.map((debt) => {
          const range = [debt.startDate, debt.endedAt].filter((d) => d !== '').map((d) => debtDay(d, i18n.language)).join(' – ')
          return (
            <li key={debt.id}>
              <button
                type="button"
                className="flex w-full items-center gap-3 rounded-md px-2 py-2.5 text-left hover:bg-econumo-hover"
                onClick={() => onOpen(debt.id)}
              >
                <span className="flex min-w-0 flex-1 flex-col">
                  <span className="truncate text-sm">{range}</span>
                  {debt.amountBorrowed !== '' ? (
                    <span className="truncate text-xs text-muted-foreground">
                      {t('budgets.page.debts.borrowed', { amount: moneyFormat(debt.amountBorrowed, currencies.find((c) => c.id === debt.currencyId)) })}
                    </span>
                  ) : null}
                </span>
                <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
              </button>
            </li>
          )
        })}
      </ul>
    </ResponsiveDialog>
  )
}
