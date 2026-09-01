import type { AccountDto } from '@/api/dto/account'
import type { RecurringDto } from '@/api/dto/recurring'
import { fixtureAccounts } from '@/test/fixtures'
import { recurringPostPayload } from './postPayload'

const accounts = fixtureAccounts as unknown as AccountDto[]
// doubling is enough to tell a converted leg from a passed-through one
const exchangeFn = (from: string, to: string, amount: string) => (from === to ? amount : String(Number(amount) * 2))

const template = (overrides: Partial<RecurringDto> = {}): RecurringDto => ({
  id: 'r1', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null,
  amount: '50.5', categoryId: 'cat-food', payeeId: 'p1', tagId: 'tg1', labelIds: ['lb1'], description: 'rent',
  schedule: 'monthly', nextPaymentAt: '2026-08-02 00:00:00', ...overrides,
})

it('posts an expense with the template amount, date and classifications', () => {
  const payload = recurringPostPayload(template(), accounts, exchangeFn)
  expect(payload).toMatchObject({
    recurringId: 'r1', type: 'expense', accountId: 'a1', amount: '50.5',
    categoryId: 'cat-food', payeeId: 'p1', tagId: 'tg1', description: 'rent',
    date: '2026-08-02 00:00:00', accountRecipientId: null, amountRecipient: null,
  })
})

it('omits labelIds so the server inherits the template\'s labels', () => {
  // absent != empty on this endpoint: [] would post a transaction with NO
  // labels, and this path has no chip row for the user to have cleared
  const payload = recurringPostPayload(template(), accounts, exchangeFn)
  expect('labelIds' in payload).toBe(false)
  expect('labelIds' in recurringPostPayload(template({ type: 'transfer', accountRecipientId: 'a2' }), accounts, exchangeFn)).toBe(false)
})

it('posts a future-dated template as now instead of its scheduled date', () => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date(2026, 7, 20, 14, 30, 45))
  try {
    const payload = recurringPostPayload(template({ nextPaymentAt: '2026-09-01 00:00:00' }), accounts, exchangeFn)
    expect(payload.date).toBe('2026-08-20 14:30:45')
  } finally {
    vi.useRealTimers()
  }
})

it('keeps the scheduled date when the template is due today', () => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date(2026, 7, 20, 14, 30, 45))
  try {
    const payload = recurringPostPayload(template({ nextPaymentAt: '2026-08-20 00:00:00' }), accounts, exchangeFn)
    expect(payload.date).toBe('2026-08-20 00:00:00')
  } finally {
    vi.useRealTimers()
  }
})

it('mints a fresh transaction id rather than reusing the template id', () => {
  const first = recurringPostPayload(template(), accounts, exchangeFn)
  const second = recurringPostPayload(template(), accounts, exchangeFn)
  expect(first.id).not.toBe('r1')
  expect(first.id).not.toBe(second.id)
})

it('mirrors the amount on a same-currency transfer and drops classifications', () => {
  const payload = recurringPostPayload(
    template({ type: 'transfer', accountId: 'a1', accountRecipientId: 'a2' }),
    accounts,
    exchangeFn,
  )
  expect(payload).toMatchObject({
    accountRecipientId: 'a2', amount: '50.5', amountRecipient: '50.5',
    categoryId: null, payeeId: null, tagId: null,
  })
})

it('converts the recipient leg of a cross-currency transfer', () => {
  const payload = recurringPostPayload(
    template({ type: 'transfer', amount: '10', accountId: 'a1', accountRecipientId: 'a3' }),
    accounts,
    exchangeFn,
  )
  expect(payload.amount).toBe('10')
  expect(payload.amountRecipient).toBe('20')
})

it('falls back to the sender amount when the recipient account is not loaded', () => {
  const payload = recurringPostPayload(
    template({ type: 'transfer', amount: '10', accountId: 'a1', accountRecipientId: 'a-unknown' }),
    accounts,
    exchangeFn,
  )
  expect(payload.amountRecipient).toBe('10')
})
