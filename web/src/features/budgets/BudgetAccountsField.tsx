import { useEffect, useState } from 'react'
import { Check, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { EntityIcon } from '@/components/EntityIcon'
import { fuzzyMatch } from '@/lib/fuzzy'
import type { AccountDto } from '@/api/dto/account'
import type { Id } from '@/api/types'
import { useFolders } from '@/features/accounts/queries'

interface BudgetAccountsFieldProps {
  accounts: AccountDto[]
  selected: Set<Id>
  locked: Set<Id>
  onToggle: (id: Id, included: boolean) => void
}

const SEARCH_THRESHOLD = 6

// Card-style include/exclude account list for the budget forms: searchable,
// with accounts that live in hidden folders separated below the rest. `selected`
// may hold ids absent from `accounts` (e.g. a deleted member on the update
// dialog) — those simply render no row, but stay in the set so submitting
// still round-trips them. The included/total counter therefore counts only
// members that HAVE a row, or a budget with deleted members reads "6 of 5".
export function BudgetAccountsField({ accounts, selected, locked, onToggle }: BudgetAccountsFieldProps) {
  const { t } = useTranslation()
  const { data: folders = [] } = useFolders()
  const [search, setSearch] = useState('')

  useEffect(() => {
    setSearch('')
  }, [accounts.length])

  const hiddenFolderIds = new Set(folders.filter((f) => f.isVisible === 0).map((f) => f.id))
  const matches = (a: AccountDto) => !search || fuzzyMatch(a.name, search)
  const visible = accounts.filter((a) => !(a.folderId && hiddenFolderIds.has(a.folderId)) && matches(a))
  const hidden = accounts.filter((a) => a.folderId && hiddenFolderIds.has(a.folderId) && matches(a))

  const row = (account: AccountDto, dimmed: boolean) => (
    <li key={account.id} className="flex items-center gap-2.5 py-2">
      <EntityIcon name={account.icon} className={`text-lg ${dimmed ? 'text-muted-foreground/50' : 'text-muted-foreground'}`} />
      <span className={`min-w-0 flex-1 truncate text-sm ${dimmed ? 'text-muted-foreground' : ''}`}>{account.name}</span>
      <Switch
        aria-label={`include ${account.name}`}
        checked={selected.has(account.id)}
        disabled={locked.has(account.id)}
        onCheckedChange={(checked) => onToggle(account.id, checked === true)}
      />
    </li>
  )

  return (
    <div className="flex flex-col gap-0.5 rounded-lg bg-econumo-card px-4 py-2.5">
      <span className="flex items-baseline justify-between">
        <Label className="text-[11px] font-normal text-muted-foreground">{t('budgets.modal.budget_form.accounts')}</Label>
        <span className="text-[11px] text-muted-foreground">
          {t('budgets.modal.budget_form.accounts_included', {
            count: String(accounts.reduce((n, a) => (selected.has(a.id) ? n + 1 : n), 0)),
            total: String(accounts.length),
          })}
        </span>
      </span>
      {accounts.length >= SEARCH_THRESHOLD ? (
        <span className="mt-1 flex items-center gap-2 rounded-md bg-background px-2.5 py-1.5">
          <Search className="size-4 shrink-0 text-muted-foreground" />
          <input
            aria-label={t('accounts.page.toolbar.search')}
            placeholder={t('accounts.page.toolbar.search')}
            className="w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </span>
      ) : null}
      {/* contain: a truncating (nowrap) name's full width would otherwise be the
          list's min-content and widen the dialog past its frame */}
      <ul className="flex max-h-72 flex-col overflow-x-hidden overflow-y-auto scrollbar-slim [contain:inline-size]">
        {visible.map((account) => row(account, false))}
        {hidden.length > 0 ? (
          <li className="pt-2 pb-1 text-[11px] uppercase tracking-wide text-muted-foreground" data-testid="hidden-accounts-heading">
            {t('budgets.modal.budget_form.accounts_hidden')}
          </li>
        ) : null}
        {hidden.map((account) => row(account, true))}
        {visible.length === 0 && hidden.length === 0 ? (
          <li className="py-2 text-sm text-muted-foreground">{t('common.list.list_empty')}</li>
        ) : null}
      </ul>
      {locked.size > 0 ? (
        <p className="text-[11px] text-muted-foreground">{t('budgets.modal.budget_form.accounts_locked_hint')}</p>
      ) : null}
    </div>
  )
}

interface BudgetSavingsFieldProps {
  /** every account the accounts field lists, in its order */
  accounts: AccountDto[]
  selected: Set<Id>
  savings: Set<Id>
  onToggle: (id: Id, on: boolean) => void
}

// The savings role, picked apart from inclusion: one tap-to-toggle chip per
// included account, so the account list above stays a single switch per row.
export function BudgetSavingsField({ accounts, selected, savings, onToggle }: BudgetSavingsFieldProps) {
  const { t } = useTranslation()
  const included = accounts.filter((a) => selected.has(a.id))
  return (
    <div className="flex flex-col gap-1.5 rounded-lg bg-econumo-card px-4 py-2.5" data-testid="budget-savings-field">
      <span className="text-[11px] text-muted-foreground">{t('budgets.modal.budget_form.savings.label')}</span>
      <p className="text-sm">{t('budgets.modal.budget_form.savings.description')}</p>
      {included.length > 0 ? (
        <div className="flex flex-wrap gap-2 py-1">
          {included.map((account) => {
            const on = savings.has(account.id)
            return (
              <button
                key={account.id}
                type="button"
                aria-pressed={on}
                aria-label={t('budgets.modal.budget_form.savings.toggle', { name: account.name })}
                title={account.name}
                onClick={() => onToggle(account.id, !on)}
                className={`inline-flex h-8 max-w-full items-center gap-1.5 rounded-full border px-3 text-sm transition-colors ${
                  on ? 'border-primary bg-primary/10 text-foreground' : 'border-input bg-background text-muted-foreground hover:text-foreground'
                }`}
              >
                {on ? <Check className="size-4 shrink-0 text-primary" /> : <EntityIcon name={account.icon} className="shrink-0 text-base" />}
                <span className="truncate">{account.name}</span>
              </button>
            )
          })}
        </div>
      ) : (
        <p className="py-1 text-sm text-muted-foreground">{t('budgets.modal.budget_form.savings.empty')}</p>
      )}
      <p className="text-[11px] text-muted-foreground">{t('budgets.modal.budget_form.savings.note')}</p>
    </div>
  )
}
