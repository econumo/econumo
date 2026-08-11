import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { CalculatorInput } from '@/components/CalculatorInput'
import { amountCardInputClass, CardField } from '@/components/CardField'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { normalizeNumber } from '@/lib/money'
import { isZero } from '@/lib/decimal'
import { limitAmountFromInput } from './limitAmount'

interface SetLimitDialogProps {
  target: { id: string; name: string; value: string } | null
  onClose: () => void
  onCommit: (elementId: string, amount: string | null) => void
}

// Mobile tap/long-press path (Vue's BudgetSetLimitModal), same unified amount rule.
export function SetLimitDialog({ target, onClose, onCommit }: SetLimitDialogProps) {
  const { t } = useTranslation()
  const [value, setValue] = useState('')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (target) {
      setValue(isZero(target.value) ? '' : normalizeNumber(target.value))
      setError(null)
    }
  }, [target])

  if (!target) {
    return null
  }

  const submit = () => {
    const result = limitAmountFromInput(value)
    if (!result.ok) {
      setError(t('common.validation.invalid_number'))
      return
    }
    onCommit(target.id, result.amount)
    onClose()
  }

  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={t('budgets.modal.set_limit_form.header')} description={target.name}>
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
