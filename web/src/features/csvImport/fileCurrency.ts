import type { ParsedFile } from './parse'

type Currency = { id: string; code: string }

const codeOf = (cell: string | undefined) => (cell ?? '').trim().toUpperCase()

/** the column whose every filled cell is a currency code the app knows, or null */
export function currencyColumn(file: ParsedFile, currencies: ReadonlyArray<Currency>): number | null {
  const known = new Set(currencies.map((c) => c.code.toUpperCase()))
  for (let column = 0; column < file.header.length; column++) {
    let filled = 0
    let allKnown = true
    for (const row of file.rows) {
      const code = codeOf(row[column])
      if (code === '') continue
      filled++
      if (!known.has(code)) {
        allKnown = false
        break
      }
    }
    if (filled > 0 && allKnown) return column
  }
  return null
}

/** the currency these rows are in, when the file's currency column names exactly one */
export function rowsCurrencyId(
  rows: ReadonlyArray<string[]>,
  column: number | null,
  currencies: ReadonlyArray<Currency>,
): string | null {
  if (column === null) return null
  const codes = new Set(rows.map((row) => codeOf(row[column])).filter((code) => code !== ''))
  if (codes.size !== 1) return null
  const [code] = codes
  return currencies.find((c) => c.code.toUpperCase() === code)?.id ?? null
}

/** the one currency the whole file is in, read from its currency column */
export function fileCurrencyId(file: ParsedFile, currencies: ReadonlyArray<Currency>): string | null {
  return rowsCurrencyId(file.rows, currencyColumn(file, currencies), currencies)
}
