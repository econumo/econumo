import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { ChevronRight, Lock } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { EntityIcon } from '@/components/EntityIcon'
import { ResponsiveDialog, dialogActionsClass } from '@/components/ResponsiveDialog'
import { fuzzyMatch } from '@/lib/fuzzy'
import type { AccountDto } from '@/api/dto/account'
import type { Id } from '@/api/types'
import { useFolders } from '@/features/accounts/queries'

const SEARCH_THRESHOLD = 6

// The budget forms keep both account choices behind picker rows (like Currency):
// the row names what is picked, a tap opens a searchable checklist. The dialog
// stays the same size whether the user has 3 accounts or 30.
function PickerRow({
  label,
  aside,
  value,
  muted,
  disabled,
  testId,
  onOpen,
}: {
  label: string
  aside?: string
  value: string
  muted: boolean
  disabled?: boolean
  testId: string
  onOpen: () => void
}) {
  return (
    <button
      type="button"
      className="flex w-full items-center justify-between gap-3 rounded-lg bg-econumo-card px-4 py-2.5 text-left hover:bg-econumo-hover disabled:opacity-60 disabled:hover:bg-econumo-card"
      data-testid={testId}
      disabled={disabled}
      onClick={onOpen}
    >
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex items-baseline justify-between gap-2 text-[11px] text-muted-foreground">
          <span>{label}</span>
          {aside ? <span>{aside}</span> : null}
        </span>
        <span className={`truncate text-sm ${muted ? 'text-muted-foreground' : ''}`}>{value}</span>
      </span>
      <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
    </button>
  )
}

interface ChecklistGroup {
  heading?: string
  accounts: AccountDto[]
}

function AccountChecklistDialog({
  open,
  title,
  description,
  groups,
  checked,
  disabled,
  itemLabel,
  onToggle,
  onClose,
  notes,
}: {
  open: boolean
  title: string
  description?: string
  groups: ChecklistGroup[]
  checked: Set<Id>
  disabled?: Set<Id>
  itemLabel: (account: AccountDto) => string
  onToggle: (id: Id, on: boolean) => void
  onClose: () => void
  notes?: ReactNode
}) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  useEffect(() => {
    if (open) {
      setSearch('')
    }
  }, [open])
  const total = groups.reduce((n, g) => n + g.accounts.length, 0)
  const matches = (a: AccountDto) => !search || fuzzyMatch(a.name, search)
  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={title}
      description={description}
      footer={
        <div className={dialogActionsClass}>
          <Button type="button" className="col-span-2" onClick={onClose}>
            {t('common.button.ok.label')}
          </Button>
        </div>
      }
    >
      <Command shouldFilter={false}>
        {total >= SEARCH_THRESHOLD ? (
          <CommandInput placeholder={t('accounts.page.toolbar.search')} value={search} onValueChange={setSearch} />
        ) : null}
        <CommandList className="mt-2 max-h-96 max-md:max-h-none">
          <CommandEmpty>{t('common.list.list_empty')}</CommandEmpty>
          {groups.map((group) => {
            const shown = group.accounts.filter(matches)
            if (shown.length === 0) {
              return null
            }
            return (
              <CommandGroup key={group.heading ?? ''} heading={group.heading}>
                {shown.map((account) => {
                  const on = checked.has(account.id)
                  const locked = !!disabled?.has(account.id)
                  return (
                    <CommandItem
                      key={account.id}
                      value={account.id}
                      data-checked={on}
                      disabled={locked}
                      // a locked member is still checked: it must not read as greyed-out/unchecked
                      className="data-[disabled=true]:opacity-100"
                      aria-label={itemLabel(account)}
                      onSelect={() => onToggle(account.id, !on)}
                    >
                      <EntityIcon name={account.icon} className="text-lg text-muted-foreground" />
                      <span className="min-w-0 flex-1 truncate">{account.name}</span>
                      {locked ? <Lock className="size-3.5 text-muted-foreground" data-testid={`locked-${account.id}`} /> : null}
                    </CommandItem>
                  )
                })}
              </CommandGroup>
            )
          })}
        </CommandList>
      </Command>
      {notes ? <div className="mt-3 flex flex-col gap-1 text-[11px] text-muted-foreground">{notes}</div> : null}
    </ResponsiveDialog>
  )
}

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
      <AccountChecklistDialog
        open={open}
        title={t('budgets.modal.budget_form.accounts')}
        groups={[
          { accounts: accounts.filter((a) => !inHidden(a)) },
          { heading: t('budgets.modal.budget_form.accounts_hidden'), accounts: accounts.filter(inHidden) },
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
      <AccountChecklistDialog
        open={open}
        title={t('budgets.modal.budget_form.savings.label')}
        description={t('budgets.modal.budget_form.savings.description')}
        groups={[{ accounts: included }]}
        checked={savings}
        itemLabel={(a) => t('budgets.modal.budget_form.savings.toggle', { name: a.name })}
        onToggle={onToggle}
        onClose={() => setOpen(false)}
        notes={<p>{t('budgets.modal.budget_form.savings.note')}</p>}
      />
    </>
  )
}
