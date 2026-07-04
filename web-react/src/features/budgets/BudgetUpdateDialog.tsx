import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { CurrencySelect } from '@/components/CurrencySelect'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { isNotEmpty, isValidBudgetName } from '@/lib/validation'
import type { BudgetDto } from '@/api/dto/budget'
import type { Id } from '@/api/types'
import { useAccounts } from '@/features/accounts/queries'
import { useUserData } from '@/features/user/queries'
import { useUpdateBudgetDetail, canConfigureBudget } from './queries'

interface BudgetUpdateDialogProps {
  open: boolean
  budget: BudgetDto
  onClose: () => void
}

export function BudgetUpdateDialog({ open, budget, onClose }: BudgetUpdateDialogProps) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const updateBudget = useUpdateBudgetDetail()

  const [name, setName] = useState('')
  const [currencyId, setCurrencyId] = useState<Id | null>(null)
  const [excluded, setExcluded] = useState<Set<Id>>(new Set())
  const [error, setError] = useState<string | null>(null)

  const canConfigure = canConfigureBudget(budget.meta, user?.id)

  useEffect(() => {
    if (open) {
      setName(budget.meta.name)
      setCurrencyId(budget.meta.currencyId)
      setExcluded(new Set(budget.filters.excludedAccountsIds))
      setError(null)
    }
  }, [open, budget])

  const ownAccounts = accounts.filter((a) => !user || a.owner.id === user.id)

  const submit = () => {
    if (!isNotEmpty(name)) {
      setError(t('modules.budget.form.budget.name.validation.required_field'))
      return
    }
    if (!isValidBudgetName(name)) {
      setError(t('modules.budget.form.budget.name.validation.invalid_name'))
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
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={t('modules.budget.modal.update_budget_form.header')}>
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="flex flex-col gap-2">
          <Label htmlFor="budget-upd-name">{t('modules.budget.form.budget.name.label')}</Label>
          <Input id="budget-upd-name" maxLength={64} disabled={!canConfigure} value={name} onChange={(e) => setName(e.target.value)} />
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="budget-upd-currency">{t('modules.budget.form.budget_envelope.currency.label')}</Label>
          <CurrencySelect
            id="budget-upd-currency"
            aria-label={t('modules.budget.form.budget_envelope.currency.label')}
            value={currencyId}
            onChange={setCurrencyId}
            disabled={!canConfigure}
          />
        </div>
        {ownAccounts.length > 0 ? (
          <div className="flex flex-col gap-2">
            <Label>{t('modules.budget.modal.budget_form.accounts')}</Label>
            <ul className="flex max-h-40 flex-col gap-1 overflow-y-auto">
              {ownAccounts.map((account) => (
                <li key={account.id} className="flex items-center justify-between gap-2 px-1">
                  <span className="truncate text-sm">{account.name}</span>
                  <Switch
                    aria-label={`include ${account.name}`}
                    checked={!excluded.has(account.id)}
                    onCheckedChange={(checked) => {
                      setExcluded((prev) => {
                        const next = new Set(prev)
                        if (checked) {
                          next.delete(account.id)
                        } else {
                          next.add(account.id)
                        }
                        return next
                      })
                    }}
                  />
                </li>
              ))}
            </ul>
          </div>
        ) : null}
        <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('elements.button.cancel.label')}
          </Button>
          <Button type="submit" disabled={updateBudget.isPending}>
            {t('elements.button.update.label')}
          </Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
