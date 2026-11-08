import { useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { ChevronDown, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { v7 as uuidv7 } from 'uuid'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { EntityIcon } from '@/components/EntityIcon'
import { useAccounts } from '@/features/accounts/queries'
import { useCurrencies, useCurrencyRates } from '@/features/currencies/queries'
import { selectableCurrencies } from '@/features/currencies/selectable'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { addThousandSeparators, formatNumber } from '@/lib/money'
import { guessCurrencyCode } from '@/lib/regionCurrency'
import { cn } from '@/lib/utils'
import type { Id } from '@/api/types'
import type { AccountRow } from './draft'
import { guessAccountIcon, looksLikeSavings } from './defaults'
import { parseBalance } from './balance'
import { nameLengthValid, NAME_MAX, NAME_MIN } from './names'
import { CurrenciesBlock, CurrencyListSheet } from './CurrenciesBlock'
import { rowInput, rowTile } from './CategoriesStep'
import type { StepBodyProps } from './OnboardingPage'
import { StepLoadError } from './StepLoadError'
import { useDay0 } from './useDay0'

export interface ExistingAccount {
  id: Id
  name: string
  icon: string
  balance: string
  currencyId: Id
}

interface AccountsStepProps {
  // the user's own accounts: listed first and locked, no balance asked for
  existing: ExistingAccount[]
  initialRows?: AccountRow[]
  initialExtraCurrencyIds?: Id[]
  // server account id by draft row key, for rows an earlier submit created
  createdIds?: Record<string, Id>
  onContinue: (rows: AccountRow[], extraCurrencyIds: Id[], mainCurrencyId: Id) => void
  onSkip: () => void
  pending: boolean
  error?: ReactNode
}

// currencyId '' = the row follows the default currency until the user picks one
const emptyRow = (): AccountRow => ({ key: uuidv7(), name: '', balance: '', currencyId: '', savings: false, icon: 'credit_card' })

export function AccountsStep({ existing, initialRows, initialExtraCurrencyIds, createdIds, onContinue, onSkip, pending, error }: AccountsStepProps) {
  const { t, i18n } = useTranslation()
  const { data: currencies } = useCurrencies()
  const { data: rates } = useCurrencyRates()
  const { data: user } = useUserData()
  const words = () => ({ cash: t('onboarding.words.cash'), savings: t('onboarding.words.savings') })

  // A draft row whose create landed but whose response was lost has no created id; the
  // account it made is matched by name, so it is shown once and its op id is never replayed.
  const claims = useMemo(() => {
    const byAccount = new Map<Id, AccountRow>()
    const draft = initialRows ?? []
    for (const r of draft) {
      const id = r.accountId ?? createdIds?.[r.key]
      if (id) byAccount.set(id, r)
    }
    for (const a of existing) {
      if (byAccount.has(a.id)) continue
      const name = a.name.trim().toLowerCase()
      const taken = new Set([...byAccount.values()].map((r) => r.key))
      const row = draft.find((r) => !taken.has(r.key) && !r.accountId && !createdIds?.[r.key] && r.name.trim().toLowerCase() === name)
      if (row) byAccount.set(a.id, row)
    }
    return byAccount
    // the draft and the list are read once, at mount, like the rows seeded from them
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  const claimedKeys = new Set([...claims.values()].map((r) => r.key))
  const draftRowOf = (accountId: Id) => claims.get(accountId) ?? (initialRows ?? []).find((r) => (r.accountId ?? createdIds?.[r.key]) === accountId)
  const [savingsOverride, setSavingsOverride] = useState<Record<Id, boolean>>({})
  const savingsOf = (a: ExistingAccount) => savingsOverride[a.id] ?? draftRowOf(a.id)?.savings ?? looksLikeSavings(a.name, words().savings)
  const [allRows, setRows] = useState<AccountRow[]>(() => {
    const leftover = (initialRows ?? []).filter((r) => !r.accountId && !createdIds?.[r.key] && !claimedKeys.has(r.key))
    return leftover.length > 0 || existing.length > 0 ? leftover : [emptyRow()]
  })
  // a row an earlier (failed) submit created comes back in the server list
  const rows = allRows.filter((r) => !createdIds?.[r.key])
  const [extraIds, setExtraIds] = useState<Id[]>(initialExtraCurrencyIds ?? [])
  const [mainOverride, setMainOverride] = useState<Id | null>(null)
  // the first new row's pick: rows the user has not changed by hand follow it
  const [sharedId, setSharedId] = useState<Id | null>(null)
  const [pickingRow, setPickingRow] = useState<string | null>(null)
  const [open, setOpen] = useState<string | null>(null)
  const [focusKey, setFocusKey] = useState<string | null>(null)
  const savingsTouched = useRef(new Set<string>())
  // a name still being typed is short on its way to valid, so its error waits for blur
  const [nameTouched, setNameTouched] = useState<ReadonlySet<string>>(new Set())

  const defaultId = useMemo(() => {
    const list = currencies ?? []
    const guessed = guessCurrencyCode(navigator.languages ?? [])
    const byGuess = guessed ? list.find((c) => c.code === guessed && selectableCurrencies([c]).length > 0) : undefined
    return byGuess?.id ?? userCurrencyId(user) ?? list[0]?.id ?? ''
  }, [currencies, user])

  const currencyOf = (row: AccountRow) => row.currencyId || sharedId || defaultId
  const update = (key: string, patch: Partial<AccountRow>) => setRows((prev) => prev.map((r) => (r.key === key ? { ...r, ...patch } : r)))

  const rename = (row: AccountRow, name: string) => {
    const patch: Partial<AccountRow> = { name, icon: guessAccountIcon(name, words()) }
    if (!savingsTouched.current.has(row.key)) patch.savings = looksLikeSavings(name, words().savings)
    update(row.key, patch)
  }

  const addRow = () => {
    const row = emptyRow()
    setRows((prev) => [...prev, row])
    setFocusKey(row.key)
  }
  const remove = (key: string) => setRows((prev) => prev.filter((r) => r.key !== key))

  const active = rows.filter((r) => r.name.trim() !== '')
  const counted = [...existing.map((a) => a.currencyId), ...(active.length > 0 || existing.length > 0 ? active : rows).map(currencyOf)].filter(Boolean)
  const usedIds = [...new Set(counted)]
  const firstRow = active[0] ?? rows[0]
  const computedMain: Id | null = (firstRow ? currencyOf(firstRow) : existing[0]?.currencyId) || null
  const extras = extraIds.filter((id) => !usedIds.includes(id))
  const selectedIds = [...usedIds, ...extras]
  const mainId = mainOverride && selectedIds.includes(mainOverride) ? mainOverride : computedMain

  const baseId = rates?.[0]?.baseCurrencyId ?? userCurrencyId(user)
  const missingRateIds = rates ? selectedIds.filter((id) => id !== baseId && !rates.some((r) => r.currencyId === id)) : []

  const parse = (raw: string) => parseBalance(raw, i18n.language)
  const invalid = new Set(active.filter((r) => parse(r.balance) === null).map((r) => r.key))
  const invalidName = new Set(active.filter((r) => !nameLengthValid(r.name)).map((r) => r.key))
  const canContinue = active.length + existing.length > 0 && invalid.size === 0 && invalidName.size === 0 && mainId !== null && !pending

  const submit = () => {
    if (!canContinue || !mainId) return
    onContinue(
      [
        ...existing.map((a) => ({
          key: draftRowOf(a.id)?.key ?? `account:${a.id}`,
          name: a.name,
          balance: '',
          currencyId: a.currencyId,
          savings: savingsOf(a),
          icon: a.icon,
          accountId: a.id,
        })),
        ...active.map((r) => ({ ...r, name: r.name.trim(), balance: parse(r.balance) ?? '0', currencyId: currencyOf(r) })),
      ],
      extras,
      mainId,
    )
  }

  const editing = rows.find((r) => r.key === pickingRow)
  const codeOf = (id: Id) => currencies?.find((c) => c.id === id)?.code ?? ''
  const chip = 'shrink-0 rounded-md border px-1.5 py-0.5 text-xs text-muted-foreground'

  const toggle = (key: string) => (
    <button
      type="button"
      aria-label={t('onboarding.accounts.details')}
      aria-expanded={open === key}
      className="-mr-1 grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground hover:bg-accent"
      onClick={() => setOpen((o) => (o === key ? null : key))}
    >
      <ChevronDown className={cn('size-4 transition-transform', open === key && 'rotate-180')} />
    </button>
  )
  const savingsLabel = <div className="px-1 text-[11px] leading-tight text-primary">{t('onboarding.accounts.savings_label')}</div>
  const savingsSwitch = (checked: boolean, onChange: (v: boolean) => void) => (
    <label className="flex items-center gap-2 text-sm text-muted-foreground">
      {t('onboarding.accounts.savings')}
      <Switch checked={checked} onCheckedChange={onChange} />
    </label>
  )

  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-2xl font-normal">{t('onboarding.accounts.heading')}</h2>
      <p className="text-sm text-muted-foreground">{t('onboarding.accounts.subtext')}</p>

      <ul className="mt-4 divide-y divide-border/60 overflow-hidden rounded-xl border bg-card">
        {existing.map((a) => {
          const cur = currencies?.find((c) => c.id === a.currencyId)
          const savings = savingsOf(a)
          return (
            <li key={a.id} className="px-3 py-2" data-testid="account-row">
              <div className="flex items-center gap-2 sm:gap-3">
                <span className={rowTile}>
                  <EntityIcon name={a.icon} className="text-lg" />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="truncate px-1">{a.name}</div>
                  {savings && savingsLabel}
                </div>
                <span className="shrink-0 text-sm text-muted-foreground tabular-nums">
                  {addThousandSeparators(formatNumber(a.balance, cur?.fractionDigits ?? 2, true)).join('.')}
                </span>
                <span className={chip}>{cur?.code ?? ''}</span>
                {toggle(a.id)}
              </div>
              {open === a.id && (
                <div className="mt-2 flex items-center pl-11">
                  {savingsSwitch(savings, (v) => setSavingsOverride((prev) => ({ ...prev, [a.id]: v })))}
                </div>
              )}
            </li>
          )
        })}
        {rows.map((row) => {
          const showName = invalidName.has(row.key) && nameTouched.has(row.key)
          return (
            <li key={row.key} className="px-3 py-2" data-testid="account-row">
              <div className="flex items-center gap-2 sm:gap-3">
                <span className={rowTile}>
                  <EntityIcon name={row.icon} className="text-lg" />
                </span>
                <div className="min-w-0 flex-1">
                  <input
                    className={cn(rowInput, 'w-full')}
                    aria-label={t('onboarding.accounts.name_label')}
                    placeholder={t('onboarding.accounts.name_label')}
                    autoFocus={row.key === focusKey}
                    value={row.name}
                    aria-invalid={showName}
                    onChange={(e) => rename(row, e.target.value)}
                    onBlur={() => setNameTouched((prev) => new Set(prev).add(row.key))}
                  />
                  {row.savings && savingsLabel}
                </div>
                <input
                  className="w-20 shrink-0 rounded-lg border bg-transparent px-2 py-1.5 text-right text-sm tabular-nums outline-none focus-visible:ring-2 focus-visible:ring-ring/50 aria-invalid:border-destructive sm:w-28"
                  aria-label={t('onboarding.accounts.balance_label')}
                  inputMode="decimal"
                  placeholder="0"
                  value={row.balance}
                  aria-invalid={invalid.has(row.key)}
                  onChange={(e) => update(row.key, { balance: e.target.value })}
                />
                <button
                  type="button"
                  className={cn(chip, 'hover:bg-accent')}
                  aria-label={t('onboarding.accounts.currency_title')}
                  onClick={() => setPickingRow(row.key)}
                >
                  {codeOf(currencyOf(row))}
                </button>
                {toggle(row.key)}
              </div>
              {showName && <p className="mt-1 pl-11 text-sm text-destructive">{t('errors.account.name_length', { min: NAME_MIN, max: NAME_MAX })}</p>}
              {invalid.has(row.key) && <p className="mt-1 pl-11 text-sm text-destructive">{t('onboarding.accounts.balance_invalid')}</p>}
              {open === row.key && (
                <div className="mt-2 flex items-center justify-between gap-2 pl-11">
                  {savingsSwitch(row.savings, (v) => {
                    savingsTouched.current.add(row.key)
                    update(row.key, { savings: v })
                  })}
                  <Button type="button" variant="ghost" size="sm" className="text-muted-foreground" onClick={() => remove(row.key)}>
                    {t('onboarding.accounts.remove')}
                  </Button>
                </div>
              )}
            </li>
          )
        })}
        <li>
          <button type="button" className="flex w-full items-center gap-3 px-3 py-2 text-left text-muted-foreground hover:bg-accent/50" onClick={addRow}>
            <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-muted">
              <Plus className="size-4" />
            </span>
            {t('onboarding.accounts.add')}
          </button>
        </li>
      </ul>

      <CurrenciesBlock
        currencies={currencies ?? []}
        selectableOptions={selectableCurrencies(currencies)}
        usedIds={usedIds}
        extraIds={extras}
        mainId={mainId}
        missingRateIds={missingRateIds}
        onAddExtra={(id) => setExtraIds((prev) => (prev.includes(id) ? prev : [...prev, id]))}
        onRemoveExtra={(id) => setExtraIds((prev) => prev.filter((x) => x !== id))}
        onMainChange={setMainOverride}
      />

      {error}
      <div className="mt-6 flex items-center justify-between">
        <Button type="button" variant="ghost" disabled={pending} onClick={onSkip}>
          {t('onboarding.accounts.skip')}
        </Button>
        <Button type="button" disabled={!canContinue} onClick={submit}>
          {t('onboarding.accounts.continue')}
        </Button>
      </div>

      <CurrencyListSheet
        open={editing !== undefined}
        title={t('onboarding.accounts.currency_title')}
        options={selectableCurrencies(currencies, editing ? currencyOf(editing) : undefined)}
        value={editing ? currencyOf(editing) : null}
        onClose={() => setPickingRow(null)}
        onPick={(id) => {
          if (!pickingRow) return
          if (pickingRow === rows[0]?.key) {
            setSharedId(id)
            update(pickingRow, { currencyId: '' })
          } else update(pickingRow, { currencyId: id })
        }}
      />
    </section>
  )
}

export function AccountsStepBody({ onDone, onSkip }: StepBodyProps) {
  const { data: user } = useUserData()
  const accounts = useAccounts()
  const day0 = useDay0()
  const [skipping, setSkipping] = useState(false)
  if (accounts.isError) return <StepLoadError error={accounts.error} onRetry={() => void accounts.refetch()} />
  if (!day0.ready) return null
  const existing = (accounts.data ?? [])
    .filter((a) => a.owner.id === user?.id)
    .map((a) => ({ id: a.id, name: a.name, icon: a.icon, balance: a.balance, currencyId: a.currency.id }))
  return (
    <AccountsStep
      existing={existing}
      initialRows={day0.draft?.rows}
      initialExtraCurrencyIds={day0.draft?.extraCurrencyIds}
      createdIds={day0.draft?.created}
      pending={day0.pending || skipping}
      error={
        day0.error != null ? (
          <p role="alert" className="text-sm text-destructive">
            {apiErrorMessage(day0.error)}
          </p>
        ) : undefined
      }
      onContinue={(rows, extras, main) => {
        void day0
          .saveAccounts(rows, extras, main)
          .then(() => onDone())
          .catch(() => undefined)
      }}
      onSkip={() => {
        setSkipping(true)
        void onSkip().finally(() => setSkipping(false))
      }}
    />
  )
}
