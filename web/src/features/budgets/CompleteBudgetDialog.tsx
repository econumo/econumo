import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router'
import { v7 as uuidv7 } from 'uuid'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { METRICS, trackEvent } from '@/lib/metrics'
import { isNotEmpty, isValidBudgetName } from '@/lib/validation'
import type { BudgetMetaDto } from '@/api/dto/budget'
import { RouterPage } from '@/app/router-pages'
import { useUpdateDefaultBudget } from '@/features/user/queries'
import { useArchiveBudget, useCloneBudget, useUpdateBudgetDetail } from './queries'
import { addMonths, currentMonth, formatPlanMonth, monthDiff } from './planMath'

interface CompleteBudgetDialogProps {
  budget: BudgetMetaDto | null
  onClose: () => void
}

export function CompleteBudgetDialog({ budget, onClose }: CompleteBudgetDialogProps) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const updateBudget = useUpdateBudgetDetail()
  const archiveBudget = useArchiveBudget()
  const cloneBudget = useCloneBudget()
  const updateDefaultBudget = useUpdateDefaultBudget()

  const [endMonth, setEndMonth] = useState('')
  const [continueCopy, setContinueCopy] = useState(true)
  const [copyPlans, setCopyPlans] = useState(true)
  const [name, setName] = useState('')
  const [error, setError] = useState<string | undefined>()

  // Every month from the budget's start through the current one, newest first.
  // A budget that started this month can only end this month.
  const months = useMemo(() => {
    if (!budget) return []
    const start = budget.startedAt.slice(0, 7) + '-01'
    const here = currentMonth()
    const span = Math.max(0, monthDiff(start, here))
    return Array.from({ length: span + 1 }, (_, i) => addMonths(here, -i))
  }, [budget])

  useEffect(() => {
    if (budget) {
      const here = currentMonth()
      const lastMonth = addMonths(here, -1)
      const start = budget.startedAt.slice(0, 7) + '-01'
      setEndMonth(monthDiff(start, lastMonth) < 0 ? here : lastMonth)
      setContinueCopy(true)
      setCopyPlans(true)
      setName(budget.name)
      setError(undefined)
    }
  }, [budget])

  const submit = async () => {
    if (!budget) return
    if (continueCopy) {
      if (!isNotEmpty(name)) {
        setError(t('budgets.form.budget.name.validation.required_field'))
        return
      }
      if (!isValidBudgetName(name)) {
        setError(t('budgets.form.budget.name.validation.invalid_name'))
        return
      }
    }
    // Sequential, stopping at the first failure: each step is a valid end state
    // on its own, and the hooks already surface the standard error toast.
    try {
      await updateBudget.mutateAsync({
        id: budget.id,
        name: budget.name,
        currencyId: budget.currencyId,
        endDate: endMonth,
      })
      await archiveBudget.mutateAsync(budget.id)
      if (continueCopy) {
        const copy = await cloneBudget.mutateAsync({
          id: budget.id,
          newId: uuidv7(),
          name,
          startDate: currentMonth(),
          withLimits: copyPlans,
        })
        await updateDefaultBudget.mutateAsync(copy.id)
        navigate(RouterPage.BUDGET)
      }
      trackEvent(METRICS.BUDGET_COMPLETE)
      onClose()
    } catch {
      // the failing mutation already reported; stop the sequence here
    }
  }

  return (
    <ResponsiveDialog
      open={budget !== null}
      caps
      onOpenChange={(o) => !o && onClose()}
      title={t('budgets.modal.complete_budget_form.header')}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" form="budget-complete-form">
            {t('common.button.save.label')}
          </Button>
        </div>
      }
    >
      <form
        id="budget-complete-form"
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          void submit()
        }}
      >
        <CardField label={t('budgets.modal.complete_budget_form.end_month')} htmlFor="complete-end-month">
          <Select value={endMonth} onValueChange={setEndMonth}>
            <SelectTrigger
              id="complete-end-month"
              aria-label={t('budgets.modal.complete_budget_form.end_month')}
              className="w-full"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {months.map((m) => (
                <SelectItem key={m} value={m}>
                  {formatPlanMonth(m, i18n.language)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </CardField>
        <label className="flex items-center justify-between gap-3 rounded-lg bg-econumo-card px-4 py-2.5 text-sm">
          {t('budgets.modal.complete_budget_form.continue_copy')}
          <Switch checked={continueCopy} onCheckedChange={setContinueCopy} />
        </label>
        {continueCopy ? (
          <>
            <label className="flex items-center justify-between gap-3 rounded-lg bg-econumo-card px-4 py-2.5 text-sm">
              {t('budgets.modal.complete_budget_form.copy_plans')}
              <Switch checked={copyPlans} onCheckedChange={setCopyPlans} />
            </label>
            <CardField label={t('budgets.form.budget.name.label')} htmlFor="complete-budget-name" error={error}>
              <Input
                id="complete-budget-name"
                className={cardFieldControlClass}
                maxLength={64}
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </CardField>
          </>
        ) : null}
      </form>
    </ResponsiveDialog>
  )
}
