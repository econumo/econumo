import type { ParsedFile } from './parse'
import type { AccountChoice } from './prepare'
import { nameKey } from './wizard'

type Currency = { id: string; code: string }

const codeOf = (cell: string | undefined) => (cell ?? '').trim().toUpperCase()
const CODE = /^[A-Z]{3}$/

function everyFilledCell(file: ParsedFile, column: number, ok: (code: string) => boolean): boolean {
  let filled = 0
  for (const row of file.rows) {
    const code = codeOf(row[column])
    if (code === '') continue
    if (!ok(code)) return false
    filled++
  }
  return filled > 0
}

/**
 * The column naming each row's currency, or null: the first whose every filled cell is a
 * currency code the app knows, else a "currency" column of three-letter codes, so a code
 * this instance lacks (an Econumo export's "CAD") is still read and can be pointed out.
 */
export function currencyColumn(file: ParsedFile, currencies: ReadonlyArray<Currency>, taken: ReadonlySet<number> = new Set()): number | null {
  const known = new Set(currencies.map((c) => c.code.toUpperCase()))
  for (let column = 0; column < file.header.length; column++) {
    if (!taken.has(column) && everyFilledCell(file, column, (code) => known.has(code))) return column
  }
  for (let column = 0; column < file.header.length; column++) {
    if (taken.has(column)) continue
    if (file.header[column].toLowerCase().includes('currency') && everyFilledCell(file, column, (code) => CODE.test(code))) return column
  }
  return null
}

/** the one currency code these rows name in the currency column, or null */
export function rowsCurrencyCode(rows: ReadonlyArray<string[]>, column: number | null): string | null {
  if (column === null) return null
  const codes = new Set(rows.map((row) => codeOf(row[column])).filter((code) => code !== ''))
  if (codes.size !== 1) return null
  const [code] = codes
  return code
}

/** the currency these rows are in, when the file's currency column names exactly one the app knows */
export function rowsCurrencyId(
  rows: ReadonlyArray<string[]>,
  column: number | null,
  currencies: ReadonlyArray<Currency>,
): string | null {
  const code = rowsCurrencyCode(rows, column)
  if (code === null) return null
  return currencies.find((c) => c.code.toUpperCase() === code)?.id ?? null
}

/** the code these rows name when the app has no such currency, for a note beside the fallback */
export function unknownRowsCurrency(
  rows: ReadonlyArray<string[]>,
  column: number | null,
  currencies: ReadonlyArray<Currency>,
): string | null {
  const code = rowsCurrencyCode(rows, column)
  if (code === null || currencies.some((c) => c.code.toUpperCase() === code)) return null
  return code
}

/** the one currency the whole file is in, read from its currency column */
export function fileCurrencyId(file: ParsedFile, currencies: ReadonlyArray<Currency>): string | null {
  return rowsCurrencyId(file.rows, currencyColumn(file, currencies), currencies)
}

type PerRowChoice = Extract<AccountChoice, { kind: 'column' }>

export interface AccountCurrency {
  currencyId: string
  /** the one code the account's rows name in the currency column, when the app has no such currency */
  unknownCode: string | null
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
    result.set(key, { currencyId: override ?? known ?? choice.currencyId, unknownCode: code && !known ? code : null })
  }
  return result
}

/** the choice with every account's final currency filled in */
export function withAccountCurrencies(file: ParsedFile, choice: PerRowChoice, currencies: ReadonlyArray<Currency>): PerRowChoice {
  const resolved = accountCurrencies(file, choice, currencies)
  // fromEntries, not assignment: a name like "__proto__" must stay a plain key
  return { ...choice, currencies: Object.fromEntries([...resolved].map(([key, { currencyId }]) => [key, currencyId])) }
}
