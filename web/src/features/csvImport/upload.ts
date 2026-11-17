import { v7 as uuidv7 } from 'uuid'
import { createAccount } from '@/api/account'
import type { ImportResultDto } from '@/api/transaction'
import { apiErrorMessage } from '@/lib/apiError'
import { buildCsvText, chunkRows } from '@/lib/csv'
import { defaultAccountIcon } from '@/lib/icons'
import type { PreparedRow, WizardAnswers, buildUpload } from './prepare'

export const CHUNK_SIZE = 500

export type ImportPoster = (
  file: File,
  mapping: Record<string, string | null>,
  fields: Record<string, string>,
) => Promise<ImportResultDto>

export interface ImportResult {
  imported: number
  failed: number
  errors: { message: string; rows: number[] }[]
  failedChunks: number[]
}

type Upload = ReturnType<typeof buildUpload>

export interface RunChunksOptions {
  /** chunk indices to send; all of them when absent */
  chunks?: number[]
  /** rows the browser already refused; counted as failed with their file rows */
  skipped?: PreparedRow[]
  onProgress?: (done: number, total: number) => void
}

// A failed chunk is recorded and the run goes on; the caller retries it with `chunks`.
export async function runChunks(upload: Upload, post: ImportPoster, opts: RunChunksOptions = {}): Promise<ImportResult> {
  const chunks = chunkRows(upload.rows, CHUNK_SIZE)
  const indices = opts.chunks ?? chunks.map((_, i) => i)
  const errorMap = new Map<string, Set<number>>()
  const addError = (message: string, rows: number[]) => {
    const bucket = errorMap.get(message) ?? new Set<number>()
    for (const row of rows) bucket.add(row)
    errorMap.set(message, bucket)
  }

  let imported = 0
  let failed = 0
  const failedChunks: number[] = []

  for (const row of opts.skipped ?? []) {
    failed += 1
    addError(`transactions.import_wizard.skipped.${row.reason ?? 'user_skipped'}`, [row.fileRow])
  }

  for (const [done, i] of indices.entries()) {
    const chunk = chunks[i] ?? []
    // the server numbers rows per chunk file with the header as row 1
    const fileRowOf = (serverRow: number) => upload.fileRows[i * CHUNK_SIZE + (serverRow - 2)]
    const file = new File([buildCsvText(upload.header, chunk)], `chunk_${i}.csv`, { type: 'text/csv' })
    try {
      const result = await post(file, upload.mapping, upload.fields)
      imported += result.imported ?? 0
      failed += result.skipped ?? 0
      for (const [message, rows] of Object.entries(result.errors ?? {})) {
        addError(
          message,
          rows.flatMap((r) => (r === 0 ? [] : (fileRowOf(r) ?? []))),
        )
      }
    } catch (error) {
      failed += chunk.length
      failedChunks.push(i)
      addError(
        `Chunk ${i + 1} failed: ${apiErrorMessage(error)}`,
        chunk.map((_, k) => upload.fileRows[i * CHUNK_SIZE + k]),
      )
    }
    opts.onProgress?.(done + 1, indices.length)
  }

  return {
    imported,
    failed,
    errors: [...errorMap.entries()].map(([message, rows]) => ({ message, rows: [...rows].sort((a, b) => a - b) })),
    failedChunks,
  }
}

/** The run after a "Try the rest again" of `chunks`: the retried chunks' rows count
    by what the retry made of them, everything else stays as the first run left it. */
export function mergeRetry(previous: ImportResult, retry: ImportResult, upload: Upload, chunks: number[]): ImportResult {
  const retriedRows = new Set(chunks.flatMap((i) => upload.fileRows.slice(i * CHUNK_SIZE, (i + 1) * CHUNK_SIZE)))
  const errors = new Map<string, Set<number>>()
  const add = (message: string, rows: number[]) => {
    const bucket = errors.get(message) ?? new Set<number>()
    for (const row of rows) bucket.add(row)
    errors.set(message, bucket)
  }
  for (const error of previous.errors) {
    const kept = error.rows.filter((row) => !retriedRows.has(row))
    if (kept.length > 0 || error.rows.length === 0) add(error.message, kept)
  }
  for (const error of retry.errors) add(error.message, error.rows)
  return {
    imported: previous.imported + retry.imported,
    failed: previous.failed - retriedRows.size + retry.failed,
    errors: [...errors.entries()].map(([message, rows]) => ({ message, rows: [...rows].sort((a, b) => a - b) })),
    failedChunks: retry.failedChunks,
  }
}

// `created` survives retries so a second attempt never creates an account twice.
// `names` gives a per-row account its name as the file writes it; the answers key it lowercased.
export async function ensureAccounts(
  answers: WizardAnswers,
  created: Map<string, string>,
  existingNames: Set<string>,
  names?: ReadonlyMap<string, string>,
): Promise<{ accountId?: string }> {
  const choice = answers.account
  if (choice?.kind === 'new') {
    let id = created.get('new')
    if (!id) {
      id = uuidv7()
      await createAccount({ id, name: choice.name, currencyId: choice.currencyId, balance: '0', icon: defaultAccountIcon, folderId: null })
      created.set('new', id)
    }
    return { accountId: id }
  }
  if (choice?.kind === 'column') {
    for (const [name, currencyId] of Object.entries(choice.currencies)) {
      const key = name.toLowerCase()
      if (existingNames.has(key) || created.has(key)) continue
      const id = uuidv7()
      await createAccount({ id, name: names?.get(key) ?? name, currencyId, balance: '0', icon: defaultAccountIcon, folderId: null })
      created.set(key, id)
    }
  }
  return {}
}
