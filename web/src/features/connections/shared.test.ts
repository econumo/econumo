import { hasAccountAdminAccess, hasBudgetAdminAccess, isPendingForMe, sharedAccountsFor, sharedBudgetsFor } from './shared'
import { fixtureAccounts } from '@/test/fixtures'
import type { AccountDto } from '@/api/dto/account'
import type { BudgetMetaDto } from '@/api/dto/budget'

const me = { id: 'u1', avatar: '', name: 'Me' }
const partner = { id: 'u2', avatar: '', name: 'Partner' }

const base = fixtureAccounts[0] as unknown as AccountDto
const mine: AccountDto = { ...base, id: 'a1', name: 'Wallet', owner: me, sharedAccess: [{ user: partner, role: 'user', isAccepted: 1 }] }
const theirs: AccountDto = { ...base, id: 'a2', name: 'Their cash', owner: partner, sharedAccess: [{ user: me, role: 'guest', isAccepted: 1 }] }
const unshared: AccountDto = { ...base, id: 'a3', owner: me, sharedAccess: [] }

it('sharedAccountsFor picks both directions with the counterparty role', () => {
  const items = sharedAccountsFor([mine, theirs, unshared], 'u1', 'u2')
  expect(items).toEqual([
    { id: 'a1', name: 'Wallet', icon: mine.icon, role: 'user', ownedByMe: true, owner: me },
    { id: 'a2', name: 'Their cash', icon: theirs.icon, role: 'guest', ownedByMe: false, owner: partner },
  ])
})

it('sharedBudgetsFor mirrors the logic over budget access', () => {
  const myBudget: BudgetMetaDto = {
    id: 'b1', ownerUserId: 'u1', name: 'Household', startedAt: '2026-01-01 00:00:00', currencyId: 'c1',
    access: [
      { user: me, role: 'owner', isAccepted: 1 },
      { user: partner, role: 'user', isAccepted: 0 },
    ],
  }
  const theirBudget: BudgetMetaDto = {
    ...myBudget, id: 'b2', ownerUserId: 'u2',
    access: [
      { user: partner, role: 'owner', isAccepted: 1 },
      { user: me, role: 'guest', isAccepted: 1 },
    ],
  }
  expect(sharedBudgetsFor([myBudget, theirBudget], 'u1', 'u2')).toEqual([
    { id: 'b1', name: 'Household', icon: undefined, role: 'user', ownedByMe: true, owner: me },
    { id: 'b2', name: 'Household', icon: undefined, role: 'guest', ownedByMe: false, owner: partner },
  ])
})

it('isPendingForMe: true only when I have an unaccepted grant on someone else\'s account', () => {
  const pending: AccountDto = { ...theirs, sharedAccess: [{ user: me, role: 'guest', isAccepted: 0 }] }
  expect(isPendingForMe(pending, 'u1')).toBe(true)
  expect(isPendingForMe(theirs, 'u1')).toBe(false)
  expect(isPendingForMe(mine, 'u1')).toBe(false)
  expect(isPendingForMe(pending, undefined)).toBe(false)
})

it('admin access = owner or admin grant (budgets require accepted)', () => {
  expect(hasAccountAdminAccess(mine, 'u1')).toBe(true)
  expect(hasAccountAdminAccess(theirs, 'u1')).toBe(false)
  expect(hasAccountAdminAccess({ ...theirs, sharedAccess: [{ user: me, role: 'admin', isAccepted: 1 }] }, 'u1')).toBe(true)
  const budget: BudgetMetaDto = {
    id: 'b1', ownerUserId: 'u2', name: 'B', startedAt: '', currencyId: 'c1',
    access: [{ user: me, role: 'admin', isAccepted: 1 }],
  }
  expect(hasBudgetAdminAccess(budget, 'u1')).toBe(true)
  expect(hasBudgetAdminAccess({ ...budget, access: [{ user: me, role: 'admin', isAccepted: 0 }] }, 'u1')).toBe(false)
  expect(hasBudgetAdminAccess({ ...budget, ownerUserId: 'u1' }, 'u1')).toBe(true)
})
