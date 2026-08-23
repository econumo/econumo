import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { v7 as uuidv7 } from 'uuid'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { isNotEmpty, isValidBudgetName } from '@/lib/validation'
import type { BudgetMetaDto } from '@/api/dto/budget'
import { useCloneBudget } from './queries'
import { currentMonth } from './planMath'

interface DuplicateBudgetDialogProps {
  budget: BudgetMetaDto | null
  onClose: () => void
}

export function DuplicateBudgetDialog({ budget, onClose }: DuplicateBudgetDialogProps) {
  const { t } = useTranslation()
  const cloneBudget = useCloneBudget()
  const [name, setName] = useState('')
  const [copyPlans, setCopyPlans] = useState(true)
  const [startThisMonth, setStartThisMonth] = useState(false)
  const [error, setError] = useState<string | undefined>()

  useEffect(() => {
    if (budget) {
      setName(budget.name)
      setCopyPlans(true)
      setStartThisMonth(false)
      setError(undefined)
    }
  }, [budget])

  const submit = () => {
    if (!budget) return
    if (!isNotEmpty(name)) {
      setError(t('budgets.form.budget.name.validation.required_field'))
      return
    }
    if (!isValidBudgetName(name)) {
      setError(t('budgets.form.budget.name.validation.invalid_name'))
      return
    }
    cloneBudget.mutate(
      {
        id: budget.id,
        newId: uuidv7(),
        name,
        startDate: startThisMonth ? currentMonth() : null,
        withLimits: copyPlans,
      },
      { onSuccess: onClose },
    )
  }

  return (
    <ResponsiveDialog
      open={budget !== null}
      caps
      onOpenChange={(o) => !o && onClose()}
      title={t('budgets.modal.duplicate_budget_form.header')}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" form="budget-duplicate-form">
            {t('common.button.save.label')}
          </Button>
        </div>
      }
    >
      <form
        id="budget-duplicate-form"
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <CardField label={t('budgets.form.budget.name.label')} htmlFor="duplicate-budget-name" error={error}>
          <Input
            id="duplicate-budget-name"
            className={cardFieldControlClass}
            maxLength={64}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </CardField>
        <label className="flex items-center justify-between gap-3 rounded-lg bg-econumo-card px-4 py-2.5 text-sm">
          {t('budgets.modal.duplicate_budget_form.copy_plans')}
          <Switch checked={copyPlans} onCheckedChange={setCopyPlans} />
        </label>
        <label className="flex items-center justify-between gap-3 rounded-lg bg-econumo-card px-4 py-2.5 text-sm">
          {t('budgets.modal.duplicate_budget_form.start_this_month')}
          <Switch checked={startThisMonth} onCheckedChange={setStartThisMonth} />
        </label>
      </form>
    </ResponsiveDialog>
  )
}
