// Profile counts are read from the query cache rather than fetched: the numbers
// are already in memory, and sourcing them server-side would mean five
// cross-feature interfaces plus COUNT queries on every get-user-data.

import type { QueryClient } from '@tanstack/react-query'
import { queryKeys } from '@/app/queryKeys'
import type { AccountDto } from '@/api/dto/account'
import type { BudgetMetaDto } from '@/api/dto/budget'
import { isPendingForMe } from '@/features/connections/shared'

let client: QueryClient | null = null

export function setAnalyticsQueryClient(qc: QueryClient | null): void {
  client = qc
}

function list<T>(key: readonly unknown[]): T[] | undefined {
  const data = client?.getQueryData(key)
  return Array.isArray(data) ? (data as T[]) : undefined
}

// A count whose query has not loaded is omitted entirely (never sent as 0 —
// that would read as a real measurement rather than "unknown"). Every count is
// a total: active, hidden and archived items alike.
function count(out: Record<string, number>, key: readonly unknown[], name: string): void {
  const rows = list<unknown>(key)
  if (rows) {
    out[name] = rows.length
  }
}

// Whole calendar months since signup, the day and time of day included, so
// the value only ticks over on the monthly anniversary. createdAt is UTC wall
// clock ("2006-01-02 15:04:05"), parsed as such so no local zone shifts the
// boundary.
function monthsSince(createdAt: string, now: Date): number | undefined {
  const signup = new Date(`${createdAt.replace(' ', 'T')}Z`)
  if (Number.isNaN(signup.getTime())) {
    return undefined
  }
  let months = (now.getUTCFullYear() - signup.getUTCFullYear()) * 12 + (now.getUTCMonth() - signup.getUTCMonth())
  const intoMonth = (d: Date) => d.getTime() - Date.UTC(d.getUTCFullYear(), d.getUTCMonth())
  if (intoMonth(now) < intoMonth(signup)) {
    months -= 1
  }
  return Math.max(0, months)
}

export function profileAttributes(): Record<string, number> {
  const out: Record<string, number> = {}
  if (!client) {
    return out
  }

  const user = client.getQueryData<{ id?: string; createdAt?: string }>(queryKeys.user)

  // useAccounts() caches the raw get-account-list response: AccountDto[],
  // flat (folderId directly on the item, not wrapped in AccountItemDto). The
  // raw list also includes pending-for-me share invites the recipient has not
  // accepted yet (an inert placeholder row) — useAccounts() strips those via
  // its `select`, but getQueryData bypasses select, so this must filter them
  // out too, or a pile of unaccepted invites reads as "accounts I own."
  // Filtering needs "me," so if the user query hasn't loaded, the count is
  // not-yet-known rather than risking an unfiltered (skewed) number.
  const accounts = list<AccountDto>(queryKeys.accounts)
  if (accounts && user?.id) {
    out.accounts = accounts.filter((a) => !isPendingForMe(a, user.id)).length
  }

  // Same reasoning as accounts: the raw list carries budget invites the user
  // has not accepted, which useBudgets() filters out in its `select`.
  const budgets = list<BudgetMetaDto>(queryKeys.budgets)
  if (budgets && user?.id) {
    const me = user.id
    out.budgets = budgets.filter(
      (b) => b.ownerUserId === me || b.access.some((a) => a.user.id === me && a.isAccepted === 1),
    ).length
  }

  count(out, queryKeys.categories, 'categories')
  count(out, queryKeys.payees, 'payees')
  count(out, queryKeys.tags, 'tags')
  count(out, queryKeys.labels, 'labels')
  count(out, queryKeys.connections, 'connections')

  if (user?.createdAt) {
    // A cohort label, so it is read verbatim rather than parsed as a local
    // Date (which would drift across zones).
    const year = Number(user.createdAt.slice(0, 4))
    if (Number.isFinite(year)) {
      out.signup_year = year
    }
    const months = monthsSince(user.createdAt, new Date())
    if (months !== undefined) {
      out.months_since_signup = months
    }
  }
  return out
}
