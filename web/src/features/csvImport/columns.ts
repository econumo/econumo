import { parseAmountCell } from './amounts'
import { parseDateCell } from './dates'
import { SYNONYMS } from './guess'
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
    return { index, name, type, negatives, samples, internal: looksInternal(type, typed) }
  })
}

const HAN = /\p{Script=Han}/u
const words = (s: string) => ' ' + s.toLowerCase().split(/[^\p{L}\p{N}]+/u).filter(Boolean).join(' ') + ' '

// Whole words, so "Date (UTC)" names a date and "update fee" does not; Chinese has no spaces.
const names = (header: string, synonym: string) =>
  HAN.test(synonym) ? header.toLowerCase().includes(synonym) : words(header).includes(words(synonym))

const DATE_NAMES = SYNONYMS.date
const MONEY_NAMES = [...SYNONYMS.amount, ...SYNONYMS.amountIn, ...SYNONYMS.amountOut]

// A file with no header row has its first transaction read as the header; those names are
// cell values, which never go to the AI. A row that only fails to parse ("someday,n/a,medical
// memo") looks like headers too, so this fails closed: the header row counts as one only when
// both the dates and the money are headed by words that name them (one date column named when
// the file has date columns, one number column named when it has number columns), which a
// transaction's own cells practically never are: a stray word in one of them ("unknown,Amount,
// Coffee") still leaves the other unnamed. A number is read only from a name without letters:
// the amount reader strips them, so "Column 3", "Amount (2)" (names parse.ts gives a blank or a
// repeated header) and "Memo 2" would read as numbers; a cell like "-12.50 EUR" still fails the
// word check.
const HAS_LETTER = /\p{L}/u

/** some header is itself a date or a number: the file has no header row, its first transaction stands in */
export function headerHasValues(profiles: ColumnProfile[]): boolean {
  return profiles.some(({ name }) => isDate(name) || (!HAS_LETTER.test(name) && isNumber(name)))
}

export function headerLooksLikeData(profiles: ColumnProfile[]): boolean {
  if (headerHasValues(profiles)) return true
  const kinds = [
    { type: 'date', words: DATE_NAMES },
    { type: 'number', words: MONEY_NAMES },
  ].filter(({ type }) => profiles.some((p) => p.type === type))
  const named = (type: string, words: string[]) => profiles.some((p) => p.type === type && words.some((s) => names(p.name, s)))
  return kinds.length === 0 || !kinds.every(({ type, words }) => named(type, words))
}
