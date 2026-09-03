import {
  initialFormState,
  buildPayload,
  evaluatedAmount,
  accountOptions,
  categoryOptions,
  canChangeAccountData,
  classificationChips,
  scrubForeignClassifications,
  toggleClassification,
  toggleLabel,
  toggleTag,
} from './useTransactionForm'
import type { AccountDto } from '@/api/dto/account'
import type { LabelDto } from '@/api/dto/label'
import type { RecurringDto } from '@/api/dto/recurring'
import type { TagDto } from '@/api/dto/tag'
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
    labelIds: ['lb1', 'lb2'], date: '2026-07-01 10:00:00', recurringId: null, isImported: 0,
  }
  const state = initialFormState({ transaction: tx }, [account({})], null)
  expect(state.isNew).toBe(false)
  expect(state.amount).toBe('1234.50')
  expect(state.categoryId).toBe('cat1')
  expect(state.date).toBe('2026-07-01 10:00:00')
  // the update write REPLACES the label set, so the edit form must start from
  // the stored ids or saving would detach them
  expect(state.labelIds).toEqual(['lb1', 'lb2'])
  expect(buildPayload(state).labelIds).toEqual(['lb1', 'lb2'])
})

it('toggles labels freely but keeps tags single-select', () => {
  const s0 = initialFormState({}, [account({})], 'a1')
  const s1 = toggleTag(s0, 't1')
  const s2 = toggleTag(s1, 't2')
  expect(s2.tagId).toBe('t2')
  expect(toggleTag(s2, 't2').tagId).toBeNull()

  const s3 = toggleLabel(s2, 'l1')
  const s4 = toggleLabel(s3, 'l2')
  expect(s4.labelIds).toEqual(['l1', 'l2'])
  expect(toggleLabel(s4, 'l1').labelIds).toEqual(['l2'])
  // the two namespaces are independent: a label toggle never touches the tag
  expect(s4.tagId).toBe('t2')
})

it('toggleClassification dispatches on the kind', () => {
  const s0 = initialFormState({}, [account({})], 'a1')
  expect(toggleClassification(s0, 'tag', 'x1').tagId).toBe('x1')
  expect(toggleClassification(s0, 'tag', 'x1').labelIds).toEqual([])
  expect(toggleClassification(s0, 'label', 'x1').labelIds).toEqual(['x1'])
  expect(toggleClassification(s0, 'label', 'x1').tagId).toBeNull()
})

it('clears labels for a transfer, as it does for the tag', () => {
  const base = initialFormState({}, [account({})], 'a1')
  const transfer = buildPayload({ ...base, type: 'transfer', amount: '10', accountRecipientId: 'a2', tagId: 'tag1', labelIds: ['l1', 'l2'] })
  expect(transfer.tagId).toBeNull()
  // always [], never null/undefined — the write replaces the whole set
  expect(transfer.labelIds).toEqual([])

  const expense = buildPayload({ ...base, type: 'expense', amount: '10', labelIds: ['l1', 'l2'] })
  expect(expense.labelIds).toEqual(['l1', 'l2'])
})

it('hydrates labelIds from a recurring template being posted', () => {
  const rt: RecurringDto = {
    id: 'r1', ownerUserId: 'u1', type: 'expense', accountId: 'a1', accountRecipientId: null,
    amount: '50', categoryId: 'cat1', payeeId: null, tagId: 'tg1', labelIds: ['l1'], description: 'rent',
    schedule: 'monthly', nextPaymentAt: '2026-08-02 00:00:00',
  }
  expect(initialFormState({ postRecurring: rt }, [account({})], null).labelIds).toEqual(['l1'])
})

