import { useTranslation } from 'react-i18next'
import { Progress } from '@/components/ui/progress'
import { moneyFormat } from '@/lib/money'
import type { BudgetDto } from '@/api/dto/budget'
import { useCurrencies } from '@/features/currencies/queries'
import { makeBudgetExchange, widgetMath } from './budgetMath'
import { useBudgetPeriodStore } from './budgetStore'

const MONTH_KEYS = ['jan', 'feb', 'mar', 'apr', 'may', 'jun', 'jul', 'aug', 'sep', 'oct', 'nov', 'dec'] as const

export function ExpenseWidget({ budget, currencyId }: { budget: BudgetDto; currencyId: string }) {
  const { t } = useTranslation()
  const { data: currencies = [] } = useCurrencies()
  const selectedDate = useBudgetPeriodStore((s) => s.selectedDate)

  const balance = budget.balances.find((b) => b.currencyId === currencyId)
  const math = widgetMath(balance)
  const currency = currencies.find((c) => c.id === currencyId)
  const budgetCurrency = currencies.find((c) => c.id === budget.meta.currencyId)

  const [y, m] = selectedDate.split('-').map(Number)
  const period = `${t(`common.date.month_short.${MONTH_KEYS[m - 1]}`)} ${y}`

  const exchangeFn = makeBudgetExchange(budget, currencies)
  const rate = currencyId !== budget.meta.currencyId ? exchangeFn(budget.meta.currencyId, currencyId, '1') : null

  return (
    <section className="flex w-full max-w-sm flex-col gap-2 rounded-md border p-3" data-testid="expense-widget">
      <header className="flex items-baseline justify-between text-sm font-medium">
        {t('budgets.modal.expense_widget.header')}
        <span className="text-xs font-normal text-muted-foreground">{period}</span>
      </header>
      <div className="flex items-baseline justify-between text-sm">
        <span className="text-expense">{moneyFormat(math.spent, currency)}</span>
        <span className="text-muted-foreground">{moneyFormat(math.total, currency)}</span>
      </div>
      <Progress
        value={math.progress * 100}
        aria-label="outflow progress"
        className={math.overspent ? '[&>*]:bg-red-600' : undefined}
        data-overbudget={math.overspent || undefined}
      />
      {rate !== null && budgetCurrency && currency ? (
        <p className="text-xs text-muted-foreground">
          {t('budgets.modal.expense_widget.conversion_rate', {
            period,
            defaultCurrency: budgetCurrency.code,
            rate: String(rate),
          })}
        </p>
      ) : null}
    </section>
  )
}
