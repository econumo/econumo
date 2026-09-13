import { api, apiUrl } from './client'
import type { Id } from './types'
import type {
  ApplyImportRuleDto,
  ExternalAccountDto,
  ImportCredentialKeyDto,
  ImportProvider,
  ImportQueueDto,
  ImportQueuedEventPayload,
  ImportQueuedEventResultDto,
  ImportRuleDto,
  ImportRuleScopeDto,
  ImportRuleSpecDto,
  ImportRuleSuggestionDto,
  ImportRunDetailDto,
  ImportRunDto,
  ImportSourceDto,
  IngestEventDto,
  PreviewImportRuleDto,
  SyncImportSourceResultDto,
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

export async function createImportSource(provider: ImportProvider, name: string, credentialCiphertext?: string): Promise<ImportSourceDto> {
  const body: Record<string, string> = { provider, name }
  if (credentialCiphertext) {
    body.credentialCiphertext = credentialCiphertext
  }
  const response = await api.post<Envelope<{ item: ImportSourceDto }>>(apiUrl('/api/v1/import/create-source'), body)
  return response.data.data.item
}

export async function claimSetupToken(setupToken: string): Promise<string> {
  const response = await api.post<Envelope<{ accessUrl: string }>>(apiUrl('/api/v1/import/claim-setup-token'), { setupToken })
  return response.data.data.accessUrl
}

// No key yet comes back as 200 with an empty wrappedDataKey — the ordinary
// "connect for the first time" state; every failure is a real error.
export async function getImportCredentialKey(): Promise<ImportCredentialKeyDto | null> {
  const response = await api.get<Envelope<ImportCredentialKeyDto>>(apiUrl('/api/v1/import/get-credential-key'))
  return response.data.data.wrappedDataKey ? response.data.data : null
}

export async function setImportCredentialKey(key: { wrappedDataKey: string; kdf: string }): Promise<ImportCredentialKeyDto> {
  const response = await api.post<Envelope<ImportCredentialKeyDto>>(apiUrl('/api/v1/import/set-credential-key'), key)
  return response.data.data
}

export async function listExternalAccounts(sourceId: Id, accessUrl: string): Promise<ExternalAccountDto[]> {
  const response = await api.post<Envelope<{ items: ExternalAccountDto[] }>>(apiUrl('/api/v1/import/list-external-accounts'), { sourceId, accessUrl })
  return response.data.data.items
}

export async function syncImportSource(sourceId: Id, accessUrl: string, startDate: string, endDate?: string): Promise<SyncImportSourceResultDto> {
  const body: Record<string, string> = { sourceId, accessUrl, startDate }
  if (endDate) {
    body.endDate = endDate
  }
  const response = await api.post<Envelope<SyncImportSourceResultDto>>(apiUrl('/api/v1/import/sync-source'), body)
  return response.data.data
}

export async function getImportRunList(sourceId?: Id): Promise<ImportRunDto[]> {
  const response = await api.get<Envelope<{ items: ImportRunDto[] }>>(
    apiUrl('/api/v1/import/get-run-list'),
    { params: sourceId ? { sourceId } : {} },
  )
  return response.data.data.items
}

export async function getImportRun(id: Id): Promise<ImportRunDetailDto> {
  const response = await api.get<Envelope<ImportRunDetailDto>>(apiUrl('/api/v1/import/get-run'), { params: { id } })
  return response.data.data
}

export async function deleteImportSource(id: Id): Promise<ImportSourceDto[]> {
  const response = await api.post<Envelope<{ items: ImportSourceDto[] }>>(apiUrl('/api/v1/import/delete-source'), { id })
  return response.data.data.items
}

export async function linkImportAccount(sourceId: Id, externalAccountId: string, accountId: Id, externalName?: string): Promise<UpdateImportAccountDto> {
  const body: Record<string, string> = { sourceId, externalAccountId, accountId }
  if (externalName) {
    body.externalName = externalName
  }
  const response = await api.post<Envelope<UpdateImportAccountDto>>(apiUrl('/api/v1/import/link-account'), body)
  return response.data.data
}

export async function ignoreImportAccount(sourceId: Id, externalAccountId: string, externalName?: string): Promise<UpdateImportAccountDto> {
  const body: Record<string, string> = { sourceId, externalAccountId }
  if (externalName) {
    body.externalName = externalName
  }
  const response = await api.post<Envelope<UpdateImportAccountDto>>(apiUrl('/api/v1/import/ignore-account'), body)
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

export async function getImportRuleList(): Promise<ImportRuleDto[]> {
  const response = await api.get<Envelope<{ items: ImportRuleDto[] }>>(apiUrl('/api/v1/import/get-rule-list'))
  return response.data.data.items
}

// The id is REQUIRED and client-minted (it is the create's idempotency key):
// the server rejects a blank one. Typed as part of the body — an untyped
// Record let a missing required field type-check.
export async function createImportRule(spec: ImportRuleSpecDto, id: Id): Promise<ImportRuleDto> {
  const body: ImportRuleSpecDto & { id: Id } = { ...spec, id }
  const response = await api.post<Envelope<ImportRuleDto>>(apiUrl('/api/v1/import/create-rule'), body)
  return response.data.data
}

export async function updateImportRule(id: Id, spec: ImportRuleSpecDto): Promise<ImportRuleDto> {
  const response = await api.post<Envelope<ImportRuleDto>>(apiUrl('/api/v1/import/update-rule'), { id, ...spec })
  return response.data.data
}

export async function deleteImportRule(id: Id): Promise<void> {
  await api.post(apiUrl('/api/v1/import/delete-rule'), { id })
}

export async function previewImportRule(spec: ImportRuleSpecDto, scope: ImportRuleScopeDto): Promise<PreviewImportRuleDto> {
  const response = await api.post<Envelope<PreviewImportRuleDto>>(apiUrl('/api/v1/import/preview-rule'), { ...spec, ...scope })
  return response.data.data
}

export async function applyImportRule(ruleId: Id, scope: ImportRuleScopeDto, includeEdited: boolean): Promise<ApplyImportRuleDto> {
  const response = await api.post<Envelope<ApplyImportRuleDto>>(apiUrl('/api/v1/import/apply-rule'), { ruleId, ...scope, includeEdited })
  return response.data.data
}

export async function suggestImportRules(scope: ImportRuleScopeDto): Promise<ImportRuleSuggestionDto[]> {
  const response = await api.post<Envelope<{ items: ImportRuleSuggestionDto[] }>>(apiUrl('/api/v1/import/suggest-rules'), scope)
  return response.data.data.items
}