describe('classificationChips', () => {
  const tag = (over: Partial<TagDto>): TagDto => ({
    id: 'tg1', ownerUserId: 'u1', name: 'vacation', icon: 'tag', position: 0, isArchived: 0, createdAt: '', updatedAt: '', ...over,
  })
  const label = (over: Partial<LabelDto>): LabelDto => ({
    id: 'lb1', ownerUserId: 'u1', name: 'health', icon: 'sell', position: 0, isArchived: 0, createdAt: '', updatedAt: '', ...over,
  })

  it('offers the account OWNER\'s live rows, tags first, with their stored icon', () => {
    const chips = classificationChips(
      [tag({}), tag({ id: 'tg2', ownerUserId: 'u2', name: 'foreign' })],
      [label({}), label({ id: 'lb2', ownerUserId: 'u2', name: 'foreign label' })],
      { tagId: null, labelIds: [] },
      'u1',
    )
    expect(chips.map((c) => [c.kind, c.id, c.icon])).toEqual([
      ['tag', 'tg1', 'tag'],
      ['label', 'lb1', 'sell'],
    ])
    expect(chips.every((c) => c.checked)).toBe(false)
  })

  it('hides an archived row that is not attached but keeps one that is', () => {
    const tags = [tag({}), tag({ id: 'tg-old', name: 'old tag', isArchived: 1 })]
    const labels = [label({}), label({ id: 'lb-old', name: 'old label', isArchived: 1 })]

    const none = classificationChips(tags, labels, { tagId: null, labelIds: [] }, 'u1')
    expect(none.map((c) => c.id)).toEqual(['tg1', 'lb1'])

    // dropping an attached-but-archived chip would drop its id from the form,
    // and the write replaces the whole set — the save would detach it
    const attached = classificationChips(tags, labels, { tagId: 'tg-old', labelIds: ['lb-old'] }, 'u1')
    expect(attached.map((c) => c.id)).toEqual(['tg1', 'tg-old', 'lb1', 'lb-old'])
    expect(attached.filter((c) => c.checked).map((c) => c.id)).toEqual(['tg-old', 'lb-old'])
  })

  it('marks every attached label checked and leaves the rest unchecked', () => {
    const chips = classificationChips(
      [tag({})],
      [label({}), label({ id: 'lb2', name: 'work' }), label({ id: 'lb3', name: 'gift' })],
      { tagId: null, labelIds: ['lb1', 'lb3'] },
      'u1',
    )
    expect(chips.filter((c) => c.checked).map((c) => c.id)).toEqual(['lb1', 'lb3'])
  })

  it('renders a tag and a label sharing one name as two independent chips', () => {
    const chips = classificationChips([tag({ name: 'Travel' })], [label({ name: 'Travel' })], { tagId: 'tg1', labelIds: [] }, 'u1')
    expect(chips.map((c) => [c.kind, c.checked])).toEqual([
      ['tag', true],
      ['label', false],
    ])
  })
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
  expect(canChangeAccountData(account({ owner: other, sharedAccess: [{ user: owner, role: 'admin', isAccepted: 1 }] }), 'u1')).toBe(true)
  expect(canChangeAccountData(account({ owner: other, sharedAccess: [{ user: owner, role: 'user', isAccepted: 1 }] }), 'u1')).toBe(false)
})

describe('foreign classifications on a shared account', () => {
  const tag = (over: Partial<TagDto>): TagDto => ({
    id: 'tg1', ownerUserId: 'u1', name: 'vacation', icon: 'tag', position: 0, isArchived: 0, createdAt: '', updatedAt: '', ...over,
  })
  const label = (over: Partial<LabelDto>): LabelDto => ({
    id: 'lb1', ownerUserId: 'u1', name: 'health', icon: 'sell', position: 0, isArchived: 0, createdAt: '', updatedAt: '', ...over,
  })

  // The server accepts only the ACCOUNT OWNER's classifications. Rows stored
  // under an older, laxer rule can name the caller's own — keeping them checked
  // makes every save fail, so they are dropped from the selection instead.
  it('drops an attached tag or label owned by someone other than the account owner', () => {
    const tags = [tag({}), tag({ id: 'tg-mine', ownerUserId: 'u2', name: 'my tag' })]
    const labels = [label({}), label({ id: 'lb-mine', ownerUserId: 'u2', name: 'my label' })]
    const chips = classificationChips(tags, labels, { tagId: 'tg-mine', labelIds: ['lb-mine'] }, 'u1')
    expect(chips.map((c) => c.id)).toEqual(['tg1', 'lb1'])
    expect(chips.some((c) => c.checked)).toBe(false)
  })

  it('still keeps an attached archived row that the OWNER owns', () => {
    // archived is not foreign: the server accepts it, so dropping it would
    // silently detach a still-valid classification
    const tags = [tag({}), tag({ id: 'tg-old', name: 'old', isArchived: 1 })]
    const chips = classificationChips(tags, [], { tagId: 'tg-old', labelIds: [] }, 'u1')
    expect(chips.filter((c) => c.checked).map((c) => c.id)).toEqual(['tg-old'])
  })
})

