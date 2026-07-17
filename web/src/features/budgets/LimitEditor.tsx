import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { CalculatorInput } from '@/components/CalculatorInput'
import { amountCardInputClass, CardField } from '@/components/CardField'
import { Button } from '@/components/ui/button'
import { moneyFormat, normalizeNumber } from '@/lib/money'
import type { BudgetElementDto } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import { limitAmountFromInput } from './limitAmount'

interface LimitEditorProps {
  element: BudgetElementDto
  currency: CurrencyDto | undefined
  onCommit: (amount: string | null) => void
}

// Desktop inline budget-cell editor (Vue's q-popup-edit).
export function LimitEditor({ element, currency, onCommit }: LimitEditorProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [value, setValue] = useState('')
  const [error, setError] = useState<string | null>(null)

  const commit = () => {
    const result = limitAmountFromInput(value)
    if (!result.ok) {
      setError(t('common.validation.invalid_formula'))
      return
    }
    onCommit(result.amount)
    setOpen(false)
  }

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (next) {
          setValue(element.budgeted === 0 ? '' : normalizeNumber(element.budgeted))
          setError(null)
        }
      }}
    >
      <PopoverTrigger asChild>
        <button type="button" className="w-full text-right underline-offset-2 hover:underline" aria-label={`limit ${element.name}`}>
          {moneyFormat(element.budgeted, currency, { showCurrency: false, useNativePrecision: false })}
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-64 p-2" align="end">
        <form
          className="flex flex-col gap-2"
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            commit()
          }}
        >
          {/* the transaction dialog's amount card: label inside, borderless oversized input */}
          <CardField label={t('budgets.form.budget_limit.limit.label')} htmlFor={`limit-${element.id}`} error={error}>
            <div className={amountCardInputClass}>
              <CalculatorInput id={`limit-${element.id}`} autoFocus value={value} onChange={setValue} />
            </div>
          </CardField>
          <Button type="submit" size="sm">
            {t('common.button.save.label')}
          </Button>
        </form>
      </PopoverContent>
    </Popover>
  )
}
