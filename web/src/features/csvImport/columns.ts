import { parseAmountCell } from './amounts'
import { parseDateCell } from './dates'
import type { ParsedFile } from './parse'

export type ColumnType = 'date' | 'number' | 'text' | 'empty'

export interface ColumnProfile {
  index: number
  name: string
  type: ColumnType
  negatives: boolean
  samples: string[]
  /** every value reads like a bank's own reference (a UUID, a long code or number), not anything a person would pick */
  internal: boolean
  /** cells with a value */
  filled: number
  /** different values, compared as names are (trimmed, any case) */
  distinct: number
  /** the average length of a value, in characters */
  averageLength: number
  /** every value is a three-letter code such as CAD or EUR */
  codes: boolean
  /** the header is also one of the column's own values (in any case) */
  headerInValues: boolean
}

const TYPE_SAMPLE = 500
const MAJORITY = 0.8
const SAMPLE_COUNT = 3
const SAMPLE_MAX = 25

const parsesAsDate = (v: string) => {
  const hits = (['ymd', 'dmy', 'mdy'] as const).map((order) => parseDateCell(v, order))
  return hits.filter((h) => h !== null)
}

const isDate = (v: string) => {
  const hits = parsesAsDate(v)
  if (!/^\d+$/.test(v)) return hits.length > 0
  if (v.length !== 8) return false
  return hits.some((h) => {
    const year = Number(h.date.slice(0, 4))
    return year >= 1990 && year <= 2100
  })
}
const isNumber = (v: string) => parseAmountCell(v, '.') !== null || parseAmountCell(v, ',') !== null

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
const CODE = /^[A-Za-z0-9_\-./]{10,}$/
// a run of digits, not digits spread out: "02-Jan-2026" or "2026.01.02." is a date the reader missed
const DIGIT_RUN = /\d{6,}/
const LONG_INTEGER = /^\d{9,}$/
const CURRENCY_CODE = /^(?:[A-Z]{3}|[a-z]{3})$/

function looksInternal(type: ColumnType, values: string[]): boolean {
  if (values.length === 0 || type === 'date' || type === 'empty') return false
  if (type === 'number') return values.every((v) => LONG_INTEGER.test(v))
  return values.every((v) => UUID.test(v) || (CODE.test(v) && DIGIT_RUN.test(v)))
}

export function profileColumns(file: ParsedFile): ColumnProfile[] {
  return file.header.map((name, index) => {
    const values: string[] = []
    for (const row of file.rows) {
      const v = (row[index] ?? '').trim()
      if (v !== '') values.push(v)
    }
    const typed = values.slice(0, TYPE_SAMPLE)
    let type: ColumnType = 'text'
    if (values.length === 0) {
      type = 'empty'
    } else if (typed.filter(isDate).length >= MAJORITY * typed.length) {
      type = 'date'
    } else if (typed.filter(isNumber).length >= MAJORITY * typed.length) {
      type = 'number'
    }
    const samples = [...new Set(values)]
      .slice(0, SAMPLE_COUNT)
      .map((v) => (v.length > SAMPLE_MAX ? `${v.slice(0, SAMPLE_MAX)}…` : v))
    const negatives = type === 'number' && typed.some((v) => /^[-−(]/.test(v) || /-$/.test(v))
    return {
      index,
      name,
      type,
      negatives,
      samples,
      internal: looksInternal(type, typed),
      filled: values.length,
      distinct: new Set(values.map((v) => v.toLowerCase())).size,
      averageLength: values.length === 0 ? 0 : values.reduce((sum, v) => sum + v.length, 0) / values.length,
      codes: values.length > 0 && values.every((v) => CURRENCY_CODE.test(v)),
      headerInValues: name.trim() !== '' && typed.some((v) => v.toLowerCase() === name.trim().toLowerCase()),
    }
  })
}

// The amount reader strips letters at the edges, so "Column 3", "Amount (2)" (names parse.ts
// gives a blank or a repeated header) and "Memo 2" would read as numbers. A header reads as one
// only when nothing but a currency sign or one upper-case code ("-12.50 EUR") is left besides it.
const LETTERS = /\p{L}/u
const CURRENCY_MARK = /\p{Sc}|\b[A-Z]{3}\b/gu
const isNumberHeader = (name: string) => !LETTERS.test(name.replace(CURRENCY_MARK, '')) && isNumber(name)
const PLACEHOLDER = /^Column \d+$/

/** some header is itself a date or a number: the file has no header row, its first transaction stands in */
export function headerHasValues(profiles: ColumnProfile[]): boolean {
  return profiles.some(({ name }) => isDate(name) || isNumberHeader(name))
}

// A file with no header row has its first transaction read as the header; those cells never go
// to the AI. With no word lists, only the cells' content can tell: a header that is a date, a
// number, a bank reference or one of its own column's values is a transaction's cell, and a date
// or amount column without a header at all fails closed too. A first row whose date and amount
// cells are both unreadable words ("unknown,Amount,Coffee") cannot be told from a header by
// content and passes.
export function headerLooksLikeData(profiles: ColumnProfile[]): boolean {
  if (headerHasValues(profiles)) return true
  return profiles.some((p) => {
    const name = p.name.trim()
    const blank = name === '' || PLACEHOLDER.test(name)
    if (blank) return p.type === 'date' || p.type === 'number'
    return p.headerInValues || looksInternal('text', [name]) || LONG_INTEGER.test(name)
  })
}
