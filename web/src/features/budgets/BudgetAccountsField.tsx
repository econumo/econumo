import { useEffect, useState } from 'react'
import { Search } from 'lucide-react'
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
  savings: Set<Id>
  onToggleSavings: (id: Id, on: boolean) => void
}

const SEARCH_THRESHOLD = 6

// Card-style include/exclude account list for the budget forms: searchable,
// with accounts that live in hidden folders separated below the rest. `selected`
// may hold ids absent from `accounts` (e.g. a deleted member on the update
// dialog) — those simply render no row, but stay in the set so submitting
// still round-trips them. The included/total counter therefore counts only
// members that HAVE a row, or a budget with deleted members reads "6 of 5".
export function BudgetAccountsField({ accounts, selected, locked, onToggle, savings, onToggleSavings }: BudgetAccountsFieldProps) {
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

  // A row stacks — icon, name and the include switch on the first line, the
  // savings switch on a second line under the name — so the name keeps the row's
  // width. Only a list at least @md (28rem) wide puts it all on one flex line in
  // DOM order: the width that matters is the list's, not the viewport's. The
  // desktop dialog stays max-w-sm, where one line left a name ~115px; the phone
  // drawer spans the viewport, so it only goes single-line when that is wide.
  const row = (account: AccountDto, dimmed: boolean) => (
    <li
      key={account.id}
      className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-2.5 gap-y-1.5 py-2 @md:flex @md:gap-y-0"
    >
      <EntityIcon name={account.icon} className={`col-start-1 row-start-1 text-lg ${dimmed ? 'text-muted-foreground/50' : 'text-muted-foreground'}`} />
      <span className={`col-start-2 row-start-1 min-w-0 flex-1 truncate text-sm ${dimmed ? 'text-muted-foreground' : ''}`}>{account.name}</span>
      {/* the savings role is changeable even for a locked member: it never
          removes the account from the budget. mr-3.5 (not mr-2): each Switch widens
          its own tap zone by 12px a side, so on the single-line (@md) row the
          visible gap to the include switch must be at least 24px (14px here + the
          row's gap-2.5) or their hit zones overlap. */}
      {selected.has(account.id) ? (
        <span className="col-start-2 row-start-2 mr-3.5 flex shrink-0 items-center gap-1.5 justify-self-start text-[11px] text-muted-foreground">
          {t('budgets.modal.budget_form.savings.label')}
          <Switch
            aria-label={t('budgets.modal.budget_form.savings.toggle', { name: account.name })}
            checked={savings.has(account.id)}
            onCheckedChange={(checked) => onToggleSavings(account.id, checked === true)}
          />
        </span>
      ) : null}
      <Switch
        className="col-start-3 row-start-1"
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
      {/* @container is also inline-size containment: a truncating (nowrap) name's full
          width would otherwise be the list's min-content and widen the dialog past its frame */}
      <ul className="@container flex max-h-48 flex-col overflow-x-hidden overflow-y-auto scrollbar-slim">
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
      <p className="text-[11px] text-muted-foreground">{t('budgets.modal.budget_form.savings.note')}</p>
      {locked.size > 0 ? (
        <p className="text-[11px] text-muted-foreground">{t('budgets.modal.budget_form.accounts_locked_hint')}</p>
      ) : null}
    </div>
  )
}
