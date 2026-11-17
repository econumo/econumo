import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CardField, cardFieldControlClass } from '@/components/CardField'
import { Input } from '@/components/ui/input'
import { useAccounts } from '@/features/accounts/queries'
import { CurrencyField } from '@/features/currencies/CurrencyField'
import { EntitySelect } from '@/features/transactions/EntitySelect'
import { SelectCard } from '@/features/transactions/SelectCard'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { pluralPick } from '@/lib/plural'
import { ChoiceCards } from '../ChoiceCards'
import { columnLabels } from '../columnLabels'
import { FileTable } from '../FileTable'
import type { ParsedFile } from '../parse'
import type { AccountChoice } from '../prepare'
import { defaultAccountName, distinctNames, nameKey, skippedRows, writableAccounts, type StepBodyProps } from '../wizard'
import { SkippedNote } from './SkippedNote'
import { StepTitle } from './StepTitle'

type Option = AccountChoice['kind']
type NewAccount = Extract<AccountChoice, { kind: 'new' }>
type PerRow = Extract<AccountChoice, { kind: 'column' }>

// account names are user text, so a name like "constructor" must not read Object.prototype
const ownCurrency = (currencies: Record<string, string>, key: string) => (Object.hasOwn(currencies, key) ? currencies[key] : undefined)

const NAME_DELAY_MS = 300

function NewAccountFields({ account, onChange, onNameChange }: {
  account: NewAccount
  onChange: (account: NewAccount) => void
  onNameChange: (account: NewAccount) => void
}) {
  const { t } = useTranslation()
  const nameId = useId()
  const [name, setName] = useState(account.name)
  return (
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
      <CardField label={t('transactions.import_wizard.account.name')} htmlFor={nameId}>
        <Input
          id={nameId}
          className={cardFieldControlClass}
          value={name}
          onChange={(e) => {
            setName(e.target.value)
            onNameChange({ ...account, name: e.target.value })
          }}
        />
      </CardField>
      <CurrencyField
        label={t('transactions.import_wizard.account.currency')}
        value={account.currencyId || null}
        onChange={(currencyId) => onChange({ ...account, name, currencyId })}
      />
    </div>
  )
}

