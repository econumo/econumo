import type { Id } from '@/api/types'
import type { CreateTransactionDto, TransactionDto } from '@/api/dto/transaction'

type OpBase = {
  key: string
  seq: number
  /** A request may have reached the server: the key can no longer carry a different payload. */
  attempted: boolean
  status: 'waiting' | 'failed'
  error?: string
  retries: number
  notBefore: number
}

export type OpChange =
  | { kind: 'tx-create'; txId: Id; payload: CreateTransactionDto }
  | { kind: 'tx-update'; txId: Id; payload: CreateTransactionDto }
  | { kind: 'tx-delete'; txId: Id; previous: TransactionDto | null }
  | { kind: 'limit'; budgetId: Id; elementId: Id; period: string; amount: string | null }

export type PendingOp = OpBase & OpChange
