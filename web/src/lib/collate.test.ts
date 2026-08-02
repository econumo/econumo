import { describe, expect, it } from 'vitest'
import { compareNames } from './collate'

describe('compareNames', () => {
  it('sorts Chinese names by pinyin, not by code point', () => {
    const names = ['上海', '北京', '广州']
    // Běijīng < Guǎngzhōu < Shànghǎi
    expect([...names].sort((a, b) => compareNames(a, b, 'zh'))).toEqual(['北京', '广州', '上海'])
    // the code-point order the host default would have produced
    expect([...names].sort((a, b) => compareNames(a, b, 'en'))).toEqual(['上海', '北京', '广州'])
  })

  it('orders letters with diacritics by the language, not by code point', () => {
    // Swedish sorts ä after z; German treats it as a variant of a
    expect(compareNames('äpple', 'zebra', 'sv')).toBeGreaterThan(0)
    expect(compareNames('äpfel', 'zebra', 'de')).toBeLessThan(0)
  })

  it('defaults to English when no language is given', () => {
    expect(compareNames('apple', 'banana')).toBeLessThan(0)
  })
})
