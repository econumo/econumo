import type { ParsedFile } from './parse'

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
export function currencyColumn(file: ParsedFile, currencies: ReadonlyArray<Currency>): number | null {
  const known = new Set(currencies.map((c) => c.code.toUpperCase()))
  for (let column = 0; column < file.header.length; column++) {
    if (everyFilledCell(file, column, (code) => known.has(code))) return column
  }
  for (let column = 0; column < file.header.length; column++) {
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
