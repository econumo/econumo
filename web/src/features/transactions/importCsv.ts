import { apiErrorMessage } from '@/lib/apiError'
import { buildCsvText, chunkRows, parseCsv, remapRow } from '@/lib/csv'
import type { ImportResultDto } from '@/api/transaction'

export const CHUNK_SIZE = 500

export const FIELD_KEYS = ['account', 'date', 'amount', 'amountInflow', 'amountOutflow', 'category', 'description', 'payee', 'tag'] as const
export type FieldKey = (typeof FIELD_KEYS)[number]

export interface CsvAnalysis {
  header: string[]
  rows: string[][]
  /** first non-empty value per column, truncated for dropdown labels */
  samples: Record<string, string>
}

const SAMPLE_SCAN_LIMIT = 500
const SAMPLE_MAX_LENGTH = 25

export function analyzeCsv(text: string): CsvAnalysis {
  const { header, rows } = parseCsv(text)
  const samples: Record<string, string> = {}
  for (const [index, column] of header.entries()) {
    for (const row of rows.slice(0, SAMPLE_SCAN_LIMIT)) {
      const value = (row[index] ?? '').trim()
      if (value !== '') {
        samples[column] = value.length > SAMPLE_MAX_LENGTH ? `${value.slice(0, SAMPLE_MAX_LENGTH)}…` : value
        break
      }
    }
  }
  return { header, rows, samples }
}

export type AmountMode = 'single' | 'dual'

export interface FieldModes {
  account: 'csv_column' | 'existing'
  date: 'csv_column' | 'manual'
  category: 'csv_column' | 'existing'
  description: 'csv_column' | 'manual'
  payee: 'csv_column' | 'existing'
  tag: 'csv_column' | 'existing'
}

export interface ImportSelection {
  modes: FieldModes
  amountMode: AmountMode
  columns: Record<FieldKey, string | null>
  fixed: {
    accountId: string | null
    date: string
    categoryId: string | null
    description: string
    payeeId: string | null
    tagId: string | null
  }
}

export function defaultSelection(): ImportSelection {
  return {
    modes: { account: 'csv_column', date: 'csv_column', category: 'csv_column', description: 'csv_column', payee: 'csv_column', tag: 'csv_column' },
    amountMode: 'single',
    columns: { account: null, date: null, amount: null, amountInflow: null, amountOutflow: null, category: null, description: null, payee: null, tag: null },
    fixed: { accountId: null, date: '', categoryId: null, description: '', payeeId: null, tagId: null },
  }
}

// Vue matches lowercased header names against the translated field labels; a
// column is claimed once, exact matches win over substring ones.
export function autoDetect(header: string[], labels: Record<FieldKey, string>): { columns: Record<FieldKey, string | null>; amountMode: AmountMode } {
  const columns = defaultSelection().columns
  const claimed = new Set<string>()

  const claim = (field: FieldKey, matcher: (column: string, label: string) => boolean) => {
    if (columns[field]) return
    const label = labels[field].toLowerCase()
    for (const column of header) {
      if (claimed.has(column)) continue
      const col = column.trim().toLowerCase()
      if (matcher(col, label)) {
        columns[field] = column
        claimed.add(column)
        return
      }
    }
  }

  for (const field of FIELD_KEYS) {
    claim(field, (col, label) => col === label)
  }
  for (const field of FIELD_KEYS) {
    claim(field, (col, label) => col.length >= 2 && (col.includes(label) || label.includes(col)))
  }

  const amountMode: AmountMode = columns.amountInflow && columns.amountOutflow ? 'dual' : 'single'
  if (amountMode === 'dual') {
    columns.amount = null
  }
  return { columns, amountMode }
}

const DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/

function isPlausibleDate(value: string): boolean {
  if (!DATE_PATTERN.test(value)) return false
  const [, month, day] = value.split('-').map(Number)
  return month >= 1 && month <= 12 && day >= 1 && day <= 31
}

export function selectionValid(sel: ImportSelection): boolean {
  const accountOk = sel.modes.account === 'csv_column' ? sel.columns.account !== null : sel.fixed.accountId !== null
  const dateOk = sel.modes.date === 'csv_column' ? sel.columns.date !== null : isPlausibleDate(sel.fixed.date.trim())
  const amountOk =
    sel.amountMode === 'single'
      ? sel.columns.amount !== null
      : sel.columns.amountInflow !== null && sel.columns.amountOutflow !== null
  return accountOk && dateOk && amountOk
}

