import { normalizeNumber, formatNumber, addThousandSeparators, moneyFormat } from './money'

const usd = { symbol: '$', fractionDigits: 2 }
const btc = { symbol: '₿', fractionDigits: 8 }

describe('normalizeNumber', () => {
  it('handles null-ish and non-finite', () => {
    expect(normalizeNumber(null)).toBe('0')
    expect(normalizeNumber(undefined)).toBe('0')
    expect(normalizeNumber('not-a-number')).toBe('0')
    expect(normalizeNumber(Infinity)).toBe('0')
  })

  it('strips trailing zeros and caps at 8 decimals', () => {
    expect(normalizeNumber('100.50')).toBe('100.5')
    expect(normalizeNumber(0)).toBe('0')
    expect(normalizeNumber(1.123456789)).toBe('1.12345678')
  })

  it('avoids scientific notation near zero', () => {
    expect(normalizeNumber(0.0000001)).toBe('0.0000001')
    expect(normalizeNumber(1e-7)).toBe('0.0000001')
  })
})

describe('formatNumber', () => {
  it('rounds at zero digits and fixes precision', () => {
    expect(formatNumber(10.6, 0, false)).toBe('11')
    expect(formatNumber(10.5, 2, true)).toBe('10.50')
  })

  it('keeps more decimals than requested when present and not fixed', () => {
    expect(formatNumber(10.12345, 2, false)).toBe('10.12345')
    expect(formatNumber(10.1, 2, false)).toBe('10.10')
  })
})

describe('addThousandSeparators', () => {
  it('separates the integer part only', () => {
    expect(addThousandSeparators('1234567.89')).toEqual(['1,234,567', '89'])
    expect(addThousandSeparators('999')).toEqual(['999'])
  })
})

describe('moneyFormat', () => {
  it('uses native precision and appends the symbol with a space', () => {
    expect(moneyFormat(1234.5, usd)).toBe('1,234.50 $')
  })

  it('non-native precision: integers use the currency fractionDigits, decimals keep up to 8', () => {
    expect(moneyFormat(1234, usd, { useNativePrecision: false, showCurrency: false })).toBe('1,234.00')
    expect(moneyFormat(1234.5, usd, { useNativePrecision: false, showCurrency: false })).toBe('1,234.50')
    expect(moneyFormat(1234.12345, usd, { useNativePrecision: false, showCurrency: false })).toBe('1,234.12345')
  })

  it('omits the symbol when asked; no currency means 8 fixed digits (Vue parity)', () => {
    expect(moneyFormat(5, usd, { showCurrency: false })).toBe('5.00')
    expect(moneyFormat(5)).toBe('5.00000000')
  })

  it('can skip thousand separators (edit-field seeding mode)', () => {
    expect(moneyFormat(1234.5, usd, { showCurrency: false, useNativePrecision: false, useThousandSeparator: false })).toBe('1234.50')
  })

  it('keeps negatives inline and formats wire decimal strings', () => {
    expect(moneyFormat('-42.1', usd)).toBe('-42.10 $')
    expect(moneyFormat('0', btc)).toBe('0.00000000 ₿')
  })
})
