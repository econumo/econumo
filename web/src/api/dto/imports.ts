import type { Id } from '../types'
import type { AccountDto } from './account'
import type { CreateTransactionDto, TransactionDto, TransactionType } from './transaction'

export type ImportProvider = 'apple-wallet' | 'simplefin'
export type ImportCardState = 'mapped' | 'ignored' | 'unmapped'
export type ImportQueueReason = 'unmapped' | 'account_deleted' | 'no_rate'
export type IngestStatus = 'created' | 'queued' | 'skipped' | 'duplicate' | 'failed'
export type ImportRunStatus = 'running' | 'completed' | 'partial' | 'failed'

export interface ImportCardDto {
  externalAccountId: string
  externalName: string
  externalCurrency: string
  state: ImportCardState
  /** '' unless state === 'mapped' */
  accountId: Id | ''
  queuedCount: number
  tapCount: number
  /** "YYYY-MM-DD HH:mm:ss" of the newest event from this card, '' when none */
  lastSeenAt: string
}

export interface ImportSourceDto {
  id: Id
  provider: ImportProvider
  name: string
  status: string
  createdAt: string
  /** "YYYY-MM-DD HH:mm:ss" of the last non-failed sync, '' before the first */
  lastSyncedAt: string
  /** opaque to the server; '' for push providers */
  credentialCiphertext: string
  cards: ImportCardDto[]
}

export interface ImportRunErrorDto {
  /** '' for a run-level (bridge) error */
  externalAccountId: string
  message: string
}

export interface ImportRunDto {
  id: Id
  sourceId: Id
  provider: ImportProvider
  status: ImportRunStatus
  trigger: string
  importedCount: number
  matchedCount: number
  amountsUpdatedCount: number
  queuedCount: number
  skippedCount: number
  failedCount: number
  errors: ImportRunErrorDto[]
  startedAt: string
  /** '' while running */
  finishedAt: string
}

export interface ImportRunLinkDto {
  id: Id
  externalAccountId: string
  externalTransactionId: string
  /** '' for queued/skipped rows and for tombstones (transaction deleted since) */
  transactionId: Id | ''
  status: string
  externalPayee: string
  externalAmount: string
  externalCurrency: string
  externalPostedAt: string
}

export interface ImportRunDetailDto {
  item: ImportRunDto
  links: ImportRunLinkDto[]
}

export interface ExternalAccountDto {
  externalAccountId: string
  externalName: string
  externalCurrency: string
  balance: string
  orgName: string
  state: ImportCardState
  accountId: Id | ''
}

export interface ImportCredentialKeyDto {
  wrappedDataKey: string
  kdf: string
  updatedAt: string
}

export interface SyncImportSourceResultDto {
  run: ImportRunDto
  accounts: ExternalAccountDto[]
}

/** link-account / ignore-account / unlink-account: the refreshed source, plus
    the conversion run when link-account drained a queue (null otherwise). */
export interface UpdateImportAccountDto {
  item: ImportSourceDto
  run: ImportRunDto | null
}

export interface IngestEventDto {
  status: IngestStatus
  /** '' for a duplicate (nothing new was stored) */
  eventId: Id | ''
}

export interface ImportQueuedEventDto {
  linkId: Id
  sourceId: Id
  externalAccountId: string
  /** '' while the card is unmapped */
  accountId: Id | ''
  payee: string
  amount: string
  currency: string
  type: TransactionType
  /** "YYYY-MM-DD HH:mm:ss" */
  postedAt: string
  reason: ImportQueueReason
}

export interface ImportFailedEventDto {
  eventId: Id
  sourceId: Id
  receivedAt: string
  error: string
  /** the raw body as received, for the "needs attention" panel */
  payload: string
}

export interface ImportQueueDto {
  queued: ImportQueuedEventDto[]
  skipped: ImportQueuedEventDto[]
  failed: ImportFailedEventDto[]
}

export interface ImportQueuedEventPayload {
  linkId: Id
  transaction: CreateTransactionDto
}

export interface ImportQueuedEventResultDto {
  item: TransactionDto
  accounts: AccountDto[]
}

export interface TransactionImportLinkDto {
  id: Id
  sourceId: Id
  provider: ImportProvider
  sourceName: string
  externalAccountId: string
  externalTransactionId: string
  externalPayee: string
  externalAmount: string
  externalCurrency: string
  externalPostedAt: string
  status: string
  importedAt: string
}
