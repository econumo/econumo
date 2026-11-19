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
    return { index, name, type, negatives, samples }
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
// a date column or a money column is headed by a word that names it, which a transaction's own
// cell in that column practically never is.
export function headerLooksLikeData(profiles: ColumnProfile[]): boolean {
  if (profiles.some(({ name }) => isDate(name) || isNumber(name))) return true
  const named = profiles.some(
    ({ name, type }) =>
      (type === 'date' && DATE_NAMES.some((s) => names(name, s))) ||
      (type === 'number' && MONEY_NAMES.some((s) => names(name, s))),
  )
  return !named
}
