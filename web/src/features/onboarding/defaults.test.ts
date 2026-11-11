import { describe, expect, it } from 'vitest'
import { availableIcons } from '@/lib/icons'
import {
  DEFAULT_EXPENSE_CATEGORIES,
  DEFAULT_INCOME_CATEGORIES,
} from './defaults'

describe('onboarding defaults', () => {
  it('uses only known icons', () => {
    for (const c of [...DEFAULT_EXPENSE_CATEGORIES, ...DEFAULT_INCOME_CATEGORIES]) {
      expect(availableIcons).toContain(c.icon)
    }
    for (const i of ['payments', 'credit_card']) expect(availableIcons).toContain(i)
  })

  it('has the expected keys', () => {
    expect(DEFAULT_EXPENSE_CATEGORIES.map((c) => c.key)).toHaveLength(11)
    expect(DEFAULT_EXPENSE_CATEGORIES.map((c) => c.key).slice(8, 10)).toEqual(['kids', 'pets'])
    expect(DEFAULT_INCOME_CATEGORIES.map((c) => c.key)).toEqual(['salary', 'bonus', 'other_income'])
  })
})
