import { useEffect, useId, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Check } from 'lucide-react'
import { fullCurrencyLabel } from '@/components/CurrencySelect'
import { EntityIcon } from '@/components/EntityIcon'
import { Input } from '@/components/ui/input'
import { useAccounts } from '@/features/accounts/queries'
import { CurrencyField } from '@/features/currencies/CurrencyField'
import { useCurrencies } from '@/features/currencies/queries'
import { selectableCurrencies } from '@/features/currencies/selectable'
import { EntitySelect } from '@/features/transactions/EntitySelect'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { METRICS, trackEvent } from '@/lib/metrics'
import { pluralPick } from '@/lib/plural'
import { cn } from '@/lib/utils'
import { ColumnChoices } from '../ColumnChoices'
import { NamesList, type NamesListItem } from '../NamesList'
import { columnTags, fromName } from '../columnLabels'
import { candidates } from '../candidates'
import { AnswerRow, Field, QuietButton, Segmented, SkippedNote } from '../controls'
import { accountCurrencies, currencyColumn, fileCurrencyId, unknownRowsCurrency } from '../fileCurrency'
import type { ParsedFile } from '../parse'
import type { AccountChoice, PreparedRow } from '../prepare'
import { distinctNames, existingCodes, hasGuess, nameKey, writableAccounts, type StepBodyProps } from '../wizard'

type Option = AccountChoice['kind']
type NewAccount = Extract<AccountChoice, { kind: 'new' }>
type PerRow = Extract<AccountChoice, { kind: 'column' }>

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
          placeholder={t('transactions.import_wizard.account.name_placeholder')}
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

function CompactCurrencyPicker({ label, value, onChange }: { label: string; value: string; onChange: (currencyId: string) => void }) {
  const { data: currencies } = useCurrencies()
  const options = useMemo(
    () => selectableCurrencies(currencies, value).map((c) => ({ value: c.id, label: c.code, searchText: fullCurrencyLabel(c), ariaLabel: fullCurrencyLabel(c) })),
    [currencies, value],
  )
  return (
    <span className="inline-block w-32 align-middle text-sm">
      <EntitySelect aria-label={label} value={value} options={options} onChange={(id) => id && onChange(id)} />
    </span>
  )
}

/** where each account's currency comes from: a part of the file, or one currency picked for all new accounts */
function CurrencyAnswer({ state, account, onChange }: { state: StepBodyProps['state']; account: PerRow; onChange: (account: PerRow) => void }) {
  const { t, i18n } = useTranslation()
  const [choosing, setChoosing] = useState(false)
  const open = () => {
    trackEvent(METRICS.TRANSACTION_IMPORT_CHANGE_ANSWER, { step: 'account_currency' })
    setChoosing(true)
  }
  if (choosing) {
    return (
      <ColumnChoices
        profiles={state.profiles}
        label={t('transactions.import_wizard.account.currency_question')}
        selected={account.currencyColumn}
        onSelect={(currencyColumn) => {
          onChange({ ...account, currencyColumn })
          setChoosing(false)
        }}
        candidates={candidates('currency', state.profiles, state.answers).filter((c) => c !== account.column)}
        exclude={[account.column]}
        tags={columnTags(state.answers, t, i18n.language, ['currency'])}
        noneLabel={t('transactions.import_wizard.account.currency_none')}
        noneWhenNothingFits
      />
    )
  }
  if (account.currencyColumn !== null) {
    return (
      <AnswerRow from={fromName(state.profiles[account.currencyColumn], state.profiles, t)} onChange={open}>
        {t('transactions.import_wizard.account.currency_from_file')}
      </AnswerRow>
    )
  }
  const label = t('transactions.import_wizard.account.currency_new_accounts')
  // the picker sits where the sentence names the currency, whatever the language's word order
  const [before, after = ''] = t('transactions.import_wizard.account.currency_default', { currency: PICKER_MARK }).split(PICKER_MARK)
  return (
    <AnswerRow from={t('transactions.import_wizard.account.currency_default_hint')} onChange={open}>
      <span className="inline-flex flex-wrap items-center gap-x-2 gap-y-1">
        {before.trim() ? <span>{before.trim()}</span> : null}
        <CompactCurrencyPicker label={label} value={account.currencyId} onChange={(currencyId) => onChange({ ...account, currencyId })} />
        {after.trim() ? <span>{after.trim()}</span> : null}
      </span>
    </AnswerRow>
  )
}

const PICKER_MARK = '\u0000'

