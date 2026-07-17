import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { CalculatorInput } from '@/components/CalculatorInput'
import { amountCardInputClass, CardField } from '@/components/CardField'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { normalizeNumber } from '@/lib/money'
import type { BudgetElementDto } from '@/api/dto/budget'
import { limitAmountFromInput } from './limitAmount'

interface SetLimitDialogProps {
  element: BudgetElementDto | null
  onClose: () => void
  onCommit: (elementId: string, amount: string | null) => void
}

// Mobile tap/long-press path (Vue's BudgetSetLimitModal), same unified amount rule.
export function SetLimitDialog({ element, onClose, onCommit }: SetLimitDialogProps) {
  const { t } = useTranslation()
  const [value, setValue] = useState('')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (element) {
      setValue(element.budgeted === 0 ? '' : normalizeNumber(element.budgeted))
      setError(null)
    }
  }, [element])

  if (!element) {
    return null
  }

  const submit = () => {
    const result = limitAmountFromInput(value)
    if (!result.ok) {
      setError(t('common.validation.invalid_number'))
      return
    }
    onCommit(element.id, result.amount)
    onClose()
  }

  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={t('budgets.modal.set_limit_form.header')} description={element.name}>
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        {/* the transaction dialog's amount card: label inside, borderless oversized input */}
        <CardField label={t('budgets.form.budget_limit.limit.label')} htmlFor="set-limit-amount" error={error}>
          <div className={amountCardInputClass}>
            <CalculatorInput id="set-limit-amount" autoFocus value={value} onChange={setValue} />
          </div>
        </CardField>
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit">{t('common.button.save.label')}</Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
