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
}

const TYPE_SAMPLE = 500
const MAJORITY = 0.8
const SAMPLE_COUNT = 3
const SAMPLE_MAX = 25

const isDate = (v: string) =>
  parseDateCell(v, 'ymd') !== null || parseDateCell(v, 'dmy') !== null || parseDateCell(v, 'mdy') !== null
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
