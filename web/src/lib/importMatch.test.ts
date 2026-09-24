import type { CreateTransactionDto } from '@/api/dto/transaction'
import type { TransactionImportLinkDto } from '@/api/dto/imports'
import { latestImportLink, normalizeMatchText, ruleDiff, suggestMatchValue } from './importMatch'

const link = (over: Partial<TransactionImportLinkDto> = {}): TransactionImportLinkDto => ({
  id: 'l1', sourceId: 's1', runId: 'r1', provider: 'apple-wallet', sourceName: 'iPhone', externalAccountId: 'wallet',
  externalTransactionId: 'tap-1', externalPayee: 'BLUE BOTTLE COFFEE', externalDescription: '', externalAmount: '4.75',
  externalCurrency: 'USD', externalPostedAt: '2026-09-01 10:00:00', status: 'created', importedAt: '2026-09-01 10:00:05',
  appliedCategoryId: '', appliedPayeeId: '', appliedTagId: '', appliedLabelIds: [], appliedRuleId: '', ...over,
})

const payload = (over: Partial<CreateTransactionDto> = {}): CreateTransactionDto => ({
  id: 't1', type: 'expense', accountId: 'a1', accountRecipientId: null, amount: '4.75', amountRecipient: '4.75',
  categoryId: null, description: 'BLUE BOTTLE COFFEE', payeeId: null, tagId: null, labelIds: [], date: '2026-09-01 10:00:00', ...over,
})

describe('normalizeMatchText', () => {
  it('trims and collapses whitespace runs exactly like the Go NormalizeText', () => {
    expect(normalizeMatchText('  BLUE   BOTTLE\t COFFEE \n')).toBe('BLUE BOTTLE COFFEE')
    expect(normalizeMatchText('')).toBe('')
  })
})

describe('suggestMatchValue', () => {
  it.each([
    ['BLUE BOTTLE COFFEE #142 SAN FRANCISCO CA', 'BLUE BOTTLE COFFEE'],
    ['SQ *BLUE BOTTLE', 'BLUE BOTTLE'],
    ['AMZN Mktp US*2K4L19XZ0', 'AMZN Mktp'],
    ['UBER TRIP 09/08', 'UBER TRIP'],
    ['  Apple   Pay  ', 'Apple Pay'],
    ['IKEA', 'IKEA'],
    ['1234', '1234'],
    ['', ''],
  ])('%s -> %s', (raw, want) => {
    expect(suggestMatchValue(raw)).toBe(want)
  })
})

describe('ruleDiff', () => {
  const before = (over: Partial<CreateTransactionDto> = {}) => payload(over)

  it('reports classification fields whose saved value differs from the applied snapshot', () => {
    expect(ruleDiff(payload({ categoryId: 'c1' }), link(), before())).toEqual({ categoryId: 'c1' })
    expect(ruleDiff(payload({ categoryId: 'c1', tagId: 't1', labelIds: ['l2', 'l1'] }), link({ appliedCategoryId: 'c1', appliedLabelIds: ['l1', 'l2'] }), before({ categoryId: 'c1', labelIds: ['l1', 'l2'] })))
      .toEqual({ tagId: 't1' })
  })
  it('is null when nothing classification-related changed — amount, date, notes edits never prompt', () => {
    expect(ruleDiff(payload({ amount: '5.00', description: 'edited', date: '2026-09-02 10:00:00', categoryId: 'c1' }), link({ appliedCategoryId: 'c1' }), before({ categoryId: 'c1' }))).toBeNull()
    expect(ruleDiff(payload(), link(), before())).toBeNull()
  })
  it('clearing a field is not a rule-worthy change', () => {
    expect(ruleDiff(payload({ categoryId: null }), link({ appliedCategoryId: 'c1' }), before({ categoryId: 'c1' }))).toBeNull()
  })
  it('reports only newly added labels, never the full replacement set', () => {
    expect(ruleDiff(payload({ labelIds: ['l1', 'l2'] }), link({ appliedLabelIds: ['l1'] }), before({ labelIds: ['l1'] }))).toEqual({ labelIds: ['l2'] })
  })
  it('a label REMOVAL is not rule-worthy on its own — a rule can only add labels', () => {
    expect(ruleDiff(payload({ labelIds: ['l1'] }), link({ appliedLabelIds: ['l1', 'l2'] }), before({ labelIds: ['l1', 'l2'] }))).toBeNull()
  })

  // The row a rule was created from is "edited" by construction, so a default
  // apply SKIPS it and never refreshes its applied_* snapshot. Comparing the
  // save against the snapshot alone would therefore re-prompt — offering a
  // duplicate rule — every time that transaction is saved again.
  it('does not re-prompt when this save left the classification untouched, even though it still diverges from the snapshot', () => {
    const corrected = link({ appliedCategoryId: '', appliedRuleId: '' })
    expect(ruleDiff(payload({ categoryId: 'c1', description: 'typo fixed' }), corrected, before({ categoryId: 'c1' }))).toBeNull()
    expect(ruleDiff(payload({ labelIds: ['l1'] }), corrected, before({ labelIds: ['l1'] }))).toBeNull()
    // ... but a further correction in the SAME field still prompts
    expect(ruleDiff(payload({ categoryId: 'c2' }), corrected, before({ categoryId: 'c1' }))).toEqual({ categoryId: 'c2' })
  })

  it('falls back to the snapshot alone when the pre-edit values are unknown', () => {
    expect(ruleDiff(payload({ categoryId: 'c1' }), link(), null)).toEqual({ categoryId: 'c1' })
  })
})

describe('latestImportLink', () => {
  it('picks the most recently imported link', () => {
    const older = link({ id: 'old', importedAt: '2026-08-01 00:00:00' })
    const newer = link({ id: 'new', importedAt: '2026-09-01 00:00:00' })
    expect(latestImportLink([older, newer])?.id).toBe('new')
    expect(latestImportLink([newer, older])?.id).toBe('new')
    expect(latestImportLink([])).toBeNull()
  })
})
