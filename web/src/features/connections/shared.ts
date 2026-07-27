import type { AccountDto } from '@/api/dto/account'
import type { BudgetMetaDto } from '@/api/dto/budget'
import type { ConnectionDto } from '@/api/dto/connection'
import type { UserDto } from '@/api/dto/user'
import type { Id } from '@/api/types'

export interface SharedItem {
  id: Id
  name: string
  icon?: string
  role: string
  ownedByMe: boolean
  owner: UserDto
}

// Vue derives a connection's shared items from the accounts/budgets stores, not
// from the connection payload: my item -> the partner's role, their item -> my role.
export function sharedAccountsFor(accounts: AccountDto[], meId: Id, otherId: Id): SharedItem[] {
  const items: SharedItem[] = []
  for (const account of accounts) {
    if (account.owner.id === meId) {
      const entry = account.sharedAccess.find((a) => a.user.id === otherId)
      if (entry) items.push({ id: account.id, name: account.name, icon: account.icon, role: entry.role, ownedByMe: true, owner: account.owner })
    } else if (account.owner.id === otherId) {
      const entry = account.sharedAccess.find((a) => a.user.id === meId)
      if (entry) items.push({ id: account.id, name: account.name, icon: account.icon, role: entry.role, ownedByMe: false, owner: account.owner })
    }
  }
  return items
}

export function sharedBudgetsFor(budgets: BudgetMetaDto[], meId: Id, otherId: Id): SharedItem[] {
  const items: SharedItem[] = []
  for (const budget of budgets) {
    const owner = budget.access.find((a) => a.user.id === budget.ownerUserId)?.user
    if (!owner) continue
    if (budget.ownerUserId === meId) {
      const entry = budget.access.find((a) => a.user.id === otherId)
      if (entry) items.push({ id: budget.id, name: budget.name, icon: undefined, role: entry.role, ownedByMe: true, owner })
    } else if (budget.ownerUserId === otherId) {
      const entry = budget.access.find((a) => a.user.id === meId)
      if (entry) items.push({ id: budget.id, name: budget.name, icon: undefined, role: entry.role, ownedByMe: false, owner })
    }
  }
  return items
}

/** True when the account is shared TO me and I have not accepted yet. */
export function isPendingForMe(account: AccountDto, meId: Id | undefined): boolean {
  if (!meId || account.owner.id === meId) return false
  return account.sharedAccess.some((a) => a.user.id === meId && a.isAccepted === 0)
}

export interface ShareEntry {
  user: UserDto
  role: string | null
  /** undefined = no accept step for this grant (e.g. the owner row) */
  isAccepted?: boolean
}

// Vue seeds every connection user (owner role for the item owner, else none),
// then overlays the item's access grants; the current user never lists themselves.
export function buildShareEntries(
  connections: ConnectionDto[],
  access: { user: UserDto; role: string; isAccepted?: 0 | 1 }[],
  meId: Id,
  ownerUserId: Id,
): ShareEntry[] {
  return connections
    .filter((connection) => connection.user.id !== meId)
    .map((connection) => {
      const entry = access.find((a) => a.user.id === connection.user.id)
      if (entry) {
        return { user: connection.user, role: entry.role, isAccepted: entry.isAccepted === undefined ? undefined : entry.isAccepted === 1 }
      }
      return { user: connection.user, role: connection.user.id === ownerUserId ? 'owner' : null, isAccepted: undefined }
    })
}

export function hasAccountAdminAccess(account: AccountDto, meId: Id): boolean {
  return account.owner.id === meId || account.sharedAccess.some((a) => a.user.id === meId && a.role === 'admin')
}

export function hasBudgetAdminAccess(budget: BudgetMetaDto, meId: Id): boolean {
  if (budget.ownerUserId === meId) return true
  const entry = budget.access.find((a) => a.user.id === meId)
  return entry?.role === 'admin' && entry.isAccepted === 1
}
