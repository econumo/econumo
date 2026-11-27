import type { CategoryType } from '@/api/dto/category'
import { useAccounts } from '@/features/accounts/queries'
import { useUserData } from '@/features/user/queries'
import { isNegative } from '@/lib/decimal'
import type { AccountChoice, PreparedRow } from './prepare'
import { nameKey } from './wizard'

/** Whose categories, payees and tags the import fills: the target account's owner. A member
    with a user grant imports into the owner's lists but cannot grow them: the server leaves a
    name the owner does not have yet out of the row. */
export function useOwnerLists(account: AccountChoice | null): { ownerId: string | undefined; readOnlyLists: boolean } {
  const { data: user } = useUserData()
  const { data: accounts = [] } = useAccounts()
  const target = account?.kind === 'existing' ? accounts.find((a) => a.id === account.accountId) : undefined
  const readOnlyLists =
    target !== undefined && target.owner.id !== user?.id && target.sharedAccess.find((a) => a.user.id === user?.id)?.role !== 'admin'
  return { ownerId: target?.owner.id ?? user?.id, readOnlyLists }
}

// A category is found within its kind, as the server does: an income row (an amount that is
// not negative) naming the expense category "Food" gets an income "Food" of its own.
export const categoryKey = (type: CategoryType, name: string) => `${type}:${nameKey(name)}`
const rowKind = (row: PreparedRow): CategoryType => (row.amount !== null && isNegative(row.amount) ? 'expense' : 'income')

// Only rows that will upload: a name found only on a skipped row is never created.
export function distinctNew(rows: PreparedRow[], role: 'category' | 'labels', split: string | null, existing: Set<string>): number {
  const found = new Set<string>()
  for (const row of rows) {
    if (row.status !== 'ok') continue
    const cell = row[role]
    for (const part of split === null ? [cell] : cell.split(split)) {
      if (nameKey(part) === '') continue
      const key = role === 'category' ? categoryKey(rowKind(row), part) : nameKey(part)
      if (!existing.has(key)) found.add(key)
    }
  }
  return found.size
}
