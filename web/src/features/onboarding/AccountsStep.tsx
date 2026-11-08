import { useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { v7 as uuidv7 } from 'uuid'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { EntityIcon } from '@/components/EntityIcon'
import { useCurrencies, useCurrencyRates } from '@/features/currencies/queries'
import { selectableCurrencies } from '@/features/currencies/selectable'
import { useUserData, userCurrencyId } from '@/features/user/queries'
import { guessCurrencyCode } from '@/lib/regionCurrency'
import type { Id } from '@/api/types'
import type { AccountRow } from './draft'
import { guessAccountIcon, looksLikeSavings } from './defaults'
import { parseBalance } from './balance'
import { nameLengthValid, NAME_MAX, NAME_MIN } from './names'
import { CurrenciesBlock, CurrencyListSheet } from './CurrenciesBlock'

// currencyId '' = the row follows the default currency until the user picks one
type FormRow = AccountRow

interface AccountsStepProps {
  initialRows?: AccountRow[]
  initialExtraCurrencyIds?: Id[]
  createdKeys?: ReadonlySet<string>
  onContinue: (rows: AccountRow[], extraCurrencyIds: Id[], mainCurrencyId: Id) => void
  onSkip: () => void
  pending: boolean
  error?: ReactNode
}

export function AccountsStep({ initialRows, initialExtraCurrencyIds, createdKeys, onContinue, onSkip, pending, error }: AccountsStepProps) {
  const { t, i18n } = useTranslation()
  const { data: currencies } = useCurrencies()
  const { data: rates } = useCurrencyRates()
  const { data: user } = useUserData()

  const emptyRow = (): FormRow => ({ key: uuidv7(), name: '', balance: '', currencyId: '', savings: false, icon: 'credit_card' })
  const [rows, setRows] = useState<FormRow[]>(() => {
    const seeded = initialRows && initialRows.length > 0 ? initialRows : []
    return seeded.length > 0 && seeded[seeded.length - 1].name.trim() === '' ? seeded : [...seeded, emptyRow()]
  })
  const [extraIds, setExtraIds] = useState<Id[]>(initialExtraCurrencyIds ?? [])
  const [mainOverride, setMainOverride] = useState<Id | null>(null)
  const [pickingRow, setPickingRow] = useState<string | null>(null)
  const savingsTouched = useRef(new Set<string>())
  // a name still being typed is short on its way to valid, so its error waits for blur
  const [nameTouched, setNameTouched] = useState<ReadonlySet<string>>(new Set())

  const defaultId = useMemo(() => {
    const list = currencies ?? []
    const guessed = guessCurrencyCode(navigator.languages ?? [])
    const byGuess = guessed ? list.find((c) => c.code === guessed && selectableCurrencies([c]).length > 0) : undefined
    return byGuess?.id ?? userCurrencyId(user) ?? list[0]?.id ?? ''
  }, [currencies, user])

  const words = () => ({ cash: t('onboarding.words.cash'), savings: t('onboarding.words.savings') })
  const currencyOf = (row: FormRow) => row.currencyId || defaultId
  const isCreated = (row: FormRow) => createdKeys?.has(row.key) ?? false

  const update = (key: string, patch: Partial<FormRow>) => setRows((prev) => prev.map((r) => (r.key === key ? { ...r, ...patch } : r)))

  const rename = (row: FormRow, name: string) => {
    const patch: Partial<FormRow> = { name, icon: guessAccountIcon(name, words()) }
    if (!savingsTouched.current.has(row.key)) patch.savings = looksLikeSavings(name, words().savings)
    setRows((prev) => {
      const next = prev.map((r) => (r.key === row.key ? { ...r, ...patch } : r))
      return next[next.length - 1].name.trim() !== '' ? [...next, emptyRow()] : next
    })
  }

  const remove = (key: string) =>
    setRows((prev) => {
      const next = prev.filter((r) => r.key !== key)
      return next.length === 0 || next[next.length - 1].name.trim() !== '' ? [...next, emptyRow()] : next
    })

  const active = rows.filter((r) => r.name.trim() !== '')
  const counted = active.length > 0 ? active : rows
  const usedIds = [...new Set(counted.map(currencyOf).filter(Boolean))]
  const counts = new Map<Id, number>()
  for (const r of counted) counts.set(currencyOf(r), (counts.get(currencyOf(r)) ?? 0) + 1)
  let computedMain: Id | null = null
  for (const id of usedIds) if (computedMain === null || (counts.get(id) ?? 0) > (counts.get(computedMain) ?? 0)) computedMain = id
  const extras = extraIds.filter((id) => !usedIds.includes(id))
  const selectedIds = [...usedIds, ...extras]
  const mainId = mainOverride && selectedIds.includes(mainOverride) ? mainOverride : computedMain

  const baseId = rates?.[0]?.baseCurrencyId ?? userCurrencyId(user)
  const missingRateIds = rates
    ? selectedIds.filter((id) => id !== baseId && !rates.some((r) => r.currencyId === id))
    : []

  const parse = (raw: string) => parseBalance(raw, i18n.language)
  const invalid = new Set(active.filter((r) => parse(r.balance) === null).map((r) => r.key))
  const invalidName = new Set(active.filter((r) => !isCreated(r) && !nameLengthValid(r.name)).map((r) => r.key))
  const canContinue = active.length > 0 && invalid.size === 0 && invalidName.size === 0 && mainId !== null && !pending

  const submit = () => {
    if (!canContinue || !mainId) return
    onContinue(
      active.map((r) => ({ ...r, name: r.name.trim(), balance: parse(r.balance) ?? '0', currencyId: currencyOf(r) })),
      extras,
      mainId,
    )
  }

  const editing = rows.find((r) => r.key === pickingRow)

  return (
    <div className="space-y-6">
      <header className="space-y-1">
        <h1 className="text-2xl">{t('onboarding.accounts.heading')}</h1>
        <p className="text-muted-foreground">{t('onboarding.accounts.subtext')}</p>
      </header>

      <ul className="space-y-3">
        {rows.map((row) => {
          const created = isCreated(row)
          const cur = currencies?.find((c) => c.id === currencyOf(row))
          const onlyRow = rows.length === 1
          return (
            <li key={row.key} className="space-y-3 rounded-xl border bg-card p-3" data-testid="account-row">
              <div className="flex items-center gap-2">
                <EntityIcon name={row.icon} className="shrink-0 text-2xl text-muted-foreground" />
                <Input
                  aria-label={t('onboarding.accounts.name_label')}
                  placeholder={t('onboarding.accounts.name_label')}
                  value={row.name}
                  readOnly={created}
                  aria-invalid={invalidName.has(row.key) && nameTouched.has(row.key)}
                  onChange={(e) => rename(row, e.target.value)}
                  onBlur={() => setNameTouched((prev) => new Set(prev).add(row.key))}
                />
                {!created && !onlyRow && (
                  <Button type="button" variant="ghost" size="icon" aria-label={t('onboarding.accounts.remove')} onClick={() => remove(row.key)}>
                    <X className="size-4" />
                  </Button>
                )}
              </div>
              {invalidName.has(row.key) && nameTouched.has(row.key) && (
                <p className="text-sm text-destructive">{t('errors.account.name_length', { min: NAME_MIN, max: NAME_MAX })}</p>
              )}
              <div className="flex items-center gap-2">
                <Input
                  aria-label={t('onboarding.accounts.balance_label')}
                  placeholder="0"
                  value={row.balance}
                  readOnly={created}
                  aria-invalid={invalid.has(row.key)}
                  onChange={(e) => update(row.key, { balance: e.target.value })}
                />
                <Button
                  type="button"
                  variant="outline"
                  aria-label={t('onboarding.accounts.currency_title')}
                  disabled={created}
                  onClick={() => setPickingRow(row.key)}
                >
                  {cur?.code ?? ''}
                </Button>
              </div>
              {invalid.has(row.key) && <p className="text-sm text-destructive">{t('onboarding.accounts.balance_invalid')}</p>}
              <label className="flex items-center justify-between gap-2 text-sm text-muted-foreground">
                {t('onboarding.accounts.savings')}
                <Switch
                  checked={row.savings}
                  onCheckedChange={(v) => {
                    savingsTouched.current.add(row.key)
                    update(row.key, { savings: v })
                  }}
                />
              </label>
            </li>
          )
        })}
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

      <div className="flex flex-col items-center gap-3">
        {error}
        <Button type="button" className="w-full" disabled={!canContinue} onClick={submit}>
          {t('onboarding.accounts.continue')}
        </Button>
        <Button type="button" variant="link" disabled={pending} onClick={onSkip}>
          {t('onboarding.accounts.skip')}
        </Button>
      </div>

      <CurrencyListSheet
        open={editing !== undefined}
        title={t('onboarding.accounts.currency_title')}
        options={selectableCurrencies(currencies, editing ? currencyOf(editing) : undefined)}
        value={editing ? currencyOf(editing) : null}
        onClose={() => setPickingRow(null)}
        onPick={(id) => pickingRow && update(pickingRow, { currencyId: id })}
      />
    </div>
  )
}
