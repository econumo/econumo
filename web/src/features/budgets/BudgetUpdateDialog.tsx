import { useEffect, useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { CurrencyPickerDialog } from '@/components/CurrencyPickerDialog'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { isNotEmpty, isValidBudgetName } from '@/lib/validation'
import type { BudgetDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { useAccounts } from '@/features/accounts/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData } from '@/features/user/queries'
import { useUpdateBudgetDetail, canConfigureBudget, canEditBudget } from './queries'
import { BudgetAccountsField } from './BudgetAccountsField'

interface BudgetUpdateDialogProps {
  open: boolean
  budget: BudgetDto
  onClose: () => void
}

export function BudgetUpdateDialog({ open, budget, onClose }: BudgetUpdateDialogProps) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const { data: currencies } = useCurrencies()
  const updateBudget = useUpdateBudgetDetail()

  const [name, setName] = useState('')
  const [currencyId, setCurrencyId] = useState<Id | null>(null)
  const [currencyOpen, setCurrencyOpen] = useState(false)
  const [excluded, setExcluded] = useState<Set<Id>>(new Set())
  const [error, setError] = useState<string | null>(null)

  const canConfigure = canConfigureBudget(budget.meta, user?.id)
  // guest (read-only) may open the dialog but must not change anything
  const canEdit = canEditBudget(budget.meta, user?.id)

  useEffect(() => {
    if (open) {
      setName(budget.meta.name)
      setCurrencyId(budget.meta.currencyId)
      setExcluded(new Set(budget.filters.excludedAccountsIds))
      setError(null)
    }
  }, [open, budget])

  const ownAccounts = accounts.filter((a) => !user || a.owner.id === user.id)

  const toggleAccount = (id: Id, included: boolean) => {
    setExcluded((prev) => {
      const next = new Set(prev)
      if (included) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const submit = () => {
    if (!canEdit) {
      return
    }
    if (!isNotEmpty(name)) {
      setError(t('budgets.form.budget.name.validation.required_field'))
      return
    }
    if (!isValidBudgetName(name)) {
      setError(t('budgets.form.budget.name.validation.invalid_name'))
      return
    }
    if (!currencyId) {
      return
    }
    updateBudget.mutate(
      { id: budget.meta.id, name, currencyId, excludedAccounts: [...excluded] },
      { onSuccess: onClose },
    )
  }

  return (
    <ResponsiveDialog
      open={open}
      caps
      fullScreen
      onOpenChange={(o) => !o && onClose()}
      title={t('budgets.modal.update_budget_form.header')}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('common.button.cancel.label')}
          </Button>
          <Button type="submit" form="budget-update-form" disabled={!canEdit || updateBudget.isPending}>
            {t('common.button.update.label')}
          </Button>
        </div>
      }
    >
      <form
        id="budget-update-form"
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <CardField label={t('budgets.form.budget.name.label')} htmlFor="budget-upd-name" error={error}>
          <Input
            id="budget-upd-name"
            className={cardFieldControlClass}
            maxLength={64}
            disabled={!canConfigure}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </CardField>

        {/* same card shape, but a picker row: tap opens the currency search dialog */}
        <button
          type="button"
          className="flex w-full items-center justify-between gap-3 rounded-lg bg-econumo-card px-4 py-2.5 text-left hover:bg-econumo-hover disabled:opacity-60 disabled:hover:bg-econumo-card"
          title={t('budgets.form.budget_envelope.currency.label')}
          disabled={!canConfigure}
          onClick={() => setCurrencyOpen(true)}
        >
          <span className="flex min-w-0 flex-col gap-0.5">
            <span className="text-[11px] text-muted-foreground">{t('budgets.form.budget_envelope.currency.label')}</span>
            <span className="truncate text-sm">{currencies?.find((c) => c.id === currencyId)?.code ?? ''}</span>
          </span>
          <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
        </button>

        {ownAccounts.length > 0 ? (
          <BudgetAccountsField accounts={ownAccounts} excluded={excluded} disabled={!canEdit} onToggle={toggleAccount} />
        ) : null}
      </form>

      <CurrencyPickerDialog
        open={currencyOpen}
        title={t('budgets.form.budget_envelope.currency.label')}
        value={currencyId}
        onClose={() => setCurrencyOpen(false)}
        onPick={setCurrencyId}
      />
    </ResponsiveDialog>
  )
}
