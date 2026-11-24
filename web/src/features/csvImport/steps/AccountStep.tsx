import { useEffect, useId, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Check } from 'lucide-react'
import { EntityIcon } from '@/components/EntityIcon'
import { Input } from '@/components/ui/input'
import { useAccounts } from '@/features/accounts/queries'
import { CurrencyField } from '@/features/currencies/CurrencyField'
import { useCurrencies } from '@/features/currencies/queries'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { pluralPick } from '@/lib/plural'
import { cn } from '@/lib/utils'
import { ColumnChoices } from '../ColumnChoices'
import { columnTags, fromName } from '../columnLabels'
import { candidates } from '../candidates'
import { AnswerRow, Field, Segmented, SkippedNote } from '../controls'
import { currencyColumn, fileCurrencyId, rowsCurrencyId, unknownRowsCurrency } from '../fileCurrency'
import type { ParsedFile } from '../parse'
import type { AccountChoice, PreparedRow } from '../prepare'
import { defaultAccountName, distinctNames, hasGuess, nameKey, writableAccounts, type StepBodyProps } from '../wizard'

type Option = AccountChoice['kind']
type NewAccount = Extract<AccountChoice, { kind: 'new' }>
type PerRow = Extract<AccountChoice, { kind: 'column' }>

// account names are user text, so a name like "constructor" must not read Object.prototype
const ownCurrency = (currencies: Record<string, string>, key: string) => (Object.hasOwn(currencies, key) ? currencies[key] : undefined)

const NAME_DELAY_MS = 300

// The fallback currency stays; the note says why it is not the file's own.
function UnknownCurrencyNote({ code }: { code: string }) {
  const { t } = useTranslation()
  return (
    <p className="text-xs text-muted-foreground">
      {t('transactions.import_wizard.account.currency_unknown', { code })}
    </p>
  )
}

function NewAccountFields({ account, unknownCode, onChange, onNameChange }: {
  account: NewAccount
  unknownCode: string | null
  onChange: (account: NewAccount) => void
  onNameChange: (account: NewAccount) => void
}) {
  const { t } = useTranslation()
  const nameId = useId()
  const [name, setName] = useState(account.name)
  return (
    <div className="flex max-w-md flex-col gap-5">
      <Field label={t('transactions.import_wizard.account.name')} hint={t('transactions.import_wizard.account.name_hint')} htmlFor={nameId}>
        <Input
          id={nameId}
          className="h-10"
          value={name}
          onChange={(e) => {
            setName(e.target.value)
            onNameChange({ ...account, name: e.target.value })
          }}
        />
      </Field>
      <div className="flex flex-col gap-1.5">
        <CurrencyField
          label={t('transactions.import_wizard.account.currency')}
          value={account.currencyId || null}
          onChange={(currencyId) => onChange({ ...account, name, currencyId })}
        />
        {unknownCode ? <UnknownCurrencyNote code={unknownCode} /> : null}
      </div>
    </div>
  )
}

