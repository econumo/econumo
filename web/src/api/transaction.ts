import { api, apiUrl } from './client'
import type { Id } from './types'
import type { CreateTransactionDto, TransactionDto, TransactionItemDto } from './dto/transaction'
import { coerceAccount, coerceTransaction } from './account'

interface Envelope<T> {
  data: T
}

function coerceItem(raw: TransactionItemDto): TransactionItemDto {
  return {
    item: coerceTransaction(raw.item),
    accounts: raw.accounts.map(coerceAccount),
  }
}

export async function getTransactionList(): Promise<TransactionDto[]> {
  const response = await api.get<Envelope<{ items: TransactionDto[] }>>(apiUrl('/api/v1/transaction/get-transaction-list'))
  return response.data.data.items.map(coerceTransaction)
}

export async function createTransaction(form: CreateTransactionDto): Promise<TransactionItemDto> {
  const response = await api.post<Envelope<TransactionItemDto>>(apiUrl('/api/v1/transaction/create-transaction'), form)
  return coerceItem(response.data.data)
}

export async function updateTransaction(form: CreateTransactionDto): Promise<TransactionItemDto> {
  const response = await api.post<Envelope<TransactionItemDto>>(apiUrl('/api/v1/transaction/update-transaction'), form)
  return coerceItem(response.data.data)
}

export async function deleteTransaction(id: Id): Promise<TransactionItemDto> {
  const response = await api.post<Envelope<TransactionItemDto>>(apiUrl('/api/v1/transaction/delete-transaction'), { id })
  return coerceItem(response.data.data)
}

export interface ImportResultDto {
  imported: number
  skipped: number
  /** message -> 1-based file row numbers (0 = top-level error) */
  errors: Record<string, number[]>
}

// multipart: file + mapping JSON (column names) + optional fixed-value fields
export async function importTransactionList(
  file: File,
  mapping: Record<string, string | null>,
  fields: Record<string, string>,
): Promise<ImportResultDto> {
  const form = new FormData()
  form.append('file', file)
  form.append('mapping', JSON.stringify(mapping))
  for (const [key, value] of Object.entries(fields)) {
    form.append(key, value)
  }
  // no explicit Content-Type: the browser adds multipart/form-data WITH the boundary
  const response = await api.post<Envelope<ImportResultDto>>(apiUrl('/api/v1/transaction/import-transaction-list'), form)
  return response.data.data
}

// raw CSV, not the JSON envelope; a single comma-joined accountId param
export async function exportTransactionList(accountIds: Id[]): Promise<Blob> {
  const response = await api.get<Blob>(apiUrl('/api/v1/transaction/export-transaction-list'), {
    params: { accountId: accountIds.join(',') },
    responseType: 'blob',
  })
  return response.data
}
