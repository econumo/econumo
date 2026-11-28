import type { ParsedFile } from './parse'
import type { AccountChoice } from './prepare'
import { nameKey } from './wizard'

type Currency = { id: string; code: string }

const codeOf = (cell: string | undefined) => (cell ?? '').trim().toUpperCase()
type PerRowChoice = Extract<AccountChoice, { kind: 'column' }>

export interface AccountCurrency {
  currencyId: string
  /** the one code the account's rows name in the currency column, when the app has no such currency */
  unknownCode: string | null
  /** set by the user's pick or by the file's currency column, not by the default */
  explicit: boolean
}

/** Each account's currency, by lowercased name: the user's pick for it, else the one known code its
    own rows name, else the choice's currency for new accounts. */
export function accountCurrencies(file: ParsedFile, choice: PerRowChoice, currencies: ReadonlyArray<Currency>): Map<string, AccountCurrency> {
  const idByCode = new Map(currencies.map((c) => [c.code.toUpperCase(), c.id]))
  const codes = new Map<string, Set<string>>()
  for (const row of file.rows) {
    const key = nameKey(row[choice.column] ?? '')
    if (key === '') continue
    const found = codes.get(key) ?? new Set<string>()
    codes.set(key, found)
    const code = choice.currencyColumn === null ? '' : codeOf(row[choice.currencyColumn])
    if (code !== '') found.add(code)
  }
  const result = new Map<string, AccountCurrency>()
  for (const [key, found] of codes) {
    const [code] = found.size === 1 ? found : [null]
    const known = code ? idByCode.get(code) : undefined
    const override = Object.hasOwn(choice.overrides, key) ? choice.overrides[key] : undefined
    result.set(key, { currencyId: override ?? known ?? choice.currencyId, unknownCode: code && !known ? code : null, explicit: (override ?? known) !== undefined })
  }
  return result
}

/** the choice with the currencies the user or the file fixed; the default is never copied into an account */
export function withAccountCurrencies(file: ParsedFile, choice: PerRowChoice, currencies: ReadonlyArray<Currency>): PerRowChoice {
  const resolved = accountCurrencies(file, choice, currencies)
  // fromEntries, not assignment: a name like "__proto__" must stay a plain key
  return { ...choice, currencies: Object.fromEntries([...resolved].filter(([, { explicit }]) => explicit).map(([key, { currencyId }]) => [key, currencyId])) }
}
