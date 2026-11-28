import { describe, expect, it } from 'vitest'
import { detectDecimalSeparator, parseAmountCell, signedAmount } from './amounts'

describe('parseAmountCell', () => {
  it('reads thousands and decimal separators by the given separator', () => {
    expect(parseAmountCell('1.234,56', ',')).toBe('1234.56')
    expect(parseAmountCell('-1,234.56', '.')).toBe('-1234.56')
    expect(parseAmountCell('1 234,56', ',')).toBe('1234.56')
    expect(parseAmountCell('1 234,56', ',')).toBe('1234.56')
    expect(parseAmountCell('1 234.56', '.')).toBe('1234.56')
    expect(parseAmountCell('1,234', '.')).toBe('1234')
    expect(parseAmountCell('1,234', ',')).toBe('1.234')
  })

  it('strips currency symbols and codes', () => {
    expect(parseAmountCell('€ 12,00', ',')).toBe('12.00')
    expect(parseAmountCell('$-12.50', '.')).toBe('-12.50')
    expect(parseAmountCell('-€12.50', '.')).toBe('-12.50')
    expect(parseAmountCell('12.50 USD', '.')).toBe('12.50')
    expect(parseAmountCell('USD 1,234.50', '.')).toBe('1234.50')
  })

  it('reads negatives in parentheses and with a trailing minus', () => {
    expect(parseAmountCell('(45.00)', '.')).toBe('-45.00')
    expect(parseAmountCell('45.00-', '.')).toBe('-45.00')
    expect(parseAmountCell('−45.00', '.')).toBe('-45.00')
    expect(parseAmountCell('+45.00', '.')).toBe('45.00')
  })

  it('returns null for empty and non-numeric cells', () => {
    expect(parseAmountCell('', '.')).toBeNull()
    expect(parseAmountCell('   ', '.')).toBeNull()
    expect(parseAmountCell('abc', '.')).toBeNull()
    expect(parseAmountCell('12abc34', '.')).toBeNull()
    expect(parseAmountCell('-', '.')).toBeNull()
  })

  it('normalizes bare fractions and trailing separators', () => {
    expect(parseAmountCell('.5', '.')).toBe('0.5')
    expect(parseAmountCell('12.', '.')).toBe('12')
  })

  it('never keeps a sign on zero', () => {
    expect(parseAmountCell('-0.00', '.')).toBe('0.00')
  })
})

describe('detectDecimalSeparator', () => {
  it('lets a value with both separators settle it by the last one', () => {
    expect(detectDecimalSeparator(['1.234,56'])).toEqual({ sep: ',', ambiguous: false, example: null })
    expect(detectDecimalSeparator(['1,234.56'])).toEqual({ sep: '.', ambiguous: false, example: null })
  })

  it('settles on a separator that is not followed by three digits', () => {
    expect(detectDecimalSeparator(['12,5'])).toMatchObject({ sep: ',', ambiguous: false })
    expect(detectDecimalSeparator(['1,234', '12,50'])).toMatchObject({ sep: ',', ambiguous: false })
  })

  it('treats a repeated separator as thousands', () => {
    expect(detectDecimalSeparator(['1,234,567'])).toMatchObject({ sep: '.', ambiguous: false })
  })

  it('flags a lone separator followed by three digits as ambiguous', () => {
    expect(detectDecimalSeparator(['1,234'])).toEqual({ sep: '.', ambiguous: true, example: '1,234' })
  })

  it('defaults to a dot', () => {
    expect(detectDecimalSeparator([])).toEqual({ sep: '.', ambiguous: false, example: null })
    expect(detectDecimalSeparator(['12', 'abc'])).toEqual({ sep: '.', ambiguous: false, example: null })
  })
})

describe('signedAmount', () => {
  it('uses the outflow column as a negative amount', () => {
    expect(signedAmount({ inflow: '', outflow: '-12.00' }, 'dual', false, '.')).toBe('-12.00')
    expect(signedAmount({ inflow: '', outflow: '12.00' }, 'dual', false, '.')).toBe('-12.00')
    expect(signedAmount({ inflow: '', outflow: '' }, 'dual', false, '.')).toBeNull()
  })

  it('prefers a non-zero inflow', () => {
    expect(signedAmount({ inflow: '5.00', outflow: '' }, 'dual', false, '.')).toBe('5.00')
    expect(signedAmount({ inflow: '-5.00', outflow: '' }, 'dual', false, '.')).toBe('5.00')
    expect(signedAmount({ inflow: '0.00', outflow: '7.00' }, 'dual', false, '.')).toBe('-7.00')
    expect(signedAmount({ inflow: '5.00', outflow: '7.00' }, 'dual', false, '.')).toBe('5.00')
  })

  it('negates a single column when flipped', () => {
    expect(signedAmount({ single: '4.50' }, 'single', true, '.')).toBe('-4.50')
    expect(signedAmount({ single: '-4.50' }, 'single', true, '.')).toBe('4.50')
    expect(signedAmount({ single: '4.50' }, 'single', false, '.')).toBe('4.50')
    expect(signedAmount({ single: '0.00' }, 'single', true, '.')).toBe('0.00')
    expect(signedAmount({ single: 'x' }, 'single', true, '.')).toBeNull()
    expect(signedAmount({}, 'single', false, '.')).toBeNull()
  })
})