describe('scrubForeignClassifications', () => {
  const owned = { ownerUserId: 'u1' }
  const mine = { ownerUserId: 'u2' }
  const lists = {
    categories: [{ id: 'cat-owner', ...owned }, { id: 'cat-mine', ...mine }],
    payees: [{ id: 'pay-owner', ...owned }, { id: 'pay-mine', ...mine }],
    tags: [{ id: 'tg-owner', ...owned }, { id: 'tg-mine', ...mine }],
    labels: [{ id: 'lb-owner', ...owned }, { id: 'lb-mine', ...mine }],
  } as never

  it('clears every reference the account owner does not own', () => {
    const out = scrubForeignClassifications(
      { categoryId: 'cat-mine', payeeId: 'pay-mine', tagId: 'tg-mine', labelIds: ['lb-mine', 'lb-owner'] },
      lists,
      'u1',
    )
    // an unowned category falls back to uncategorized (null), the rest unselect
    expect(out).toEqual({ categoryId: null, payeeId: null, tagId: null, labelIds: ['lb-owner'] })
  })

  it('leaves the owner\'s own references untouched', () => {
    const selection = { categoryId: 'cat-owner', payeeId: 'pay-owner', tagId: 'tg-owner', labelIds: ['lb-owner'] }
    expect(scrubForeignClassifications(selection, lists, 'u1')).toEqual(selection)
  })

  it('keeps an id the lists have not loaded yet, so a slow query cannot wipe the form', () => {
    const selection = { categoryId: 'cat-owner', payeeId: null, tagId: null, labelIds: ['lb-owner'] }
    expect(scrubForeignClassifications(selection, { categories: [], payees: [], tags: [], labels: [] } as never, 'u1')).toEqual(selection)
  })

  it('is a no-op without a resolved account owner', () => {
    const selection = { categoryId: 'cat-mine', payeeId: null, tagId: null, labelIds: [] }
    expect(scrubForeignClassifications(selection, lists, undefined)).toEqual(selection)
  })
})

it('a queued import seeds a new transaction from the bank data, payee as description', () => {
  const state = initialFormState(
    { importQueued: { linkId: 'l1', type: 'expense', accountId: 'a1', amount: '12.5', currency: 'USD', payee: 'Blue Bottle', date: '2026-08-20 10:42:03' } },
    [account({})],
    null,
  )
  expect(state.id).toMatch(UUID_V7)
  expect(state.isNew).toBe(true)
  expect(state.type).toBe('expense')
  expect(state.accountId).toBe('a1')
  expect(state.amount).toBe('12.50')
  expect(state.description).toBe('Blue Bottle')
  expect(state.date).toBe('2026-08-20 10:42:03')
  expect(state.categoryId).toBeNull()
})

it('an unmapped queued import (accountId "") defaults to the first account, not a blank select', () => {
  const state = initialFormState(
    { importQueued: { linkId: 'l1', type: 'expense', accountId: '', amount: '12.5', currency: 'USD', payee: 'Blue Bottle', date: '2026-08-20 10:42:03' } },
    [account({ id: 'a1' }), account({ id: 'a2' })],
    null,
  )
  expect(state.accountId).toBe('a1')
})

it('seeds the amount when the queued row currency matches the account (case-insensitive)', () => {
  const state = initialFormState(
    { importQueued: { linkId: 'l1', type: 'expense', accountId: 'a1', amount: '12.5', currency: 'usd', payee: 'Blue Bottle', date: '2026-08-20 10:42:03' } },
    [account({})],
    null,
  )
  expect(state.amount).toBe('12.50')
})

it('leaves the amount blank when the queued row currency does not match the account, so a foreign amount is never misprefilled', () => {
  const state = initialFormState(
    { importQueued: { linkId: 'l1', type: 'expense', accountId: 'a1', amount: '12.5', currency: 'EUR', payee: 'Blue Bottle', date: '2026-08-20 10:42:03' } },
    [account({})],
    null,
  )
  expect(state.amount).toBe('')
})
