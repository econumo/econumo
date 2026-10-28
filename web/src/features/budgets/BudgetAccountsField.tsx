import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChecklistSheet, PickerRow } from '@/components/ChecklistField'
import type { AccountDto } from '@/api/dto/account'
import type { Id } from '@/api/types'
import { useFolders } from '@/features/accounts/queries'

const names = (accounts: AccountDto[]) => accounts.map((a) => a.name).join(', ')

interface BudgetAccountsFieldProps {
  accounts: AccountDto[]
  selected: Set<Id>
  locked: Set<Id>
  onToggle: (id: Id, included: boolean) => void
}

// Which of the user's accounts the budget counts. Accounts in hidden folders list
// after the rest under their own heading. `selected` may hold ids absent from
// `accounts` (e.g. a deleted member on the update dialog) — those simply render no
// row, but stay in the set so submitting still round-trips them. The included/total
// counter therefore counts only members that HAVE a row, or a budget with deleted
// members reads "6 of 5".
export function BudgetAccountsField({ accounts, selected, locked, onToggle }: BudgetAccountsFieldProps) {
  const { t } = useTranslation()
  const { data: folders = [] } = useFolders()
  const [open, setOpen] = useState(false)

  const hiddenFolderIds = new Set(folders.filter((f) => f.isVisible === 0).map((f) => f.id))
  const inHidden = (a: AccountDto) => !!a.folderId && hiddenFolderIds.has(a.folderId)
  const included = accounts.filter((a) => selected.has(a.id))

  return (
    <>
      <PickerRow
        label={t('budgets.modal.budget_form.accounts')}
        aside={t('budgets.modal.budget_form.accounts_included', { count: String(included.length), total: String(accounts.length) })}
        value={included.length > 0 ? names(included) : t('budgets.modal.budget_form.savings.none')}
        muted={included.length === 0}
        testId="budget-accounts-field"
        onOpen={() => setOpen(true)}
      />
      <ChecklistSheet
        open={open}
        title={t('budgets.modal.budget_form.accounts')}
        groups={[
          { items: accounts.filter((a) => !inHidden(a)) },
          { heading: t('budgets.modal.budget_form.accounts_hidden'), items: accounts.filter(inHidden) },
        ]}
        checked={selected}
        disabled={locked}
        itemLabel={(a) => `include ${a.name}`}
        onToggle={onToggle}
        onClose={() => setOpen(false)}
        notes={locked.size > 0 ? <p>{t('budgets.modal.budget_form.accounts_locked_hint')}</p> : null}
      />
    </>
  )
}

interface BudgetSavingsFieldProps {
  /** every account the accounts field lists, in its order */
  accounts: AccountDto[]
  selected: Set<Id>
  savings: Set<Id>
  onToggle: (id: Id, on: boolean) => void
}

// The savings role, picked apart from inclusion, over the included accounts only.
export function BudgetSavingsField({ accounts, selected, savings, onToggle }: BudgetSavingsFieldProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const included = accounts.filter((a) => selected.has(a.id))
  const chosen = included.filter((a) => savings.has(a.id))
  return (
    <>
      <PickerRow
        label={t('budgets.modal.budget_form.savings.label')}
        value={
          included.length === 0
            ? t('budgets.modal.budget_form.savings.empty')
            : chosen.length > 0
              ? names(chosen)
              : t('budgets.modal.budget_form.savings.none')
        }
        muted={chosen.length === 0}
        disabled={included.length === 0}
        testId="budget-savings-field"
        onOpen={() => setOpen(true)}
      />
      <ChecklistSheet
        open={open}
        title={t('budgets.modal.budget_form.savings.label')}
        description={t('budgets.modal.budget_form.savings.description')}
        groups={[{ items: included }]}
        checked={savings}
        itemLabel={(a) => t('budgets.modal.budget_form.savings.toggle', { name: a.name })}
        onToggle={onToggle}
        onClose={() => setOpen(false)}
        notes={<p>{t('budgets.modal.budget_form.savings.note')}</p>}
      />
    </>
  )
}