// mapping always carries all 9 keys (column name or null); fixed/manual values
// travel as separate multipart fields instead
export function buildImportPayload(sel: ImportSelection): { mapping: Record<FieldKey, string | null>; fields: Record<string, string> } {
  const mapping: Record<FieldKey, string | null> = { ...defaultSelection().columns }
  mapping.account = sel.modes.account === 'csv_column' ? sel.columns.account : null
  mapping.date = sel.modes.date === 'csv_column' ? sel.columns.date : null
  mapping.amount = sel.amountMode === 'single' ? sel.columns.amount : null
  mapping.amountInflow = sel.amountMode === 'dual' ? sel.columns.amountInflow : null
  mapping.amountOutflow = sel.amountMode === 'dual' ? sel.columns.amountOutflow : null
  mapping.category = sel.modes.category === 'csv_column' ? sel.columns.category : null
  mapping.description = sel.modes.description === 'csv_column' ? sel.columns.description : null
  mapping.payee = sel.modes.payee === 'csv_column' ? sel.columns.payee : null
  mapping.tag = sel.modes.tag === 'csv_column' ? sel.columns.tag : null

  const fields: Record<string, string> = {}
  if (sel.modes.account === 'existing' && sel.fixed.accountId) fields.accountId = sel.fixed.accountId
  if (sel.modes.date === 'manual' && sel.fixed.date.trim()) fields.date = sel.fixed.date.trim()
  if (sel.modes.category === 'existing' && sel.fixed.categoryId) fields.categoryId = sel.fixed.categoryId
  if (sel.modes.description === 'manual' && sel.fixed.description.trim()) fields.description = sel.fixed.description.trim()
  if (sel.modes.payee === 'existing' && sel.fixed.payeeId) fields.payeeId = sel.fixed.payeeId
  if (sel.modes.tag === 'existing' && sel.fixed.tagId) fields.tagId = sel.fixed.tagId
  return { mapping, fields }
}

export interface AggregatedImportResult {
  imported: number
  failed: number
  errors: { message: string; rows: number[] }[]
}

export type ImportPoster = (
  file: File,
  mapping: Record<string, string | null>,
  fields: Record<string, string>,
) => Promise<ImportResultDto>

// Chunked sequential upload with row numbers remapped back to the original
// file; a failed chunk counts its rows as failed but never aborts the run.
export async function runImport(
  analysis: CsvAnalysis,
  selection: ImportSelection,
  post: ImportPoster,
  onProgress?: (done: number, total: number) => void,
): Promise<AggregatedImportResult> {
  const { mapping, fields } = buildImportPayload(selection)
  const chunks = analysis.rows.length > 0 ? chunkRows(analysis.rows, CHUNK_SIZE) : [[] as string[][]]
  const errorMap = new Map<string, Set<number>>()
  const addError = (message: string, rows: number[]) => {
    const bucket = errorMap.get(message) ?? new Set<number>()
    for (const row of rows) bucket.add(row)
    if (rows.length === 0 && !errorMap.has(message)) errorMap.set(message, bucket)
    errorMap.set(message, bucket)
  }

  let imported = 0
  let failed = 0
  for (const [i, chunk] of chunks.entries()) {
    const file = new File([buildCsvText(analysis.header, chunk)], `chunk_${i}.csv`, { type: 'text/csv' })
    try {
      const result = await post(file, mapping, fields)
      imported += result.imported ?? 0
      failed += result.skipped ?? 0
      for (const [message, rows] of Object.entries(result.errors ?? {})) {
        addError(message, rows.map((row) => remapRow(row, i, CHUNK_SIZE)))
      }
    } catch (error) {
      failed += chunk.length
      addError(`Chunk ${i + 1} failed: ${apiErrorMessage(error)}`, [])
    }
    onProgress?.(i + 1, chunks.length)
  }

  return {
    imported,
    failed,
    errors: [...errorMap.entries()].map(([message, rows]) => ({ message, rows: [...rows].sort((a, b) => a - b) })),
  }
}
