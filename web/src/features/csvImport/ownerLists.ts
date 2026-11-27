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

export interface NameInFile {
  key: string
  /** the first spelling in the file */
  name: string
  /** transactions that name it */
  count: number
  isNew: boolean
  /** a category named by both kinds of row that exists for only one of them: the kind it is new for */
  newKind: CategoryType | null
}

// Only rows that will upload: a name found only on a skipped row is never created. Most used
// first, ties in file order.
export function namesInFile(rows: PreparedRow[], role: 'category' | 'payee' | 'tag' | 'labels', split: string | null, existing: Set<string>): NameInFile[] {
  const found = new Map<string, { name: string; count: number; kinds: Set<CategoryType> }>()
  for (const row of rows) {
    if (row.status !== 'ok') continue
    const cell = row[role]
    const seen = new Set<string>()
    for (const part of split === null ? [cell] : cell.split(split)) {
      const key = nameKey(part)
      if (key === '') continue
      const entry = found.get(key) ?? { name: part.trim(), count: 0, kinds: new Set<CategoryType>() }
      entry.kinds.add(rowKind(row))
      if (!seen.has(key)) entry.count++
      seen.add(key)
      found.set(key, entry)
    }
  }
  return [...found.entries()]
    .map(([key, { name, count, kinds }]) => {
      if (role !== 'category') return { key, name, count, isNew: !existing.has(key), newKind: null }
      const newKinds = [...kinds].filter((kind) => !existing.has(categoryKey(kind, name)))
      return { key, name, count, isNew: newKinds.length > 0, newKind: newKinds.length === 1 && kinds.size === 2 ? newKinds[0] : null }
    })
    .sort((a, b) => b.count - a.count)
}
