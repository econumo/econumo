import { describe, expect, it } from 'vitest'
import type { ImportRuleDto, ImportRuleSpecDto } from '@/api/dto/imports'
import { suggestionSpec } from './suggestionRule'

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