export function PerRowAccounts({ file, rows, account, existing, fallbackCurrency, onChange, heading, labelledBy }: {
  file: ParsedFile
  /** the prepared rows: each account says how many of them name it */
  rows?: PreparedRow[]
  account: PerRow
  existing: ReadonlySet<string>
  fallbackCurrency: string
  onChange: (account: PerRow) => void
  heading?: ReactNode
  /** the answer above names the list instead of a heading of its own */
  labelledBy?: string
}) {
  const { t, i18n } = useTranslation()
  const headingId = useId()
  const { data: currencies = [] } = useCurrencies()
  const names = useMemo(() => distinctNames(file, account.column), [file, account.column])
  // what each account's own rows say, when it is a code this instance has no currency for
  const unknownCodes = useMemo(() => {
    const codes = currencyColumn(file, currencies)
    const found = new Map<string, string>()
    if (codes === null) return found
    for (const name of names) {
      const key = nameKey(name)
      const code = unknownRowsCurrency(file.rows.filter((row) => nameKey(row[account.column] ?? '') === key), codes, currencies)
      if (code !== null) found.set(key, code)
    }
    return found
  }, [file, account.column, names, currencies])
  const counts = useMemo(() => {
    const found = new Map<string, number>()
    for (const row of rows ?? []) {
      if (row.account === null) continue
      const key = nameKey(row.account)
      found.set(key, (found.get(key) ?? 0) + 1)
    }
    return found
  }, [rows])
  if (names.length === 0) return null
  return (
    <div className="flex flex-col gap-2">
      {labelledBy ? null : (
        <div id={headingId} className="text-sm">
          {heading ?? t('transactions.import_wizard.account.found')}
        </div>
      )}
      <ul aria-labelledby={labelledBy ?? headingId} className="divide-y divide-border rounded-xl border border-border">
        {names.map((name) => {
          const key = nameKey(name)
          const unknownCode = unknownCodes.get(key)
          return (
            <li key={key} className="grid grid-cols-1 items-center gap-2 px-3 py-2 sm:grid-cols-2">
              <span className="min-w-0 truncate text-sm">
                {name}
                {rows ? (
                  <span className="text-muted-foreground">
                    {' · '}
                    {pluralPick(t('transactions.import_wizard.account.transactions'), counts.get(key) ?? 0, i18n.language)}
                  </span>
                ) : null}
              </span>
              {existing.has(key) ? (
                <span className="text-sm text-muted-foreground">{t('transactions.import_wizard.account.existing_name')}</span>
              ) : (
                <div className="flex min-w-0 flex-col gap-1.5">
                  <CurrencyField
                    label={t('transactions.import_wizard.account.currency_of', { name })}
                    value={ownCurrency(account.currencies, key) ?? fallbackCurrency}
                    onChange={(currencyId) => onChange({ ...account, currencies: { ...account.currencies, [key]: currencyId } })}
                  />
                  {unknownCode ? <UnknownCurrencyNote code={unknownCode} /> : null}
                </div>
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}

function AccountList({ accounts, selected, label, onSelect }: {
  accounts: ReturnType<typeof writableAccounts>
  selected: string | null
  label: string
  onSelect: (id: string) => void
}) {
  const name = useId()
  return (
    <div role="radiogroup" aria-label={label} className="grid gap-2">
      {accounts.map((a) => {
        const on = selected === a.id
        return (
          <label
            key={a.id}
            className={cn(
              'flex cursor-pointer items-center gap-3 rounded-xl border p-3 text-left has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring/50',
              on ? 'border-primary bg-econumo-magenta-light/25' : 'border-border hover:border-primary/40',
            )}
          >
            <input type="radio" name={name} className="sr-only" checked={on} onChange={() => onSelect(a.id)} aria-label={a.name} />
            <span aria-hidden className="flex size-9 shrink-0 items-center justify-center rounded-full bg-econumo-card text-lg text-entity-icon">
              <EntityIcon name={a.icon || 'account_balance_wallet'} />
            </span>
            <span className="min-w-0 flex-1 truncate text-sm">{a.name}</span>
            <span className="text-xs text-muted-foreground">{a.currency.code}</span>
            {on ? <Check aria-hidden className="size-4 text-primary" /> : null}
          </label>
        )
      })}
    </div>
  )
}

export function AccountStep({ state, setAnswers, choosing, change, closeChoices }: StepBodyProps) {
  const { t, i18n } = useTranslation()
  const answerId = useId()
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const writable = useMemo(() => writableAccounts(accounts, user?.id), [accounts, user?.id])
  const existing = useMemo(() => new Set(writable.map((a) => nameKey(a.name))), [writable])
  const { data: currencies = [] } = useCurrencies()
  const currencyId = userCurrencyId(user) ?? ''
  const fileUnknownCode = useMemo(() => unknownRowsCurrency(state.file.rows, currencyColumn(state.file, currencies), currencies), [state.file, currencies])
  const title = t('transactions.import_wizard.account.title')
  const columnQuestion = t('transactions.import_wizard.account.column_question')
  const guessed = hasGuess('account', state)
  const open = choosing || !guessed
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
      next === 'new' ? { kind: 'new', name: defaultAccountName(state.fileName), currencyId: fileCurrencyId(state.file, currencies) ?? currencyId } : null
    set(remembered.current[next] ?? fallback)
    // back to a column chosen before: its answer, not the list the other options left open
    if (next === 'column' && remembered.current.column) closeChoices()
  }

  const chooseColumn = (column: number | null) => {
    if (column === null) return
    const previous = account?.kind === 'column' ? account.currencies : {}
    const keys = distinctNames(state.file, column).map(nameKey).filter((key) => !existing.has(key))
    // an account's own rows may say its currency; a currency already chosen for it is kept
    const codes = currencyColumn(state.file, currencies)
    const fromFile = (key: string) => rowsCurrencyId(state.file.rows.filter((row) => nameKey(row[column] ?? '') === key), codes, currencies)
    // fromEntries, not assignment: a name like "__proto__" must stay a plain key
    const perAccount = Object.fromEntries(keys.map((key) => [key, ownCurrency(previous, key) ?? fromFile(key) ?? currencyId]))
    set({ kind: 'column', column, currencies: perAccount })
    closeChoices()
  }

  const accountLabel = t('transactions.import_wizard.steps.account')
  const nameless = account?.kind === 'column' ? state.rows.filter((row) => row.reason === 'no_account').length : 0

  return (
    <div className="flex flex-col gap-5">
      <Segmented<Option>
        label={title}
        value={option}
        options={[
          { value: 'new', label: t('transactions.import_wizard.account.new') },
          ...(writable.length > 0 ? [{ value: 'existing' as const, label: t('transactions.import_wizard.account.existing') }] : []),
          { value: 'column', label: t('transactions.import_wizard.account.column') },
        ]}
        onChange={choose}
      />
      {option === 'new' && account?.kind === 'new' ? (
        <NewAccountFields
          account={account}
          unknownCode={fileUnknownCode}
          onChange={set}
          onNameChange={typeName}
        />
      ) : null}
      {option === 'existing' ? (
        <AccountList
          accounts={writable}
          label={accountLabel}
          selected={account?.kind === 'existing' ? account.accountId : null}
          onSelect={(accountId) => set({ kind: 'existing', accountId })}
        />
      ) : null}
      {option === 'column' ? (
        <>
          {open ? (
            <ColumnChoices
              profiles={state.profiles}
              label={columnQuestion}
              selected={account?.kind === 'column' ? account.column : null}
              onSelect={chooseColumn}
              candidates={candidates('account', state.profiles, state.answers)}
              tags={columnTags(state.answers, t, i18n.language, ['account'])}
            />
          ) : account?.kind === 'column' ? (
            <>
              <AnswerRow id={answerId} from={fromName(state.profiles[account.column], state.profiles, t)} onChange={change}>
                {t('transactions.import_wizard.account.per_row')}
              </AnswerRow>
              <PerRowAccounts
                file={state.file}
                rows={state.rows}
                account={account}
                existing={existing}
                fallbackCurrency={currencyId}
                onChange={set}
                labelledBy={answerId}
              />
            </>
          ) : null}
          {nameless > 0 ? (
            <SkippedNote>{pluralPick(t('transactions.import_wizard.account.skipped'), nameless, i18n.language)}</SkippedNote>
          ) : null}
        </>
      ) : null}
    </div>
  )
}
