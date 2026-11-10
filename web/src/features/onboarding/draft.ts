import type { Id } from '@/api/types'

// accountId: the row stands for an account the user already has, so it is never created
// labelKey: a default the user has not renamed; its name follows the UI language
// iconPicked: the icon was chosen by hand, so renaming no longer re-guesses it
export type AccountRow = {
  key: string
  name: string
  balance: string
  currencyId: Id
  icon: string
  accountId?: Id
  labelKey?: string
  iconPicked?: boolean
}
export type CategoryRow = { key: string; name: string; type: 'expense' | 'income'; icon: string; labelKey?: string; defaultKey?: string }
export type Day0Step = 'categories' | 'accounts' | 'plan'
export type Day0Draft = {
  // the operation id of a step's submit, minted on its first Continue and kept until the
  // server answers, so a retry after a lost response replays the same id
  ops: Partial<Record<Day0Step, Id>>
  // the rows last submitted by saveAccounts; a reload seeds the form from them to keep their keys
  rows: AccountRow[]
  extraCurrencyIds: Id[]
  mainCurrencyId: Id | null
  // server account id by row key / server category id by row key, from the step's answer
  created: Record<string, Id>
  createdCategories: Record<string, Id>
  // the categories last submitted by saveCategories, so a retry after a failure reseeds the form with the same keys
  categoryRows: CategoryRow[]
  // set once the user reorders categories: existing category ids and row keys, both groups in display order
  categoryOrder: string[]
  // catalogue keys of the defaults the first submit offered, kept across retries for the analytics event
  offeredDefaults?: string[]
  // the budget the categories or accounts step answered with; the plan step plans it
  budgetId: Id | null
}

export function emptyDraft(): Day0Draft {
  return {
    ops: {},
    rows: [],
    extraCurrencyIds: [],
    mainCurrencyId: null,
    created: {},
    createdCategories: {},
    categoryRows: [],
    categoryOrder: [],
    budgetId: null,
  }
}

const STEPS: Day0Step[] = ['categories', 'accounts', 'plan']

const storageKey = (userId: Id) => `econumo-day0:${userId}`

export function loadDraft(userId: Id): Day0Draft | null {
  try {
    const raw = localStorage.getItem(storageKey(userId))
    if (!raw) {
      return null
    }
    const parsed = JSON.parse(raw) as Partial<Day0Draft> | null
    if (!parsed || typeof parsed !== 'object' || typeof parsed.ops !== 'object') {
      return null
    }
    // a draft from the per-row flow keyed its ids by row; those are never replayed
    const ops: Day0Draft['ops'] = {}
    for (const step of STEPS) {
      const id = (parsed.ops as Record<string, unknown>)[step]
      if (typeof id === 'string') ops[step] = id
    }
    return { ...emptyDraft(), ...parsed, ops }
  } catch {
    return null
  }
}

export function saveDraft(userId: Id, draft: Day0Draft): void {
  try {
    localStorage.setItem(storageKey(userId), JSON.stringify(draft))
  } catch {
    // storage unavailable: a retry after a reload mints a fresh id; the server still never runs a done step twice
  }
}

export function clearDraft(userId: Id): void {
  try {
    localStorage.removeItem(storageKey(userId))
  } catch {
    // nothing stored to clear
  }
}
