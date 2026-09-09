import type { CreateTransactionDto } from '@/api/dto/transaction'
import type { ImportRuleSpecDto, TransactionImportLinkDto } from '@/api/dto/imports'
import { latestImportLink, matchesRule, normalizeMatchText, ruleDiff, suggestMatchValue } from './importMatch'

const spec = (over: Partial<ImportRuleSpecDto> = {}): ImportRuleSpecDto => ({
  sourceId: '', action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: 'blue bottle',
  isCaseSensitive: false, categoryId: 'c1', payeeId: '', tagId: '', labelIds: [], priority: 0, ...over,
})

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

describe('matchesRule', () => {
  it('folds case unless the rule is case-sensitive', () => {
    expect(matchesRule(spec(), { payee: 'Blue Bottle Coffee', description: '' })).toBe(true)
    expect(matchesRule(spec({ isCaseSensitive: true }), { payee: 'Blue Bottle Coffee', description: '' })).toBe(false)
    expect(matchesRule(spec({ isCaseSensitive: true, matchValue: 'Blue Bottle' }), { payee: 'Blue Bottle Coffee', description: '' })).toBe(true)
  })
  it('exact and prefix compare normalized text; an empty value never matches', () => {
    expect(matchesRule(spec({ matchType: 'exact', matchValue: 'blue bottle coffee' }), { payee: ' BLUE  BOTTLE COFFEE', description: '' })).toBe(true)
    expect(matchesRule(spec({ matchType: 'exact' }), { payee: 'BLUE BOTTLE COFFEE', description: '' })).toBe(false)
    expect(matchesRule(spec({ matchType: 'prefix' }), { payee: 'BLUE BOTTLE COFFEE', description: '' })).toBe(true)
    expect(matchesRule(spec({ matchType: 'prefix', matchValue: 'coffee' }), { payee: 'BLUE BOTTLE COFFEE', description: '' })).toBe(false)
    expect(matchesRule(spec({ matchValue: '   ' }), { payee: 'BLUE BOTTLE COFFEE', description: '' })).toBe(false)
  })
  it('description rules read the description and fall back to the payee when the provider sent none', () => {
    expect(matchesRule(spec({ matchField: 'description', matchValue: 'memo' }), { payee: 'BLUE BOTTLE', description: 'card memo' })).toBe(true)
    expect(matchesRule(spec({ matchField: 'description' }), { payee: 'BLUE BOTTLE', description: 'card memo' })).toBe(false)
    expect(matchesRule(spec({ matchField: 'description' }), { payee: 'BLUE BOTTLE', description: '' })).toBe(true)
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
  it('reports classification fields whose saved value differs from the applied snapshot', () => {
    expect(ruleDiff(payload({ categoryId: 'c1' }), link())).toEqual({ categoryId: 'c1' })
    expect(ruleDiff(payload({ categoryId: 'c1', tagId: 't1', labelIds: ['l2', 'l1'] }), link({ appliedCategoryId: 'c1', appliedLabelIds: ['l1', 'l2'] })))
      .toEqual({ tagId: 't1' })
  })
  it('is null when nothing classification-related changed — amount, date, notes edits never prompt', () => {
    expect(ruleDiff(payload({ amount: '5.00', description: 'edited', date: '2026-09-02 10:00:00', categoryId: 'c1' }), link({ appliedCategoryId: 'c1' }))).toBeNull()
    expect(ruleDiff(payload(), link())).toBeNull()
  })
  it('clearing a field is not a rule-worthy change', () => {
    expect(ruleDiff(payload({ categoryId: null }), link({ appliedCategoryId: 'c1' }))).toBeNull()
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
