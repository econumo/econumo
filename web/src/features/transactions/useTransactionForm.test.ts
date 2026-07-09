import { initialFormState, buildPayload, evaluatedAmount, accountOptions, categoryOptions, canChangeAccountData } from './useTransactionForm'
import type { AccountDto } from '@/api/dto/account'
import type { TransactionDto } from '@/api/dto/transaction'

const UUID_V7 = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

const usd = { id: 'usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 }
const owner = { id: 'u1', avatar: '', name: 'Ada' }
const other = { id: 'u2', avatar: '', name: 'Bob' }

const account = (over: Partial<AccountDto>): AccountDto => ({
  id: 'a1', owner, folderId: 'f1', name: 'Cash', position: 0, currency: usd,
  balance: '0', type: 1, icon: 'wallet', sharedAccess: [], ...over,
})

afterEach(() => vi.useRealTimers())

it('creation defaults: v7 id, now date, expense type, route account', () => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date(2026, 6, 3, 14, 30, 45))
  const state = initialFormState({}, [account({})], 'a1')
  expect(state.id).toMatch(UUID_V7)
  expect(state.isNew).toBe(true)
  expect(state.type).toBe('expense')
  expect(state.accountId).toBe('a1')
  expect(state.date).toBe('2026-07-03 14:30:45')
})

it('edit seeds all fields from the transaction, amounts unformatted', () => {
  const tx: TransactionDto = {
    id: 't1', author: owner, type: 'expense', accountId: 'a1', accountRecipientId: null,
    amount: '1234.5', amountRecipient: null, categoryId: 'cat1', description: 'x', payeeId: 'p1', tagId: null,
    date: '2026-07-01 10:00:00',
  }
  const state = initialFormState({ transaction: tx }, [account({})], null)
  expect(state.isNew).toBe(false)
  expect(state.amount).toBe('1234.50')
  expect(state.categoryId).toBe('cat1')
  expect(state.date).toBe('2026-07-01 10:00:00')
})

it('payload nulls the right fields per type and evaluates formulas', () => {
  const base = initialFormState({}, [account({})], 'a1')
  const expense = buildPayload({ ...base, type: 'expense', amount: '5+5', categoryId: 'cat1', payeeId: 'p1', tagId: 'tag1', accountRecipientId: 'a2', amountRecipient: '99' })
  expect(expense.amount).toBe('10')
  expect(expense.accountRecipientId).toBeNull()
  expect(expense.amountRecipient).toBeNull()
  expect(expense.categoryId).toBe('cat1')

  const transfer = buildPayload({ ...base, type: 'transfer', amount: '10', accountRecipientId: 'a2', amountRecipient: '9', categoryId: 'cat1', payeeId: 'p1', tagId: 'tag1' })
  expect(transfer.categoryId).toBeNull()
  expect(transfer.payeeId).toBeNull()
  expect(transfer.tagId).toBeNull()
  expect(transfer.accountRecipientId).toBe('a2')
  expect(transfer.amountRecipient).toBe('9')

  const sameCurrencyTransfer = buildPayload({ ...base, type: 'transfer', amount: '10', accountRecipientId: 'a2', amountRecipient: '' })
  expect(sameCurrencyTransfer.amountRecipient).toBe('10')
})

it('posts large plain amounts verbatim', () => {
  const base = initialFormState({}, [account({})], 'a1')
  const form = { ...base, amount: '12345678901234567.89', type: 'expense' as const }
  expect(buildPayload(form).amount).toBe('12345678901234567.89')
})

it('posts large negative plain amounts verbatim', () => {
  expect(evaluatedAmount('-12345678901234567.89')).toBe('-12345678901234567.89')
})

it('sanitizes comma-decimal recipient amounts before normalizing', () => {
  const base = initialFormState({}, [account({})], 'a1')
  const transfer = buildPayload({ ...base, type: 'transfer', amount: '10', accountRecipientId: 'a2', amountRecipient: '9,99' })
  expect(transfer.amountRecipient).toBe('9.99')
})

it('falls back to the primary amount when the recipient amount is unparseable', () => {
  const base = initialFormState({}, [account({})], 'a1')
  const transfer = buildPayload({ ...base, type: 'transfer', amount: '10', accountRecipientId: 'a2', amountRecipient: 'garbage' })
  expect(transfer.amountRecipient).toBe('10')
})

it('creation offers only accounts in visible folders; edit offers all', () => {
  const accounts = [account({ id: 'a1', folderId: 'f-visible' }), account({ id: 'a2', folderId: 'f-hidden' })]
  const folders = [
    { id: 'f-visible', name: 'V', position: 0, isVisible: 1 as const },
    { id: 'f-hidden', name: 'H', position: 1, isVisible: 0 as const },
  ]
  expect(accountOptions(accounts, folders, true).map((a) => a.id)).toEqual(['a1'])
  expect(accountOptions(accounts, folders, false).map((a) => a.id)).toEqual(['a1', 'a2'])
})

it('categories filter by type, owner and archived flag', () => {
  const categories = [
    { id: 'c1', ownerUserId: 'u1', name: 'Food', position: 0, type: 'expense' as const, icon: '', isArchived: 0 as const, createdAt: '', updatedAt: '' },
    { id: 'c2', ownerUserId: 'u1', name: 'Salary', position: 1, type: 'income' as const, icon: '', isArchived: 0 as const, createdAt: '', updatedAt: '' },
    { id: 'c3', ownerUserId: 'u1', name: 'Old', position: 2, type: 'expense' as const, icon: '', isArchived: 1 as const, createdAt: '', updatedAt: '' },
    { id: 'c4', ownerUserId: 'u2', name: 'Foreign', position: 3, type: 'expense' as const, icon: '', isArchived: 0 as const, createdAt: '', updatedAt: '' },
  ]
  expect(categoryOptions(categories, 'expense', 'u1').map((c) => c.id)).toEqual(['c1'])
})

it('canChangeAccountData: owner or shared admin only', () => {
  expect(canChangeAccountData(account({}), 'u1')).toBe(true)
  expect(canChangeAccountData(account({ owner: other }), 'u1')).toBe(false)
  expect(canChangeAccountData(account({ owner: other, sharedAccess: [{ user: owner, role: 'admin' }] }), 'u1')).toBe(true)
  expect(canChangeAccountData(account({ owner: other, sharedAccess: [{ user: owner, role: 'user' }] }), 'u1')).toBe(false)
})
