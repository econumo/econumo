import type { AccountDto } from '@/api/dto/account'
import { BudgetElementType, UNCATEGORIZED_ID } from '@/api/dto/budget'
import { fixtureAccounts } from '@/test/fixtures'
import { elementEditAccess } from './elementEdit'

const accounts = fixtureAccounts as unknown as AccountDto[]
const owner = fixtureAccounts[0].owner.id

it('has nothing to edit on the Uncategorized row', () => {
  expect(elementEditAccess({ id: UNCATEGORIZED_ID, type: BudgetElementType.CATEGORY, ownerUserId: null }, owner, true, accounts)).toBeNull()
})

it('lets an envelope be edited by the budget role alone', () => {
  for (const type of [BudgetElementType.ENVELOPE, BudgetElementType.INCOME_ENVELOPE]) {
    expect(elementEditAccess({ id: 'e', type, ownerUserId: null }, 'someone', true, [])).toBe(true)
    expect(elementEditAccess({ id: 'e', type, ownerUserId: null }, owner, false, [])).toBe(false)
  }
})

it('lets only the owner edit a category or tag, whatever the budget role', () => {
  for (const type of [BudgetElementType.CATEGORY, BudgetElementType.INCOME_CATEGORY, BudgetElementType.TAG]) {
    expect(elementEditAccess({ id: 'c', type, ownerUserId: 'u1' }, 'u1', false, [])).toBe(true)
    expect(elementEditAccess({ id: 'c', type, ownerUserId: 'u9' }, 'u1', true, [])).toBe(false)
    expect(elementEditAccess({ id: 'c', type, ownerUserId: 'u1' }, undefined, true, [])).toBe(false)
  }
})

it('lets a savings account be edited by its owner or an admin, and not once it is gone', () => {
  const account = accounts[0]
  const savings = { id: account.id, type: BudgetElementType.SAVINGS, ownerUserId: owner }
  expect(elementEditAccess(savings, owner, false, accounts)).toBe(true)
  const shared = (role: string) => [{ ...account, sharedAccess: [{ user: { id: 'u7', name: 'Sam', avatar: 'face:sky' }, role }] }] as unknown as AccountDto[]
  expect(elementEditAccess(savings, 'u7', true, shared('admin'))).toBe(true)
  expect(elementEditAccess(savings, 'u7', true, shared('user'))).toBe(false)
  expect(elementEditAccess(savings, owner, true, [])).toBe(false)
})
