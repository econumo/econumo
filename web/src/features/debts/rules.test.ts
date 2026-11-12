import type { TFunction } from 'i18next'
import type { DebtDto, DebtRuleDto } from '@/api/dto/debt'
import { addRuleBlocker, ruleOwnerLabel, rulesChanged } from './rules'
import { wireDebt, wireRule } from './testDebt'

const rule = (over: Partial<DebtRuleDto>): DebtRuleDto => ({ ...wireRule, id: `r-${Math.random()}`, ...over })
const debt = (id: string, rules: DebtRuleDto[], over: Partial<DebtDto> = {}): DebtDto => ({ ...wireDebt, id, rules, startDate: '', ...over })

describe('addRuleBlocker', () => {
  const picked = debt('d-picked', [rule({ categoryId: 'cat-car' })])

  it('a debt that already has a rule on the category is covered, a narrowed one included', () => {
    expect(addRuleBlocker([picked], 'd-picked', 'cat-car')).toBe('covered')
    const narrowed = debt('d-picked', [rule({ categoryId: 'cat-car', payeeId: 'p1', payeeName: 'Bank' })])
    expect(addRuleBlocker([narrowed], 'd-picked', 'cat-car')).toBe('covered')
  })

  it('another debt with a whole rule on the category and the same start date conflicts', () => {
    const other = debt('d-other', [rule({ categoryId: 'cat-food' })])
    expect(addRuleBlocker([picked, other], 'd-picked', 'cat-food')).toBe('conflict')
  })

  it('another debt with a payee-narrowed rule on the category conflicts', () => {
    const other = debt('d-other', [rule({ categoryId: 'cat-food', payeeId: 'p1', payeeName: 'Grocer' })], { startDate: '2020-01-01' })
    expect(addRuleBlocker([picked, other], 'd-picked', 'cat-food')).toBe('conflict')
  })

  it('a labelled rule on the category overlaps the whole category', () => {
    const other = debt('d-other', [rule({ categoryId: 'cat-food', labelId: 'label1', labelName: 'health' })], { startDate: '2020-01-01' })
    expect(addRuleBlocker([picked, other], 'd-picked', 'cat-food')).toBe('conflict')
  })

  it('another debt\'s whole rule starting on a different date is partitioned, not a conflict', () => {
    const other = debt('d-other', [rule({ categoryId: 'cat-food' })], { startDate: '2020-01-01' })
    expect(addRuleBlocker([picked, other], 'd-picked', 'cat-food')).toBeNull()
  })

  it('a category no debt names blocks nothing', () => {
    const other = debt('d-other', [rule({ categoryId: 'cat-rent' })])
    expect(addRuleBlocker([picked, other], 'd-picked', 'cat-food')).toBeNull()
  })
})

describe('ruleOwnerLabel', () => {
  const t = ((key: string, params?: Record<string, string>) => `${key}:${params?.owner}/${params?.category}`) as unknown as TFunction

  it('names the owner of another user\'s category', () => {
    expect(ruleOwnerLabel(rule({ ownerUserId: 'u2', ownerName: 'Anna', categoryName: 'Mortgage' }), 'u1', t)).toBe('debts.rules.owned_category:Anna/Mortgage')
  })

  it('the viewer\'s own category is just its name', () => {
    expect(ruleOwnerLabel(rule({ ownerUserId: 'u1', categoryName: 'Food' }), 'u1', t)).toBe('Food')
  })
})

describe('rulesChanged', () => {
  const before = [wireRule]
  const same = { categoryId: 'cat-food', payeeId: null, tagId: null, labelId: null }

  it('the loaded rules are no change', () => {
    expect(rulesChanged(before, [same])).toBe(false)
  })

  it('a narrowed, added or removed rule is a change', () => {
    expect(rulesChanged(before, [{ ...same, payeeId: 'p1' }])).toBe(true)
    expect(rulesChanged(before, [same, { ...same, categoryId: 'cat-rent' }])).toBe(true)
    expect(rulesChanged([wireRule, rule({ categoryId: 'cat-rent' })], [same])).toBe(true)
  })
})
