import { isAxiosError } from 'axios'
import { v7 as uuidv7 } from 'uuid'
import { createAccount, getFolderList } from '@/api/account'
import type { WriteOptions } from '@/api/client'
import type { ImportResultDto } from '@/api/transaction'
import { apiErrorMessage } from '@/lib/apiError'
import { buildCsvText, chunkRows } from '@/lib/csv'
import { defaultAccountIcon } from '@/lib/icons'
import type { PreparedRow, WizardAnswers, buildUpload } from './prepare'
import { newAccountFolderId } from './wizard'

export const CHUNK_SIZE = 500

export type ImportPoster = (
  file: File,
  mapping: Record<string, string | null>,
  fields: Record<string, string>,
  opts?: WriteOptions,
) => Promise<ImportResultDto>

export interface ImportResult {
  imported: number
  failed: number
  errors: { message: string; rows: number[] }[]
  failedChunks: number[]
}

type Upload = ReturnType<typeof buildUpload>

// The server's answer to a key it has already seen: the operation landed, the
// response that said so was lost.
function replayOf(error: unknown): { resourceId?: string } | null {
  if (!isAxiosError<{ resourceId?: string }>(error) || error.response?.status !== 409) return null
  return { resourceId: error.response.data?.resourceId || undefined }
}

// One Idempotency-Key per chunk for as long as the upload lives, so "Try the rest
// again" of a chunk the server committed is answered as a replay, not imported twice.
const chunkKeys = new WeakMap<Upload, string[]>()

function chunkKey(upload: Upload, index: number): string {
  const keys = chunkKeys.get(upload) ?? []
  chunkKeys.set(upload, keys)
  keys[index] ??= uuidv7()
  return keys[index]
}

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
      const result = await post(file, upload.mapping, upload.fields, { idempotencyKey: chunkKey(upload, i) })
      const rowsOfChunk = chunk.map((_, k) => upload.fileRows[i * CHUNK_SIZE + k])
      // An error naming no row refused the whole chunk (an account it may not write into, a
      // bad override): nothing landed, so every row failed. Sending it again is refused again.
      const refusal = Object.entries(result.errors ?? {}).find(([, rows]) => rows.length === 0)
      if (refusal && !(result.imported ?? 0)) {
        failed += chunk.length
        addError(refusal[0], rowsOfChunk)
        opts.onProgress?.(done + 1, indices.length)
        continue
      }
      imported += result.imported ?? 0
      failed += result.skipped ?? 0
      for (const [message, rows] of Object.entries(result.errors ?? {})) {
        addError(
          message,
          rows.flatMap((r) => (r === 0 ? [] : (fileRowOf(r) ?? []))),
        )
      }
    } catch (error) {
      if (replayOf(error)) {
        // the replay carries no result, so the rows the server skipped that time are unknown
        imported += chunk.length
      } else {
        failed += chunk.length
        failedChunks.push(i)
        addError(
          `Chunk ${i + 1} failed: ${apiErrorMessage(error)}`,
          chunk.map((_, k) => upload.fileRows[i * CHUNK_SIZE + k]),
        )
      }
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

// Distinct prefixes so the single new account never answers for a per-row account named "new".
const NEW_ACCOUNT = 'new'
const PER_ROW = 'row:'

type Attempt = { requestId: string; idempotencyKey: string }

const attemptsByRun = new WeakMap<Map<string, string>, Map<string, Attempt>>()

function attemptsOf(created: Map<string, string>) {
  const attempts = attemptsByRun.get(created) ?? new Map<string, Attempt>()
  attemptsByRun.set(created, attempts)
  return attempts
}

// `created` survives retries so a second attempt never creates an account twice; each
// account keeps its request id and Idempotency-Key across attempts, so one whose response
// was lost is answered with its id instead of being made again.
// `names` gives a per-row account its name as the file writes it; the answers key it lowercased.
// `folderId` is the user's first folder: the server invents one only for a user who has none
// and refuses a blank folder otherwise, so later accounts follow the first into its folder. With
// no folder known once an account exists (its create was replayed, or a stale folder list from
// an earlier attempt), the folder the server made is read back before the next create.
export async function ensureAccounts(
  answers: WizardAnswers,
  created: Map<string, string>,
  existingNames: Set<string>,
  names?: ReadonlyMap<string, string>,
  folderId: string | null = null,
): Promise<{ accountId?: string }> {
  let folder = folderId
  const attempts = attemptsOf(created)
  const create = async (key: string, name: string, currencyId: string) => {
    const existing = created.get(key)
    if (existing) return existing
    const attempt = attempts.get(key) ?? { requestId: uuidv7(), idempotencyKey: uuidv7() }
    attempts.set(key, attempt)
    if (folder === null && created.size > 0) folder = newAccountFolderId(await getFolderList())
    let id: string
    try {
      // the request id only guards the operation; the server mints the account's own id
      const made = await createAccount(
        { id: attempt.requestId, name, currencyId, balance: '0', icon: defaultAccountIcon, folderId: folder },
        { idempotencyKey: attempt.idempotencyKey },
      )
      folder = made.item.folderId ?? folder
      id = made.item.id
    } catch (error) {
      const replayed = replayOf(error)?.resourceId
      if (!replayed) throw error
      id = replayed
    }
    created.set(key, id)
    return id
  }
  const choice = answers.account
  if (choice?.kind === 'new') {
    return { accountId: await create(NEW_ACCOUNT, choice.name, choice.currencyId) }
  }
  if (choice?.kind === 'column') {
    for (const [name, currencyId] of Object.entries(choice.currencies)) {
      const key = name.toLowerCase()
      if (existingNames.has(key)) continue
      await create(`${PER_ROW}${key}`, names?.get(key) ?? name, currencyId)
    }
  }
  return {}
}