export function PerRowAccounts({ file, rows, account, existing, onChange, heading }: {
  file: ParsedFile
  /** the prepared rows: each account says how many of them name it */
  rows?: PreparedRow[]
  account: PerRow
  /** the user's accounts a name matches: lowercased name -> its currency code */
  existing: ReadonlyMap<string, string>
  onChange: (account: PerRow) => void
  heading?: ReactNode
}) {
  const { t } = useTranslation()
  const { data: currencies = [] } = useCurrencies()
  const [changing, setChanging] = useState<ReadonlySet<string>>(() => new Set())
  const names = useMemo(() => distinctNames(file, account.column), [file, account.column])
  const resolved = useMemo(() => accountCurrencies(file, account, currencies), [file, account, currencies])
  const counts = useMemo(() => {
    const found = new Map<string, number>()
    for (const row of rows ?? []) {
      if (row.account === null) continue
      const key = nameKey(row.account)
      found.set(key, (found.get(key) ?? 0) + 1)
    }
    return found
  }, [rows])
  const codeOf = (currencyId: string | undefined) => currencies.find((c) => c.id === currencyId)?.code ?? ''
  const items: NamesListItem[] = names.map((name) => {
    const key = nameKey(name)
    const existingCode = existing.get(key)
    const own = resolved.get(key)
    const overridden = Object.hasOwn(account.overrides, key)
    const unknownCode = existingCode === undefined && !overridden ? (own?.unknownCode ?? null) : null
    const picking = existingCode === undefined && changing.has(key)
    return {
      key,
      name,
      count: rows ? (counts.get(key) ?? 0) : undefined,
      status:
        existingCode !== undefined
          ? t('transactions.import_wizard.account.status_existing', { code: existingCode })
          : t('transactions.import_wizard.account.status_new', { code: codeOf(own?.currencyId ?? account.currencyId) }),
      action:
        existingCode === undefined && !picking ? (
          <QuietButton onClick={() => setChanging((prev) => new Set(prev).add(key))}>
            {t('transactions.import_wizard.account.change_currency')}
          </QuietButton>
        ) : null,
      below:
        unknownCode || picking ? (
          <>
            {unknownCode ? <UnknownCurrencyNote code={unknownCode} /> : null}
            {picking ? (
              <div>
                <CompactCurrencyPicker
                  label={t('transactions.import_wizard.account.currency_of', { name })}
                  value={own?.currencyId ?? account.currencyId}
                  onChange={(currencyId) => onChange({ ...account, overrides: { ...account.overrides, [key]: currencyId } })}
                />
              </div>
            ) : null}
          </>
        ) : null,
    }
  })
  return <NamesList heading={heading ?? t('transactions.import_wizard.account.found')} items={items} />
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
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const writable = useMemo(() => writableAccounts(accounts, user?.id), [accounts, user?.id])
  const existing = useMemo(() => existingCodes(writable), [writable])
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

  const newFallback = (): NewAccount => ({ kind: 'new', name: '', currencyId: fileCurrencyId(state.file, currencies) ?? currencyId })

  const choose = (next: Option) => {
    setPicked(next)
    if (next === option) return
    // leaving "new" keeps the typed name for coming back, without saving it as the answer
    dropTypedName()
    set(remembered.current[next] ?? (next === 'new' ? newFallback() : null))
    // back to a column chosen before: its answer, not the list the other options left open
    if (next === 'column' && remembered.current.column) closeChoices()
  }

  const chooseColumn = (column: number | null) => {
    if (column === null) return
    const previous = account?.kind === 'column' ? account : null
    // a currency answer given before is kept; none is assumed: the user points at one or keeps the default
    set({ kind: 'column', column, currencyColumn: previous?.currencyColumn ?? null, currencyId: previous?.currencyId ?? currencyId, overrides: previous?.overrides ?? {}, currencies: {} })
    closeChoices()
  }

  const accountLabel = t('transactions.import_wizard.steps.account')
  const hasNewAccount = account?.kind === 'column' && distinctNames(state.file, account.column).some((name) => !existing.has(nameKey(name)))
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
      {option === 'new' && (account === null || account.kind === 'new') ? (
        <NewAccountFields
          // "A new account" shown with no answer under it (another step took the account
          // column) still offers its fields; the answer comes with the first one filled
          account={account ?? (remembered.current.new as NewAccount | undefined) ?? newFallback()}
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
              nothingFitsNote={t('transactions.import_wizard.nothing_fits.account')}
              onlyOneNote={t('transactions.import_wizard.only_one.account')}
              tags={columnTags(state.answers, t, i18n.language, ['account'])}
            />
          ) : account?.kind === 'column' ? (
            <>
              <AnswerRow from={fromName(state.profiles[account.column], state.profiles, t)} onChange={change}>
                {t('transactions.import_wizard.account.per_row')}
              </AnswerRow>
              {hasNewAccount ? <CurrencyAnswer state={state} account={account} onChange={set} /> : null}
              <PerRowAccounts
                file={state.file}
                rows={state.rows}
                account={account}
                existing={existing}
                onChange={set}
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
