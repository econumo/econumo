import { describe, expect, it } from 'vitest'
import { availableIcons } from '@/lib/icons'
import {
  DEFAULT_EXPENSE_CATEGORIES,
  DEFAULT_INCOME_CATEGORIES,
  guessAccountIcon,
  looksLikeSavings,
} from './defaults'

const words = { cash: 'cash | wallet', savings: 'savings | reserve' }

const de = {
  cash: 'bargeld | geldbeutel | portemonnaie | cash',
  savings: 'sparkonto | sparbuch | sparen | sparschwein | tagesgeld | rücklage | notgroschen | reserve',
}

describe('onboarding defaults', () => {
  it('does not mistake bank names for savings or cash', () => {
    for (const name of ['Sparkasse', 'Sparda-Bank', 'Barclays']) {
      expect(looksLikeSavings(name, de.savings)).toBe(false)
      expect(guessAccountIcon(name, de)).toBe('credit_card')
    }
    expect(guessAccountIcon('Sparkonto', de)).toBe('savings')
  })

  it('uses only known icons', () => {
    for (const c of [...DEFAULT_EXPENSE_CATEGORIES, ...DEFAULT_INCOME_CATEGORIES]) {
      expect(availableIcons).toContain(c.icon)
    }
    for (const i of ['payments', 'savings', 'credit_card']) expect(availableIcons).toContain(i)
  })

  it('has the expected keys', () => {
    expect(DEFAULT_EXPENSE_CATEGORIES.map((c) => c.key)).toHaveLength(10)
    expect(DEFAULT_INCOME_CATEGORIES.map((c) => c.key)).toEqual(['salary', 'other_income'])
  })

  it('detects savings names', () => {
    expect(looksLikeSavings('My Savings', 'savings | reserve')).toBe(true)
    expect(looksLikeSavings('Chase', 'savings | reserve')).toBe(false)
    expect(looksLikeSavings('Chase', '')).toBe(false)
  })

  it('guesses the account icon', () => {
    expect(guessAccountIcon('Cash', words)).toBe('payments')
    expect(guessAccountIcon('Rainy reserve', words)).toBe('savings')
    expect(guessAccountIcon('Chase', words)).toBe('credit_card')
  })
})
