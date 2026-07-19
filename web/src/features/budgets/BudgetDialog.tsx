import { useEffect, useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { CurrencyPickerDialog } from '@/components/CurrencyPickerDialog'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { isNotEmpty, isValidBudgetName } from '@/lib/validation'
import type { Id } from '@/api/types'
import { useAccounts } from '@/features/accounts/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { BudgetAccountsField } from './BudgetAccountsField'

interface BudgetDialogProps {
  open: boolean
  onClose: () => void
  onSubmit: (form: { name: string; currencyId: Id; excludedAccounts: Id[] }) => void
}

export function BudgetDialog({ open, onClose, onSubmit }: BudgetDialogProps) {
  const { t } = useTranslation()
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const { data: currencies } = useCurrencies()

  const [name, setName] = useState('')
  const [currencyId, setCurrencyId] = useState<Id | null>(null)
  const [currencyOpen, setCurrencyOpen] = useState(false)
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
    const next: { name?: string; currency?: string } = {}
    if (!isNotEmpty(name)) {
      next.name = t('budgets.form.budget.name.validation.required_field')
    } else if (!isValidBudgetName(name)) {
      next.name = t('budgets.form.budget.name.validation.invalid_name')
    }
    if (!currencyId) {
      next.currency = t('budgets.form.budget_envelope.currency.validation.required_field')
    }
    setErrors(next)
    if (Object.keys(next).length > 0 || !currencyId) {
      return
    }
    onSubmit({ name, currencyId, excludedAccounts: [...excluded] })
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
            onChange={(e) => setName(e.target.value)}
          />
        </CardField>

        {/* same card shape, but a picker row: tap opens the currency search dialog */}
        <button
          type="button"
          className="flex w-full items-center justify-between gap-3 rounded-lg bg-econumo-card px-4 py-2.5 text-left hover:bg-econumo-hover"
          title={t('budgets.form.budget_envelope.currency.label')}
          onClick={() => setCurrencyOpen(true)}
        >
          <span className="flex min-w-0 flex-col gap-0.5">
            <span className="text-[11px] text-muted-foreground">{t('budgets.form.budget_envelope.currency.label')}</span>
            <span className="truncate text-sm">{currencies?.find((c) => c.id === currencyId)?.code ?? ''}</span>
          </span>
          <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
        </button>
        {errors.currency ? <p className="text-sm text-destructive">{errors.currency}</p> : null}

        {ownAccounts.length > 0 ? <BudgetAccountsField accounts={ownAccounts} excluded={excluded} onToggle={toggleAccount} /> : null}
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
