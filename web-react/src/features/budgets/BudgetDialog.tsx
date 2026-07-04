import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { CurrencySelect } from '@/components/CurrencySelect'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { isNotEmpty, isValidBudgetName } from '@/lib/validation'
import type { Id } from '@/api/types'
import { useAccounts } from '@/features/accounts/queries'
import { useUserData, userCurrencyId } from '@/features/user/queries'

interface BudgetDialogProps {
  open: boolean
  onClose: () => void
  onSubmit: (form: { name: string; currencyId: Id; excludedAccounts: Id[] }) => void
}

export function BudgetDialog({ open, onClose, onSubmit }: BudgetDialogProps) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()

  const [name, setName] = useState('')
  const [currencyId, setCurrencyId] = useState<Id | null>(null)
  const [excluded, setExcluded] = useState<Set<Id>>(new Set())
  const [errors, setErrors] = useState<{ name?: string; currency?: string }>({})

  useEffect(() => {
    if (open) {
      setName('')
      setCurrencyId(userCurrencyId(user))
      setExcluded(new Set())
      setErrors({})
    }
  }, [open, user])

  const ownAccounts = accounts.filter((a) => !user || a.owner.id === user.id)

  const submit = () => {
    const next: { name?: string; currency?: string } = {}
    if (!isNotEmpty(name)) {
      next.name = t('modules.budget.form.budget.name.validation.required_field')
    } else if (!isValidBudgetName(name)) {
      next.name = t('modules.budget.form.budget.name.validation.invalid_name')
    }
    if (!currencyId) {
      next.currency = t('modules.budget.form.budget_envelope.currency.validation.required_field')
    }
    setErrors(next)
    if (Object.keys(next).length > 0 || !currencyId) {
      return
    }
    onSubmit({ name, currencyId, excludedAccounts: [...excluded] })
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={(o) => !o && onClose()} title={t('modules.budget.page.settings.create_modal.header')}>
      <form
        className="flex flex-col gap-4"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="flex flex-col gap-2">
          <Label htmlFor="budget-name">{t('modules.budget.form.budget.name.label')}</Label>
          <Input
            id="budget-name"
            maxLength={64}
            placeholder={t('modules.budget.form.budget.name.placeholder')}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          {errors.name ? <p className="text-sm text-destructive">{errors.name}</p> : null}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="budget-currency">{t('modules.budget.form.budget_envelope.currency.label')}</Label>
          <CurrencySelect
            id="budget-currency"
            aria-label={t('modules.budget.form.budget_envelope.currency.label')}
            value={currencyId}
            onChange={setCurrencyId}
          />
          {errors.currency ? <p className="text-sm text-destructive">{errors.currency}</p> : null}
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

        <div className="grid grid-cols-2 gap-3">
          <Button type="button" variant="secondary" onClick={onClose}>
            {t('elements.button.cancel.label')}
          </Button>
          <Button type="submit">{t('elements.button.create.label')}</Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
