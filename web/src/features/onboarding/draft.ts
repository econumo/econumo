import type { Id } from '@/api/types'

export type AccountRow = { key: string; name: string; balance: string; currencyId: Id; savings: boolean; icon: string }
export type CategoryRow = { key: string; name: string; type: 'expense' | 'income'; icon: string; checked: boolean }
export type Day0Draft = {
  step: 'accounts' | 'categories'
  ops: Record<string, Id>
  accountIds: Id[]
  savingsAccountIds: Id[]
  mainCurrencyId: Id | null
}

const storageKey = (userId: Id) => `econumo-day0:${userId}`

export function loadDraft(userId: Id): Day0Draft | null {
  try {
    const raw = localStorage.getItem(storageKey(userId))
    if (!raw) {
      return null
    }
    const parsed = JSON.parse(raw) as Partial<Day0Draft>
    if (parsed.step !== 'accounts' && parsed.step !== 'categories') {
      return null
    }
    return {
      step: parsed.step,
      ops: parsed.ops ?? {},
      accountIds: parsed.accountIds ?? [],
      savingsAccountIds: parsed.savingsAccountIds ?? [],
      mainCurrencyId: parsed.mainCurrencyId ?? null,
    }
  } catch {
    return null
  }
}

export function saveDraft(userId: Id, draft: Day0Draft): void {
  try {
    localStorage.setItem(storageKey(userId), JSON.stringify(draft))
  } catch {
    // storage unavailable: retries then mint fresh ids, which the server may duplicate
  }
}

export function clearDraft(userId: Id): void {
  try {
    localStorage.removeItem(storageKey(userId))
  } catch {
    // nothing stored to clear
  }
}
