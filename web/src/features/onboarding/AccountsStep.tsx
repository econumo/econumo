import { useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { Archive, ArchiveRestore, Plus, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { v7 as uuidv7 } from 'uuid'
import { Button } from '@/components/ui/button'
import { EntityIcon } from '@/components/EntityIcon'
import { IconPicker } from '@/components/IconPicker'
import { ResponsiveDialog } from '@/components/ResponsiveDialog'
import { useAccounts, useFolders } from '@/features/accounts/queries'
import { useCurrencies } from '@/features/currencies/queries'
import { usableCurrencies } from '@/features/currencies/selectable'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { apiErrorMessage } from '@/lib/apiError'
import { cmp } from '@/lib/decimal'
import { guessCurrencyCode } from '@/lib/regionCurrency'
import { cn } from '@/lib/utils'
import type { Id } from '@/api/types'
import type { AccountRow } from './draft'
import { decimalInput, parseBalance } from './balance'
import { labelled, nameLengthValid, NAME_MAX, NAME_MIN } from './names'
import { CurrenciesBlock, CurrencyListSheet } from './CurrenciesBlock'
import { rowInput, rowTile } from './CategoriesStep'
import type { StepBodyProps } from './OnboardingPage'
import { StepLoadError } from './StepLoadError'
import { useDay0, type AccountReview } from './useDay0'
import { useSettled } from './useSettled'

export interface ExistingAccount {
  id: Id
  name: string
  icon: string
  balance: string
  currencyId: Id
  // in a hidden folder on the server; the review may flip it
  hidden?: boolean
}

interface AccountsStepProps {
  // the user's own accounts: listed first with their name locked, an editable balance, a currency
  // chip (a CSV import creates them in the instance base currency) and an Archive / Restore toggle
  existing: ExistingAccount[]
  initialRows?: AccountRow[]
  initialExtraCurrencyIds?: Id[]
  onContinue: (rows: AccountRow[], extraCurrencyIds: Id[], mainCurrencyId: Id, review?: AccountReview) => void
  onSkip: () => void
  pending: boolean
  error?: ReactNode
}

// "1234.5" in a two-digit currency reads "1234.50"; a longer fraction is never cut
function padFraction(value: string, digits: number): string {
  const [int, frac = ''] = value.split('.')
  const padded = frac.padEnd(digits, '0')
  return padded === '' ? int : `${int}.${padded}`
}

// currencyId '' = the row follows the default currency until the user picks one
const emptyRow = (): AccountRow => ({ key: uuidv7(), name: '', balance: '', currencyId: '', icon: 'credit_card' })

export function AccountsStep({ existing, initialRows, initialExtraCurrencyIds, onContinue, onSkip, pending, error }: AccountsStepProps) {
  const { t, i18n } = useTranslation()
  const { data: currencies } = useCurrencies()
  const { data: user } = useUserData()

  // rows of a submit that failed come back editable; the user's accounts are listed apart
  const leftover = useMemo(
    () => (initialRows ?? []).filter((r) => !r.accountId),
    // read once, at mount, like the rows seeded from it
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  )
  const [allRows, setRows] = useState<AccountRow[]>(() => {
    if (leftover.length > 0 || existing.length > 0) return leftover
    const labelKey = 'onboarding.accounts.default_cash'
    return [{ ...emptyRow(), name: t(labelKey), labelKey, icon: 'payments' }]
  })
  // the Cash row is only for a user with no accounts: accounts arriving after it was seeded (an
  // import landing late) withdraw it unless the user has made it their own
  const rows = allRows
    .filter((r) => !(existing.length > 0 && r.labelKey === 'onboarding.accounts.default_cash' && r.balance.trim() === '' && !r.iconPicked))
    .map((r) => labelled(r, t))
  const [extraIds, setExtraIds] = useState<Id[]>(initialExtraCurrencyIds ?? [])
  const [mainOverride, setMainOverride] = useState<Id | null>(null)
  // a failed submit's draft stores each existing account with the currency it was sent with
  const restoredCurrency = (a: ExistingAccount) => (initialRows ?? []).find((r) => r.accountId === a.id)?.currencyId || a.currencyId
  const restoredLead = existing.find((a) => !a.hidden)
  const restoredShared = restoredLead && restoredCurrency(restoredLead) !== restoredLead.currencyId ? restoredCurrency(restoredLead) : null
  // the first row's pick (the first shown existing account, else the first new row): rows the
  // user has not changed by hand follow it
  const [sharedId, setSharedId] = useState<Id | null>(existing.length === 0 ? leftover[0]?.currencyId || null : restoredShared)
  // existing accounts whose currency the user picked by hand. From a draft only a currency that
  // is neither the account's own nor the lead's counts as a pick: one equal to the lead's was
  // following it, and must keep following
  const [picked, setPicked] = useState<Readonly<Record<Id, Id>>>(() => {
    const out: Record<Id, Id> = {}
    for (const a of existing) {
      if (a.id === restoredLead?.id) continue
      const c = restoredCurrency(a)
      if (c !== a.currencyId && c !== (restoredShared ?? restoredLead?.currencyId)) out[a.id] = c
    }
    return out
  })
  const [pickingRow, setPickingRow] = useState<string | null>(null)
  const [pickingAccount, setPickingAccount] = useState<Id | null>(null)
  // balances typed into existing accounts; a failed submit's edits come back
  const [balanceEdits, setBalanceEdits] = useState<Readonly<Record<Id, string>>>(() => {
    const out: Record<Id, string> = {}
    for (const r of initialRows ?? []) {
      const a = r.accountId ? existing.find((e) => e.id === r.accountId) : undefined
      if (a && r.balance !== '' && cmp(r.balance, a.balance) !== 0) out[a.id] = decimalInput(r.balance, i18n.language)
    }
    return out
  })
  const [iconRowKey, setIconRowKey] = useState<string | null>(null)
  const [focusKey, setFocusKey] = useState<string | null>(null)
  // a name still being typed is short on its way to valid, so its error waits for blur
  const [nameTouched, setNameTouched] = useState<ReadonlySet<string>>(new Set())
  // existing accounts whose hidden state differs from the server's
  const [flipped, setFlipped] = useState<ReadonlySet<Id>>(new Set())
  const isHidden = (a: ExistingAccount) => Boolean(a.hidden) !== flipped.has(a.id)
  const toggleHidden = (id: Id) =>
    setFlipped((prev) => {
      const next = new Set(prev)
      if (!next.delete(id)) next.add(id)
      return next
    })

  const defaultId = useMemo(() => {
    const list = currencies ?? []
    const guessed = guessCurrencyCode(navigator.languages ?? [])
    const byGuess = guessed ? list.find((c) => c.code === guessed && usableCurrencies([c]).length > 0) : undefined
    return byGuess?.id ?? userCurrencyId(user) ?? list[0]?.id ?? ''
  }, [currencies, user])

  const currencyOf = (row: AccountRow) => row.currencyId || sharedId || defaultId
  // a hidden account keeps its currency; a shown one takes its own pick, else the first row's
  const lead = existing.find((a) => !isHidden(a))
  const currencyOfExisting = (a: ExistingAccount) => (isHidden(a) ? a.currencyId : picked[a.id] || (lead ? sharedId : null) || a.currencyId)
  const update = (key: string, patch: Partial<AccountRow>) => setRows((prev) => prev.map((r) => (r.key === key ? { ...r, ...patch } : r)))

  const rename = (row: AccountRow, name: string) => {
    update(row.key, { name, labelKey: undefined })
  }

  const addRow = () => {
    const row = emptyRow()
    setRows((prev) => [...prev, row])
    setFocusKey(row.key)
  }
  const remove = (key: string) => {
    if (!lead && key === rows[0]?.key) setSharedId(rows[1] ? currencyOf(rows[1]) : null)
    setRows((prev) => prev.filter((r) => r.key !== key))
  }

  const active = rows.filter((r) => r.name.trim() !== '')
  const counted = [
    ...existing.map(currencyOfExisting),
    ...(active.length > 0 || existing.length > 0 ? active : rows).map(currencyOf),
    ...(sharedId && (lead || rows.length > 0) ? [sharedId] : []),
  ].filter(Boolean)
  const usedIds = [...new Set(counted)]
  const computedMain: Id | null =
    (sharedId && (lead || rows.length > 0)
      ? sharedId
      : active[0]
        ? currencyOf(active[0])
        : (lead ? currencyOfExisting(lead) : existing[0]?.currencyId) || (rows[0] ? currencyOf(rows[0]) : '')) || null
  const extras = extraIds.filter((id) => !usedIds.includes(id))
  const selectedIds = [...usedIds, ...extras]
  const mainId = mainOverride && selectedIds.includes(mainOverride) ? mainOverride : computedMain

  const parse = (raw: string) => parseBalance(raw, i18n.language)
  const invalid = new Set(active.filter((r) => parse(r.balance) === null).map((r) => r.key))
  const balanceInput = (a: ExistingAccount) => {
    if (balanceEdits[a.id] !== undefined) return balanceEdits[a.id]
    const digits = currencies?.find((c) => c.id === a.currencyId)?.fractionDigits ?? 2
    return decimalInput(padFraction(a.balance, digits), i18n.language)
  }
  // a shown account's balance as typed, '' while untouched or unchanged, null when unreadable
  const balanceOf = (a: ExistingAccount): string | null => {
    // a cleared balance on an existing account means unchanged, never zero
    if (isHidden(a) || balanceEdits[a.id] === undefined || balanceEdits[a.id].trim() === '') return ''
    const value = parse(balanceEdits[a.id])
    return value === null ? null : cmp(value, a.balance) === 0 ? '' : value
  }
  const invalidExisting = new Set(existing.filter((a) => balanceOf(a) === null).map((a) => a.id))
  const invalidName = new Set(active.filter((r) => !nameLengthValid(r.name)).map((r) => r.key))
  const shownExisting = existing.filter((a) => !isHidden(a)).length
  const canContinue =
    active.length + shownExisting > 0 && invalid.size === 0 && invalidExisting.size === 0 && invalidName.size === 0 && mainId !== null && !pending

  const submit = () => {
    if (!canContinue || !mainId) return
    onContinue(
      [
        ...existing.map((a) => ({
          key: `account:${a.id}`,
          name: a.name,
          balance: balanceOf(a) || '',
          currencyId: currencyOfExisting(a),
          icon: a.icon,
          accountId: a.id,
        })),
        ...active.map((r) => ({ ...r, name: r.name.trim(), balance: parse(r.balance) ?? '0', currencyId: currencyOf(r) })),
      ],
      extras,
      mainId,
      {
        hide: existing.filter((a) => flipped.has(a.id) && !a.hidden).map((a) => a.id),
        unhide: existing.filter((a) => flipped.has(a.id) && a.hidden).map((a) => a.id),
        currencies: existing
          .filter((a) => currencyOfExisting(a) !== a.currencyId)
          .map((a) => ({ accountId: a.id, currencyId: currencyOfExisting(a) })),
        balances: existing.flatMap((a) => {
          const balance = balanceOf(a)
          return balance ? [{ accountId: a.id, balance }] : []
        }),
      },
    )
  }

  const editing = rows.find((r) => r.key === pickingRow)
  const editingAccount = existing.find((a) => a.id === pickingAccount)
  const editingCurrency = editingAccount ? currencyOfExisting(editingAccount) : editing ? currencyOf(editing) : undefined
  const editingIcon = rows.find((r) => r.key === iconRowKey)
  const codeOf = (id: Id) => currencies?.find((c) => c.id === id)?.code ?? ''
  const chip = 'w-14 shrink-0 rounded-md border px-1.5 py-0.5 text-center text-xs text-muted-foreground'
  // balance, currency and action cells keep one width on every row, so the columns line up
  const balanceCell =
    'w-24 shrink-0 rounded-lg border bg-transparent px-2 py-1.5 text-right text-sm tabular-nums outline-none focus-visible:ring-2 focus-visible:ring-ring/50 aria-invalid:border-destructive sm:w-28'
  const actionCell = 'flex w-8 shrink-0 justify-end sm:w-24'


  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-2xl font-normal">{t('onboarding.accounts.heading')}</h2>
      <p className="text-sm text-muted-foreground">{t('onboarding.accounts.subtext')}</p>

      <ul className="mt-4 divide-y divide-border/60 overflow-hidden rounded-xl border bg-card">
        {existing.map((a) => {
          const cur = currencies?.find((c) => c.id === currencyOfExisting(a))
          const off = isHidden(a)
          return (
            <li key={a.id} className="px-3 py-2" data-testid="account-row" data-hidden={off || undefined}>
              <div className="flex items-center gap-2 sm:gap-3">
                <span className={cn(rowTile, off && 'bg-muted text-muted-foreground')}>
                  <EntityIcon name={a.icon} className="text-lg" />
                </span>
                <div className={cn('min-w-0 flex-1 truncate px-1', off && 'text-muted-foreground')}>{a.name}</div>
                {off ? (
                  <span className="w-40 shrink-0 text-right text-xs text-muted-foreground sm:w-[10.25rem]">
                    {t('onboarding.accounts.hidden')}
                  </span>
                ) : (
                  <>
                    <input
                      className={balanceCell}
                      aria-label={t('onboarding.accounts.balance_of', { name: a.name })}
                      inputMode="decimal"
                      placeholder="0"
                      value={balanceInput(a)}
                      aria-invalid={invalidExisting.has(a.id)}
                      onChange={(e) => setBalanceEdits((prev) => ({ ...prev, [a.id]: e.target.value }))}
                    />
                    <button
                      type="button"
                      className={cn(chip, 'hover:bg-accent')}
                      aria-label={t('onboarding.accounts.currency_title')}
                      onClick={() => setPickingAccount(a.id)}
                    >
                      {cur?.code ?? ''}
                    </button>
                  </>
                )}
                <span className={actionCell}>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="-mr-1 text-muted-foreground"
                    aria-label={t(off ? 'onboarding.accounts.show_label' : 'onboarding.accounts.hide_label', { name: a.name })}
                    onClick={() => toggleHidden(a.id)}
                  >
                    {off ? <ArchiveRestore className="size-4" /> : <Archive className="size-4" />}
                    <span className="hidden sm:inline">{t(off ? 'onboarding.accounts.show' : 'onboarding.accounts.hide')}</span>
                  </Button>
                </span>
              </div>
              {invalidExisting.has(a.id) && <p className="mt-1 pl-11 text-sm text-destructive">{t('onboarding.accounts.balance_invalid')}</p>}
            </li>
          )
        })}
        {rows.map((row) => {
          const showName = invalidName.has(row.key) && nameTouched.has(row.key)
          return (
            <li key={row.key} className="px-3 py-2" data-testid="account-row">
              <div className="flex items-center gap-2 sm:gap-3">
                <button
                  type="button"
                  className={cn(rowTile, 'hover:bg-primary/15')}
                  aria-label={t('onboarding.categories_step.icon_label')}
                  onClick={() => setIconRowKey(row.key)}
                >
                  <EntityIcon name={row.icon} className="text-lg" />
                </button>
                <input
                  className={rowInput}
                  aria-label={t('onboarding.accounts.name_label')}
                  placeholder={t('onboarding.accounts.name_label')}
                  autoFocus={row.key === focusKey}
                  value={row.name}
                  aria-invalid={showName}
                  onChange={(e) => rename(row, e.target.value)}
                  onBlur={() => setNameTouched((prev) => new Set(prev).add(row.key))}
                />
                <input
                  className={balanceCell}
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
                <span className={actionCell}>
                  <button
                    type="button"
                    aria-label={t('onboarding.accounts.remove')}
                    className="-mr-1 grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 focus-visible:outline-none"
                    onClick={() => remove(row.key)}
                  >
                    <X className="size-4" />
                  </button>
                </span>
              </div>
              {showName && <p className="mt-1 pl-11 text-sm text-destructive">{t('errors.account.name_length', { min: NAME_MIN, max: NAME_MAX })}</p>}
              {invalid.has(row.key) && <p className="mt-1 pl-11 text-sm text-destructive">{t('onboarding.accounts.balance_invalid')}</p>}
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
        selectableOptions={usableCurrencies(currencies)}
        usedIds={usedIds}
        extraIds={extras}
        mainId={mainId}
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
          {t('onboarding.save_continue')}
        </Button>
      </div>

      <ResponsiveDialog
        open={editingIcon !== undefined}
        onOpenChange={(o) => !o && setIconRowKey(null)}
        title={t('onboarding.categories_step.icon_label')}
      >
        {editingIcon && (
          <IconPicker
            value={editingIcon.icon}
            onChange={(icon) => {
              update(editingIcon.key, { icon, iconPicked: true })
              setIconRowKey(null)
            }}
            aria-label={t('onboarding.categories_step.icon_label')}
          />
        )}
      </ResponsiveDialog>

      <CurrencyListSheet
        open={editing !== undefined || editingAccount !== undefined}
        title={t('onboarding.accounts.currency_title')}
        options={usableCurrencies(currencies, editingCurrency)}
        value={editingCurrency ?? null}
        onClose={() => {
          setPickingRow(null)
          setPickingAccount(null)
        }}
        onPick={(id) => {
          if (editingAccount) {
            if (editingAccount.id === lead?.id) {
              setSharedId(id)
              setPicked((prev) => Object.fromEntries(Object.entries(prev).filter(([k]) => k !== editingAccount.id)))
            } else setPicked((prev) => ({ ...prev, [editingAccount.id]: id }))
            return
          }
          if (!pickingRow) return
          if (!lead && pickingRow === rows[0]?.key) {
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
  const folders = useFolders()
  const day0 = useDay0()
  const [skipping, setSkipping] = useState(false)
  // the lists cached before an import are stale: wait for their refetch before seeding the form
  const settled = useSettled(day0.ready && !folders.isPending && !accounts.isFetching && !folders.isFetching)
  if (accounts.isError) return <StepLoadError error={accounts.error} onRetry={() => void accounts.refetch()} />
  if (folders.isError) return <StepLoadError error={folders.error} onRetry={() => void folders.refetch()} />
  if (!settled) return null
  const hiddenFolders = new Set((folders.data ?? []).filter((f) => f.isVisible === 0).map((f) => f.id))
  const existing = (accounts.data ?? [])
    .filter((a) => a.owner.id === user?.id)
    .map((a) => ({
      id: a.id,
      name: a.name,
      icon: a.icon,
      balance: a.balance,
      currencyId: a.currency.id,
      hidden: a.folderId !== null && hiddenFolders.has(a.folderId),
    }))
  return (
    <AccountsStep
      existing={existing}
      initialRows={day0.draft?.rows}
      initialExtraCurrencyIds={day0.draft?.extraCurrencyIds}
      pending={day0.pending || skipping}
      error={
        day0.error != null ? (
          <p role="alert" className="text-sm text-destructive">
            {apiErrorMessage(day0.error)}
          </p>
        ) : undefined
      }
      onContinue={(rows, extras, main, review) => {
        void day0
          .saveAccounts(rows, extras, main, review)
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
