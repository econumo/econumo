import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { CurrencyField } from '@/features/currencies/CurrencyField'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { isNotEmpty, isValidBudgetName } from '@/lib/validation'
import type { Id } from '@/api/types'
import { useAccounts } from '@/features/accounts/queries'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { useFormErrors } from '@/hooks/useFormErrors'
import { BudgetAccountsField, BudgetSavingsField } from './BudgetAccountsField'
import { PeriodStartDayField } from './PeriodStartDayField'
import { useBudgets } from './queries'

interface BudgetDialogProps {
  open: boolean
  onClose: () => void
  onSubmit: (form: { name: string; currencyId: Id; accountIds: Id[]; savingsAccountIds: Id[]; periodStartDay: number }) => void
}

export function BudgetDialog({ open, onClose, onSubmit }: BudgetDialogProps) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const { data: budgets } = useBudgets()
  // a server older than the setting sends budgets without a day and would ignore a
  // chosen one; with no budget to tell by, assume a current server
  const knowsPeriodStartDay = !budgets?.length || budgets.some((b) => b.periodStartDay !== undefined)

  const [name, setName] = useState('')
  const [currencyId, setCurrencyId] = useState<Id | null>(null)
  const [selected, setSelected] = useState<Set<Id>>(new Set())
  const [savings, setSavings] = useState<Set<Id>>(new Set())
  const [periodStartDay, setPeriodStartDay] = useState(1)
  const { errors, setErrors, clear: clearError, reset: resetErrors } = useFormErrors<{ name?: string; currency?: string; accounts?: string }>()

  useEffect(() => {
    if (open) {
      setName('')
      setCurrencyId(userCurrencyId(user))
      setSelected(new Set())
      setSavings(new Set())
      setPeriodStartDay(1)
      resetErrors()
    }
  }, [open, user])

  const ownAccounts = accounts.filter((a) => !user || a.owner.id === user.id)

  const toggleAccount = (id: Id, included: boolean) => {
    clearError('accounts')
    setSelected((prev) => {
      const next = new Set(prev)
      if (included) {
        next.add(id)
      } else {
        next.delete(id)
      }
      return next
    })
    if (!included) {
      toggleSavings(id, false)
    }
  }

  const toggleSavings = (id: Id, on: boolean) => {
    setSavings((prev) => {
      const next = new Set(prev)
      if (on) {
        next.add(id)
      } else {
        next.delete(id)
      }
      return next
    })
  }

  const submit = () => {
    const next: { name?: string; currency?: string; accounts?: string } = {}
    if (!isNotEmpty(name)) {
      next.name = t('budgets.form.budget.name.validation.required_field')
    } else if (!isValidBudgetName(name)) {
      next.name = t('budgets.form.budget.name.validation.invalid_name')
    }
    if (!currencyId) {
      next.currency = t('budgets.form.budget_envelope.currency.validation.required_field')
    }
    if (selected.size === 0) {
      next.accounts = t('budgets.form.budget.accounts.validation.required')
    }
    setErrors(next)
    if (Object.keys(next).length > 0 || !currencyId) {
      return
    }
    onSubmit({ name, currencyId, accountIds: [...selected], savingsAccountIds: [...savings].filter((id) => selected.has(id)), periodStartDay })
  }

  return (
    <ResponsiveDialog
      open={open}
      caps
      fullScreen
      onOpenChange={(o) => !o && onClose()}
      title={t('budgets.page.settings.create_modal.header')}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" form="budget-create-form">{t('common.button.create.label')}</Button>
        </div>
      }
    >
      <form
        id="budget-create-form"
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <CardField label={t('budgets.form.budget.name.label')} htmlFor="budget-name" error={errors.name}>
          <Input
            id="budget-name"
            className={cardFieldControlClass}
            maxLength={64}
            placeholder={t('budgets.form.budget.name.placeholder')}
            value={name}
            onChange={(e) => {
              clearError('name')
              setName(e.target.value)
            }}
          />
        </CardField>

        <CurrencyField
          label={t('budgets.form.budget_envelope.currency.label')}
          value={currencyId}
          onChange={(id) => {
            clearError('currency')
            setCurrencyId(id)
          }}
          error={errors.currency}
        />

        {knowsPeriodStartDay ? (
          <PeriodStartDayField id="budget-period-start-day" value={periodStartDay} onChange={setPeriodStartDay} />
        ) : null}

        {ownAccounts.length > 0 ? (
          <>
            <BudgetAccountsField accounts={ownAccounts} selected={selected} locked={new Set()} onToggle={toggleAccount} />
            {errors.accounts ? <p className="text-sm text-destructive">{errors.accounts}</p> : null}
            <BudgetSavingsField accounts={ownAccounts} selected={selected} savings={savings} onToggle={toggleSavings} />
          </>
        ) : null}
      </form>

    </ResponsiveDialog>
  )
}
