import { describe, expect, it } from 'vitest'
import { BudgetElementType } from '@/api/dto/budget'
import { planHint, planHintKind, planNeed } from './planHint'

describe('planHint', () => {
  it('an empty plan stays empty', () => {
    expect(planHint({ planned: '', spent: '300', upcoming: '250', kind: 'expense' })).toBeNull()
    expect(planHint({ planned: '', spent: '0', upcoming: '300', kind: 'savings' })).toBeNull()
    expect(planHint({ planned: '', spent: '0', upcoming: '500', kind: 'income' })).toBeNull()
  })

  it('expense negative upcoming', () => {
    expect(planHint({ planned: '', spent: '0', upcoming: '-50', kind: 'expense' })).toBeNull()
  })

  it('no plan gets no hint', () => {
    expect(planHint({ planned: '', spent: '0', upcoming: '650', kind: 'expense' })).toBeNull()
  })

  it('a plan of 0 is a plan: short of anything still due', () => {
    expect(planHint({ planned: '0', spent: '0', upcoming: '650', kind: 'expense' })).toEqual({ kind: 'short' })
  })

  it('short when the expense plan is below the need', () => {
    expect(planHint({ planned: '500', spent: '300', upcoming: '250', kind: 'expense' })).toEqual({
      kind: 'short',
    })
  })

  it('normal when the plan covers the need', () => {
    expect(planHint({ planned: '600', spent: '300', upcoming: '250', kind: 'expense' })).toBeNull()
  })

  it('nothing without upcoming', () => {
    expect(planHint({ planned: '', spent: '300', upcoming: undefined, kind: 'expense' })).toBeNull()
    expect(planHint({ planned: '', spent: '300', upcoming: '0', kind: 'expense' })).toBeNull()
  })

  it('income is never short', () => {
    expect(planHint({ planned: '100', spent: '0', upcoming: '500', kind: 'income' })).toBeNull()
  })

  it('savings net out shows nothing', () => {
    expect(planHint({ planned: '', spent: '0', upcoming: '-200', kind: 'savings' })).toBeNull()
    expect(planHint({ planned: '', spent: '0', upcoming: '0', kind: 'savings' })).toBeNull()
  })
})

describe('planNeed', () => {
  it('adds what is still due to what was spent', () => {
    expect(planNeed('300', '250')).toBe('550')
    expect(planNeed('300', undefined)).toBe('300')
  })
})

describe('planHintKind', () => {
  it('maps expense-like types', () => {
    expect(planHintKind(BudgetElementType.CATEGORY)).toBe('expense')
    expect(planHintKind(BudgetElementType.TAG)).toBe('expense')
    expect(planHintKind(BudgetElementType.ENVELOPE)).toBe('expense')
  })

  it('maps income types', () => {
    expect(planHintKind(BudgetElementType.INCOME_CATEGORY)).toBe('income')
    expect(planHintKind(BudgetElementType.INCOME_ENVELOPE)).toBe('income')
  })

  it('maps savings', () => {
    expect(planHintKind(BudgetElementType.SAVINGS)).toBe('savings')
  })
})
