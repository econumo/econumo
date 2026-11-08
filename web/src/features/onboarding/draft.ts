import type { Id } from '@/api/types'

export type AccountRow = { key: string; name: string; balance: string; currencyId: Id; savings: boolean; icon: string }
export type CategoryRow = { key: string; name: string; type: 'expense' | 'income'; icon: string; checked: boolean }
export type Day0Draft = {
  step: 'accounts' | 'categories'
  // operation ids by `account:<rowKey>` / `category:<rowKey>` / `budget` (the budget's own id)
  ops: Record<string, Id>
  // the rows last submitted at step 'accounts'; a reload seeds the form from them to keep their keys
  rows: AccountRow[]
  extraCurrencyIds: Id[]
  // server account id by row key / server category id by row key, written right after each create
  created: Record<string, Id>
  createdCategories: Record<string, Id>
  // the categories last submitted by finish, so a retry after a failure reseeds the form with the same keys
  categoryRows: CategoryRow[]
  currenciesDone: boolean
  budgetDone: boolean
  accountIds: Id[]
  savingsAccountIds: Id[]
  mainCurrencyId: Id | null
}

export function emptyDraft(): Day0Draft {
  return {
    step: 'accounts',
    ops: {},
    rows: [],
    extraCurrencyIds: [],
    created: {},
    createdCategories: {},
    categoryRows: [],
    currenciesDone: false,
    budgetDone: false,
    accountIds: [],
    savingsAccountIds: [],
    mainCurrencyId: null,
  }
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
    return { ...emptyDraft(), ...parsed, step: parsed.step }
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
