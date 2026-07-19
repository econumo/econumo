import { useEffect, useState } from 'react'
import { Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { EntityIcon } from '@/components/EntityIcon'
import { fuzzyMatch } from '@/components/CurrencySelect'
import type { AccountDto } from '@/api/dto/account'
import type { Id } from '@/api/types'
import { useFolders } from '@/features/accounts/queries'

interface BudgetAccountsFieldProps {
  accounts: AccountDto[]
  excluded: Set<Id>
  onToggle: (id: Id, included: boolean) => void
}

const SEARCH_THRESHOLD = 6

// Card-style include/exclude account list for the budget forms: searchable,
// with accounts that live in hidden folders separated below the rest.
export function BudgetAccountsField({ accounts, excluded, onToggle }: BudgetAccountsFieldProps) {
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
        checked={!excluded.has(account.id)}
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
            count: String(accounts.length - excluded.size),
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
      <ul className="flex max-h-48 flex-col overflow-x-hidden overflow-y-auto scrollbar-slim">
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
    </div>
  )
}
