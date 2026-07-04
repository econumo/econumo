import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { CalculatorInput } from '@/components/CalculatorInput'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { normalizeNumber } from '@/lib/money'
import type { BudgetElementDto } from '@/api/dto/budget'
import { limitAmountFromInput } from './limitAmount'

interface SetLimitDialogProps {
  element: BudgetElementDto | null
  onClose: () => void
  onCommit: (elementId: string, amount: string | null) => void
}

// Mobile long-press path (Vue's BudgetSetLimitModal), same unified amount rule.
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
      setError(t('elements.validation.invalid_number'))
      return
    }
    onCommit(element.id, result.amount)
    onClose()
  }

  return (
    <ResponsiveDialog open onOpenChange={(o) => !o && onClose()} title={t('modules.budget.modal.set_limit_form.header')} description={element.name}>
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="flex flex-col gap-2">
          <Label htmlFor="set-limit-amount">{t('modules.budget.form.budget_limit.limit.label')}</Label>
          <CalculatorInput id="set-limit-amount" autoFocus value={value} onChange={setValue} />
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>
        <div className="grid grid-cols-2 gap-3">
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('elements.button.cancel.label')}
          </Button>
          <Button type="submit">{t('elements.button.save.label')}</Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