export function PerRowAccounts({ file, account, existing, fallbackCurrency, onChange }: {
  file: ParsedFile
  account: PerRow
  existing: ReadonlySet<string>
  fallbackCurrency: string
  onChange: (account: PerRow) => void
}) {
  const { t } = useTranslation()
  const headingId = useId()
  const names = useMemo(() => distinctNames(file, account.column), [file, account.column])
  if (names.length === 0) return null
  return (
    <div className="flex flex-col gap-2">
      <p id={headingId} className="text-sm">
        {t('transactions.import_wizard.account.found')}
      </p>
      <ul aria-labelledby={headingId} className="flex flex-col gap-2">
        {names.map((name) => {
          const key = nameKey(name)
          return (
            <li key={key} className="grid grid-cols-1 items-center gap-2 sm:grid-cols-2">
              <span className="truncate text-sm">{name}</span>
              {existing.has(key) ? (
                <span className="text-sm text-muted-foreground">{t('transactions.import_wizard.account.existing_name')}</span>
              ) : (
                <CurrencyField
                  label={t('transactions.import_wizard.account.currency_of', { name })}
                  value={ownCurrency(account.currencies, key) ?? fallbackCurrency}
                  onChange={(currencyId) => onChange({ ...account, currencies: { ...account.currencies, [key]: currencyId } })}
                />
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}

export function AccountStep({ state, setAnswers }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const writable = useMemo(() => writableAccounts(accounts, user?.id), [accounts, user?.id])
  const existing = useMemo(() => new Set(writable.map((a) => nameKey(a.name))), [writable])
  const currencyId = userCurrencyId(user) ?? ''
  const title = t('transactions.import_wizard.account.title')
  const { account } = state.answers

  // An option can be picked before it has an answer (no account or column chosen
  // yet), and switching back restores what was chosen under it before.
  const [picked, setPicked] = useState<Option>(() => account?.kind ?? (writable.length > 0 ? 'existing' : 'new'))
  const remembered = useRef<Partial<Record<Option, AccountChoice>>>(account ? { [account.kind]: account } : {})
  const option = account?.kind ?? picked

  // Every change to the answers prepares all of the file's rows again, so a typed
  // name is saved after a pause in typing, or when the step goes away, not per key.
  const typedName = useRef<{ account: NewAccount; timer: ReturnType<typeof setTimeout> } | null>(null)
  const setAnswersRef = useRef(setAnswers)
  setAnswersRef.current = setAnswers

  const dropTypedName = (): NewAccount | null => {
    const typed = typedName.current
    if (!typed) return null
    clearTimeout(typed.timer)
    typedName.current = null
    remembered.current.new = typed.account
    return typed.account
  }

  const saveTypedName = () => {
    const typed = dropTypedName()
    if (typed) setAnswersRef.current('account', { account: typed })
  }

  useEffect(() => saveTypedName, [])

  const typeName = (next: NewAccount) => {
    if (typedName.current) clearTimeout(typedName.current.timer)
    typedName.current = { account: next, timer: setTimeout(saveTypedName, NAME_DELAY_MS) }
  }

  const set = (next: AccountChoice | null) => {
    dropTypedName()
    if (next) remembered.current[next.kind] = next
    setAnswers('account', { account: next })
  }

  const choose = (next: Option) => {
    setPicked(next)
    if (next === option) return
    // leaving "new" keeps the typed name for coming back, without saving it as the answer
    dropTypedName()
    const fallback: AccountChoice | null =
      next === 'new' ? { kind: 'new', name: defaultAccountName(state.fileName), currencyId } : null
    set(remembered.current[next] ?? fallback)
  }

  const chooseColumn = (column: number | null) => {
    if (column === null) return
    const previous = account?.kind === 'column' ? account.currencies : {}
    const keys = distinctNames(state.file, column).map(nameKey).filter((key) => !existing.has(key))
    // fromEntries, not assignment: a name like "__proto__" must stay a plain key
    const currencies = Object.fromEntries(keys.map((key) => [key, ownCurrency(previous, key) ?? currencyId]))
    set({ kind: 'column', column, currencies })
  }

  const accountLabel = t('transactions.import_wizard.steps.account')
  const nameless = account?.kind === 'column' ? state.rows.filter((row) => row.reason === 'no_account').length : 0

  return (
    <div className="flex flex-col gap-3">
      <StepTitle>{title}</StepTitle>
      <ChoiceCards<Option>
        label={title}
        value={option}
        options={[
          { value: 'new', label: t('transactions.import_wizard.account.new') },
          ...(writable.length > 0 ? [{ value: 'existing' as const, label: t('transactions.import_wizard.account.existing') }] : []),
          { value: 'column', label: t('transactions.import_wizard.account.column') },
        ]}
        onChange={choose}
      />
      {option === 'new' && account?.kind === 'new' ? <NewAccountFields account={account} onChange={set} onNameChange={typeName} /> : null}
      {option === 'existing' ? (
        <SelectCard label={accountLabel}>
          <EntitySelect
            aria-label={accountLabel}
            value={account?.kind === 'existing' ? account.accountId : null}
            onChange={(accountId) => set(accountId ? { kind: 'existing', accountId } : null)}
            options={writable.map((a) => ({
              value: a.id,
              label: `${a.name} (${a.balance} ${a.currency.code})`,
              icon: a.icon,
              searchText: a.name,
            }))}
          />
        </SelectCard>
      ) : null}
      {option === 'column' ? (
        <>
          <p className="text-sm">{t('transactions.import_wizard.account.column_question')}</p>
          <FileTable
            file={state.file}
            label={t('transactions.import_wizard.account.column_question')}
            selected={account?.kind === 'column' ? account.column : null}
            onSelect={chooseColumn}
            labels={columnLabels(state.answers, t, ['account'])}
            mutedRows={skippedRows(state.rows, 'no_account')}
          />
          {nameless > 0 ? (
            <SkippedNote>{pluralPick(t('transactions.import_wizard.account.skipped'), nameless, i18n.language)}</SkippedNote>
          ) : null}
          {account?.kind === 'column' ? (
            <PerRowAccounts file={state.file} account={account} existing={existing} fallbackCurrency={currencyId} onChange={set} />
          ) : null}
        </>
      ) : null}
    </div>
  )
}
