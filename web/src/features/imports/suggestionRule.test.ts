import { describe, expect, it } from 'vitest'
import type { ImportRuleDto, ImportRuleSpecDto } from '@/api/dto/imports'
import { findSuggestedPayee, suggestionSpec } from './suggestionRule'

const spec: ImportRuleSpecDto = {
  sourceId: '', action: 'classify', matchField: 'external_payee', matchType: 'contains', matchValue: 'Blue Bottle',
  isCaseSensitive: false, categoryId: 'cat', payeeId: '', tagId: '', labelIds: [], priority: 0,
}
const stored: ImportRuleDto = { ...spec, tagId: 'tag', priority: 3, id: 'rule', createdAt: '', updatedAt: '' }

describe('suggestionSpec', () => {
  it('writes nothing when the stored rule already matches', () => {
    expect(suggestionSpec(stored, spec)).toBeNull()
  })

  it('keeps the fields the dialog does not edit', () => {
    expect(suggestionSpec(stored, { ...spec, categoryId: 'other' })).toMatchObject({ tagId: 'tag', priority: 3, categoryId: 'other' })
  })

  it('turns a rule changed to skip on the rules page back into classify', () => {
    expect(suggestionSpec({ ...stored, action: 'skip' }, spec)).toMatchObject({ action: 'classify', categoryId: 'cat' })
    expect(suggestionSpec({ ...stored, action: 'skip' }, { ...spec, categoryId: 'other' })?.action).toBe('classify')
  })
})

describe('findSuggestedPayee', () => {
  const payee = (id: string, name: string, over: { ownerUserId?: string; isArchived?: 0 | 1 } = {}) =>
    ({ id, name, ownerUserId: over.ownerUserId ?? 'u1', isArchived: over.isArchived ?? 0 })

  it('matches a payee whose name differs only in case and punctuation', () => {
    expect(findSuggestedPayee([payee('p1', 'Grocer'), payee('p2', 'blue-bottle')], 'Blue Bottle', 'u1')?.id).toBe('p2')
  })

  it('ignores a partner\'s payee of the same name', () => {
    expect(findSuggestedPayee([payee('p1', 'Blue Bottle', { ownerUserId: 'u2' })], 'Blue Bottle', 'u1')).toBeUndefined()
  })

  it('prefers an active payee to an archived one', () => {
    const payees = [payee('old', 'BLUE BOTTLE', { isArchived: 1 }), payee('new', 'Blue Bottle')]
    expect(findSuggestedPayee(payees, 'Blue Bottle', 'u1')?.id).toBe('new')
    expect(findSuggestedPayee(payees.slice(0, 1), 'Blue Bottle', 'u1')?.id).toBe('old')
  })

  it('finds nothing for a name with no letters or digits', () => {
    expect(findSuggestedPayee([payee('p1', '...')], '!!!', 'u1')).toBeUndefined()
  })
})
