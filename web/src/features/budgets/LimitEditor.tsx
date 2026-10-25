import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { CalculatorInput } from '@/components/CalculatorInput'
import { amountCardInputClass, CardField } from '@/components/CardField'
import { Button } from '@/components/ui/button'
import { moneyFormat, normalizeNumber } from '@/lib/money'
import { isZero } from '@/lib/decimal'
import type { CurrencyDto } from '@/api/dto/currency'
import { limitAmountFromInput } from './limitAmount'
import type { PlanHint } from './planHint'
import { planHintClass } from './planHint'
import { UpcomingPaymentsPanel, type UpcomingQuery } from './UpcomingPaymentsPanel'

interface LimitEditorProps {
  id: string
  name: string
  value: string
  currency: CurrencyDto | undefined
  onCommit: (amount: string | null) => void
  hint?: PlanHint
  /** set only for a cell that can have money still to come, so other cells fetch nothing */
  upcoming?: UpcomingQuery | null
}

// Desktop inline budget-cell editor (Vue's q-popup-edit).
export function LimitEditor({ id, name, value: currentValue, currency, onCommit, hint = null, upcoming = null }: LimitEditorProps) {
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
          setValue(isZero(currentValue) ? '' : normalizeNumber(currentValue))
          setError(null)
        }
      }}
    >
      <PopoverTrigger asChild>
        <button
          type="button"
          data-limit-trigger=""
          className={`w-full text-right underline-offset-2 hover:underline ${planHintClass(hint)}`}
          aria-label={`limit ${name}`}
        >
          {moneyFormat(currentValue, currency, { showCurrency: false, useNativePrecision: false })}
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
          <CardField label={t('budgets.form.budget_limit.limit.label')} htmlFor={`limit-${id}`} error={error}>
            <div className={amountCardInputClass}>
              <CalculatorInput id={`limit-${id}`} autoFocus value={value} onChange={setValue} />
            </div>
          </CardField>
          {upcoming ? (
            <UpcomingPaymentsPanel
              query={upcoming}
              onUse={(amount) => {
                setValue(amount)
                setError(null)
                document.getElementById(`limit-${id}`)?.focus()
              }}
            />
          ) : null}
          <Button type="submit" size="sm">
            {t('common.button.save.label')}
          </Button>
        </form>
      </PopoverContent>
    </Popover>
  )
}
