import { normalize, tryNormalize, add, sub, mul, div, cmp, isZero, isNegative, abs, round, toFixedString } from './decimal'
import vectors from './decimal_vectors.json'

describe('golden parity with vo.DecimalNumber', () => {
  it.each(vectors.arith)('arith $a ∘ $b', (c) => {
    expect(add(c.a, c.b)).toBe(c.add)
    expect(sub(c.a, c.b)).toBe(c.sub)
    expect(mul(c.a, c.b)).toBe(c.mul)
    expect(div(c.a, c.b)).toBe(c.div)
  })

  it.each(vectors.round)('round($value, $precision)', (c) => {
    expect(round(c.value, c.precision)).toBe(c.result)
  })
})

describe('normalize', () => {
  it('trims zeros like the backend', () => {
    expect(normalize('0.92000000')).toBe('0.92')
    expect(normalize('100.50')).toBe('100.5')
    expect(normalize('007')).toBe('7')
    expect(normalize('')).toBe('0')
    expect(normalize(null)).toBe('0')
    expect(normalize(undefined)).toBe('0')
    expect(normalize('not-a-number')).toBe('0')
  })

  it('truncates to scale 8 and expands scientific notation', () => {
    expect(normalize('1.123456789')).toBe('1.12345678')
    expect(normalize('1e-7')).toBe('0.0000001')
    expect(normalize(1e-7)).toBe('0.0000001')
  })

  it('preserves large amounts exactly (the reason this module exists)', () => {
    expect(normalize('12345678901234567.89')).toBe('12345678901234567.89')
    expect(add('12345678901234567.89', '0.01')).toBe('12345678901234567.9')
    expect(sub('12345678901234567.90', '0.01')).toBe('12345678901234567.89')
  })
})

describe('tryNormalize', () => {
  it('is strict: null for garbage, normalized string otherwise', () => {
    expect(tryNormalize('100.50')).toBe('100.5')
    expect(tryNormalize('  42 ')).toBe('42')
    expect(tryNormalize('')).toBeNull()
    expect(tryNormalize('5+5')).toBeNull()
    expect(tryNormalize('abc')).toBeNull()
  })
})

describe('comparisons and helpers', () => {
  it('cmp / isZero / isNegative / abs', () => {
    expect(cmp('1.5', '1.50')).toBe(0)
    expect(cmp('2', '10')).toBe(-1)
    expect(cmp('-1', '-2')).toBe(1)
    expect(isZero('0.00000000')).toBe(true)
    expect(isZero('0.00000001')).toBe(false)
    expect(isNegative('-3.5')).toBe(true)
    expect(isNegative('3.5')).toBe(false)
    expect(abs('-3.5')).toBe('3.5')
  })

  it('div by zero yields 0 (display-safe, unlike the backend panic)', () => {
    expect(div('5', '0')).toBe('0')
  })

  it('toFixedString pads and rounds half-away', () => {
    expect(toFixedString('10.5', 2)).toBe('10.50')
    expect(toFixedString('10.567', 2)).toBe('10.57')
    expect(toFixedString('10.565', 2)).toBe('10.57')
  })
})
