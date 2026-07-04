import type { AccountDto, AccountRole } from '@/api/dto/account'
import type { BudgetMetaDto } from '@/api/dto/budget'
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

export function applyAccountAccess(accounts: AccountDto[], accountId: Id, user: UserDto, role: AccountRole): AccountDto[] {
  return accounts.map((account) => {
    if (account.id !== accountId) return account
    const rest = account.sharedAccess.filter((a) => a.user.id !== user.id)
    return { ...account, sharedAccess: [...rest, { user, role }] }
  })
}

export function removeAccountAccess(accounts: AccountDto[], accountId: Id, userId: Id): AccountDto[] {
  return accounts.map((account) =>
    account.id === accountId ? { ...account, sharedAccess: account.sharedAccess.filter((a) => a.user.id !== userId) } : account,
  )
}

export function hasAccountAdminAccess(account: AccountDto, meId: Id): boolean {
  return account.owner.id === meId || account.sharedAccess.some((a) => a.user.id === meId && a.role === 'admin')
}

export function hasBudgetAdminAccess(budget: BudgetMetaDto, meId: Id): boolean {
  if (budget.ownerUserId === meId) return true
  const entry = budget.access.find((a) => a.user.id === meId)
  return entry?.role === 'admin' && entry.isAccepted === 1
}
