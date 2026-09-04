// Profile counts are read from the query cache rather than fetched: the numbers
// are already in memory, and sourcing them server-side would mean five
// cross-feature interfaces plus COUNT queries on every get-user-data.

import type { QueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/app/queryKeys'

let client: QueryClient | null = null

export function setAnalyticsQueryClient(qc: QueryClient | null): void {
  client = qc
}

function list<T>(key: readonly unknown[]): T[] | undefined {
  const data = client?.getQueryData(key)
  return Array.isArray(data) ? (data as T[]) : undefined
}

// A count whose query has not loaded is omitted entirely (never sent as 0 —
// that would read as a real measurement rather than "unknown").
function archivedCounts(out: Record<string, number>, key: readonly unknown[], name: string): void {
  const rows = list<{ isArchived: 0 | 1 }>(key)
  if (!rows) {
    return
  }
  out[name] = rows.filter((r) => r.isArchived === 0).length
  out[`${name}_archived`] = rows.filter((r) => r.isArchived === 1).length
}

export function profileAttributes(): Record<string, number> {
  const out: Record<string, number> = {}
  if (!client) {
    return out
  }

  // useAccounts() caches the raw get-account-list response: AccountDto[],
  // flat (folderId directly on the item, not wrapped in AccountItemDto).
  const accounts = list<{ folderId: string | null }>(queryKeys.accounts)
  const folders = list<{ id: string; isVisible: 0 | 1 }>(queryKeys.folders)
  if (accounts) {
    out.accounts = accounts.length
    if (folders) {
      // Accounts have no visibility of their own; a hidden folder hides them,
      // and an unfoldered account (folderId: null) counts as visible.
      const hidden = new Set(folders.filter((f) => f.isVisible === 0).map((f) => f.id))
      out.accounts_hidden = accounts.filter((a) => a.folderId !== null && hidden.has(a.folderId)).length
    }
  }

  archivedCounts(out, queryKeys.categories, 'categories')
  archivedCounts(out, queryKeys.payees, 'payees')
  archivedCounts(out, queryKeys.tags, 'tags')

  const connections = list<unknown>(queryKeys.connections)
  if (connections) {
    out.connections = connections.length
  }

  const user = client.getQueryData<{ createdAt?: string }>(queryKeys.user)
  if (user?.createdAt) {
    // "2006-01-02 15:04:05" UTC; a cohort label, so it is read verbatim
    // rather than parsed as a local Date (which would drift across zones).
    const [y, m] = user.createdAt.slice(0, 7).split('-')
    const year = Number(y)
    const month = Number(m)
    if (Number.isFinite(year) && Number.isFinite(month)) {
      out.signup_year = year
      out.signup_month = month
    }
  }
  return out
}
