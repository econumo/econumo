import { expect, it } from 'vitest'
import type { AccountDto } from '@/api/dto/account'
import type { BudgetUpcomingDto } from '@/api/dto/budget'
import type { CurrencyDto } from '@/api/dto/currency'
import type { RecurringDto } from '@/api/dto/recurring'
import { recurringAsTransaction, upcomingAsTransaction } from './asTransaction'

const template = {
  id: 'rt1',
  type: 'expense',
  accountId: 'a1',
  accountRecipientId: null,
  amount: '12.00',
  categoryId: null,
  payeeId: null,
  tagId: 'tg1',
  labelIds: ['lb2', 'lb-gone'],
  description: 'gym',
  nextPaymentAt: '2026-08-01 09:00:00',
} as unknown as RecurringDto

const lookups = {
  tags: [{ id: 'tg1', name: 'Italy 2026', icon: 'tag' }],
  labels: [
    { id: 'lb1', name: 'Kitty', icon: 'label' },
    { id: 'lb2', name: 'Doggo', icon: 'label' },
  ],
} as never

it('resolves a template\'s reporting tags, ignoring ids it cannot match', () => {
  const tx = recurringAsTransaction(template, lookups)
  expect(tx.labels?.map((l) => l.name)).toEqual(['Doggo'])
})

it('leaves reporting tags empty when the template carries none', () => {
  const tx = recurringAsTransaction({ ...template, labelIds: [] } as unknown as RecurringDto, lookups)
  expect(tx.labels).toEqual([])
})

const transferIn = {
  id: 'rt2', author: { id: 'u2', name: 'Partner', avatar: 'face:sky' }, currencyId: 'eur', amount: '91',
  description: 'save', category: null, payee: null, tag: null, labelIds: [], spentAt: '2026-08-25 00:00:00',
  direction: 'in', type: 'transfer',
  templateId: 'rt2', schedule: 'monthly', accountId: '', accountName: '',
  accountRecipientId: 'a-pot', accountRecipientName: 'Euro pot', amountRecipient: '91', currencyRecipientId: 'eur',
  amountSource: '100', currencySourceId: 'usd',
  labelNames: [], isNext: true, isOverdue: false, canEdit: false,
} as BudgetUpcomingDto
const currencies = [
  { id: 'usd', code: 'USD', name: 'US Dollar', symbol: '$', fractionDigits: 2 },
  { id: 'eur', code: 'EUR', name: 'Euro', symbol: '€', fractionDigits: 2 },
] as CurrencyDto[]

it('an upcoming transfer names its recipient from the wire when the caller lacks the account', () => {
  const tx = upcomingAsTransaction(transferIn, { accounts: [], currencies })
  expect(tx.accountRecipientId).toBe('a-pot')
  expect(tx.amountRecipient).toBe('91')
  expect(tx.accountRecipient?.name).toBe('Euro pot')
  expect(tx.accountRecipient?.currency?.id).toBe('eur')
  // the sender is outside the budget: no name to show, but its own side of the money
  expect(tx.account?.name).toBeUndefined()
  expect(tx.amount).toBe('100')
  expect(tx.account?.currency?.id).toBe('usd')
})

it('an upcoming transfer resolves its recipient against the caller\'s accounts', () => {
  const pot = { id: 'a-pot', name: 'My pot', icon: 'savings' } as AccountDto
  const tx = upcomingAsTransaction(transferIn, { accounts: [pot], currencies })
  expect(tx.accountRecipient).toBe(pot)
})

it('an upcoming transfer to an account outside the budget keeps its amount but no name', () => {
  const tx = upcomingAsTransaction(
    { ...transferIn, accountId: 'a1', accountName: 'Cash', currencyId: 'usd', amount: '40', direction: 'out',
      accountRecipientId: '', accountRecipientName: '', amountRecipient: '40', currencyRecipientId: 'usd', amountSource: '40', currencySourceId: 'usd' } as BudgetUpcomingDto,
    { accounts: [], currencies },
  )
  expect(tx.accountRecipientId).toBeNull()
  expect(tx.amountRecipient).toBe('40')
  expect(tx.accountRecipient?.name).toBeUndefined()
  expect(tx.accountRecipient?.currency?.id).toBe('usd')
})

it('an upcoming expense has no recipient', () => {
  const tx = upcomingAsTransaction(
    { ...transferIn, type: 'expense', direction: undefined, accountRecipientId: '', accountRecipientName: '', amountRecipient: '', currencyRecipientId: '', amountSource: '', currencySourceId: '' } as unknown as BudgetUpcomingDto,
    { accounts: [], currencies },
  )
  expect(tx.accountRecipientId).toBeNull()
  expect(tx.amountRecipient).toBeNull()
  expect(tx.accountRecipient).toBeUndefined()
  expect(tx.amount).toBe('91')
  expect(tx.account?.currency?.id).toBe('eur')
})

it('an upcoming transfer from an older server without source fields keeps the listed amount', () => {
  const { amountSource: _a, currencySourceId: _c, ...old } = transferIn
  const tx = upcomingAsTransaction(old as BudgetUpcomingDto, { accounts: [], currencies })
  expect(tx.amount).toBe('91')
  expect(tx.account?.currency?.id).toBe('eur')
})
