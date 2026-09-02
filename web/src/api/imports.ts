import { api, apiUrl } from './client'
import type { Id } from './types'
import type {
  ImportQueueDto,
  ImportQueuedEventPayload,
  ImportQueuedEventResultDto,
  ImportSourceDto,
  IngestEventDto,
  TransactionImportLinkDto,
  UpdateImportAccountDto,
} from './dto/imports'

interface Envelope<T> {
  data: T
}

export async function getImportSourceList(): Promise<ImportSourceDto[]> {
  const response = await api.get<Envelope<{ items: ImportSourceDto[] }>>(apiUrl('/api/v1/import/get-source-list'))
  return response.data.data.items
}

export async function createImportSource(provider: 'apple-wallet', name: string): Promise<ImportSourceDto> {
  const response = await api.post<Envelope<{ item: ImportSourceDto }>>(apiUrl('/api/v1/import/create-source'), { provider, name })
  return response.data.data.item
}

export async function deleteImportSource(id: Id): Promise<ImportSourceDto[]> {
  const response = await api.post<Envelope<{ items: ImportSourceDto[] }>>(apiUrl('/api/v1/import/delete-source'), { id })
  return response.data.data.items
}

export async function linkImportAccount(sourceId: Id, externalAccountId: string, accountId: Id): Promise<UpdateImportAccountDto> {
  const response = await api.post<Envelope<UpdateImportAccountDto>>(apiUrl('/api/v1/import/link-account'), { sourceId, externalAccountId, accountId })
  return response.data.data
}

export async function ignoreImportAccount(sourceId: Id, externalAccountId: string): Promise<UpdateImportAccountDto> {
  const response = await api.post<Envelope<UpdateImportAccountDto>>(apiUrl('/api/v1/import/ignore-account'), { sourceId, externalAccountId })
  return response.data.data
}

export async function unlinkImportAccount(sourceId: Id, externalAccountId: string): Promise<UpdateImportAccountDto> {
  const response = await api.post<Envelope<UpdateImportAccountDto>>(apiUrl('/api/v1/import/unlink-account'), { sourceId, externalAccountId })
  return response.data.data
}

export async function getImportQueue(): Promise<ImportQueueDto> {
  const response = await api.get<Envelope<ImportQueueDto>>(apiUrl('/api/v1/import/get-queued-event-list'))
  return response.data.data
}

export async function importQueuedEvent(payload: ImportQueuedEventPayload): Promise<ImportQueuedEventResultDto> {
  const response = await api.post<Envelope<ImportQueuedEventResultDto>>(apiUrl('/api/v1/import/import-queued-event'), payload)
  return response.data.data
}

export async function skipQueuedEvent(linkId: Id): Promise<ImportQueueDto> {
  const response = await api.post<Envelope<ImportQueueDto>>(apiUrl('/api/v1/import/skip-queued-event'), { linkId })
  return response.data.data
}

export async function unskipQueuedEvent(linkId: Id): Promise<ImportQueueDto> {
  const response = await api.post<Envelope<ImportQueueDto>>(apiUrl('/api/v1/import/unskip-queued-event'), { linkId })
  return response.data.data
}

export async function retryImportEvent(eventId: Id): Promise<IngestEventDto> {
  const response = await api.post<Envelope<IngestEventDto>>(apiUrl('/api/v1/import/retry-event'), { eventId })
  return response.data.data
}

export async function discardImportEvent(eventId: Id): Promise<ImportQueueDto> {
  const response = await api.post<Envelope<ImportQueueDto>>(apiUrl('/api/v1/import/discard-event'), { eventId })
  return response.data.data
}

export async function getTransactionImportList(transactionId: Id): Promise<TransactionImportLinkDto[]> {
  const response = await api.get<Envelope<{ items: TransactionImportLinkDto[] }>>(
    apiUrl('/api/v1/import/get-transaction-import-list'),
    { params: { transactionId } },
  )
  return response.data.data.items
}
