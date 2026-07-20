import type { TransactionAccountPageDto, TransactionDto, TransactionPageDto } from '@/api/dto/transaction'

export const PER_ACCOUNT_LIMIT = 50
export const PAGE_LIMIT = 50

export interface TxKey {
  date: string
  id: string
}

// Per-account pagination state. oldestLoaded is the account's loaded horizon:
// rows older than it (loaded via ANOTHER account's window, e.g. a transfer)
// are hidden until scrolling reaches them, so the list never shows a false gap.
export interface AccountPageState {
  nextCursor: string | null
  hasMore: boolean
  oldestLoaded: TxKey | null
}

export type TransactionPagesMap = Record<string, AccountPageState>

// the backend list order: spent_at DESC, id ASC
export function byNewestFirst(a: TxKey, b: TxKey): number {
  if (a.date !== b.date) {
    return a.date < b.date ? 1 : -1
  }
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0
}

export function isOlderThan(tx: TxKey, boundary: TxKey): boolean {
  return tx.date < boundary.date || (tx.date === boundary.date && tx.id > boundary.id)
}

// Extend an account's loaded horizon to include key when key is older than the
// current boundary, so a backdated write stays visible in place. No-op when the
// account has no horizon (null oldestLoaded = nothing hidden) or key is already
// within it.
export function widenHorizon(state: AccountPageState, key: TxKey): AccountPageState {
  if (!state.oldestLoaded || !isOlderThan(key, state.oldestLoaded)) {
    return state
  }
  return { ...state, oldestLoaded: key }
}

export function buildPagesFromBoot(
  items: TransactionDto[],
  accounts: TransactionAccountPageDto[],
  perAccountLimit: number = PER_ACCOUNT_LIMIT,
): TransactionPagesMap {
  const map: TransactionPagesMap = {}
  for (const acc of accounts) {
    let oldestLoaded: TxKey | null = null
    if (acc.hasMore) {
      const touching = items
        .filter((t) => t.accountId === acc.id || t.accountRecipientId === acc.id)
        .sort(byNewestFirst)
      const boundary = touching[Math.min(perAccountLimit, touching.length) - 1]
      oldestLoaded = boundary ? { date: boundary.date, id: boundary.id } : null
    }
    map[acc.id] = { nextCursor: acc.nextCursor ?? null, hasMore: acc.hasMore, oldestLoaded }
  }
  return map
}

export function mergeTransactions(prev: TransactionDto[], fetched: TransactionDto[]): TransactionDto[] {
  const ids = new Set(fetched.map((t) => t.id))
  return [...prev.filter((t) => !ids.has(t.id)), ...fetched]
}

export function advancePage(
  prev: AccountPageState,
  page: TransactionPageDto | undefined,
  fetched: TransactionDto[],
): AccountPageState {
  const last = fetched[fetched.length - 1]
  return {
    nextCursor: page?.nextCursor ?? null,
    hasMore: page?.hasMore ?? false,
    oldestLoaded: last ? { date: last.date, id: last.id } : prev.oldestLoaded,
  }
}
