import { sanitizeInput, validateFormula, evaluateFormula } from './calculator'

describe('sanitizeInput (parity with the Vue implementation)', () => {
  it('passes formulas through and strips junk', () => {
    expect(sanitizeInput('5+3*2')).toBe('5+3*2')
    expect(sanitizeInput('5abc+2')).toBe('5+2')
    // commas become dots AFTER junk stripping — '$1,000.5' → '1.000.5', same as Vue
    expect(sanitizeInput('$1,000.5')).toBe('1.000.5')
  })

  it('normalizes comma decimals and leading zeros', () => {
    expect(sanitizeInput('5,5+2')).toBe('5.5+2')
    expect(sanitizeInput('007')).toBe('7')
    expect(sanitizeInput('0.5+007')).toBe('0.5+7')
  })

  it('collapses operator runs only when not preceded by a digit (Vue lookbehind semantics)', () => {
    expect(sanitizeInput('.5..+2')).toBe('.5.2')
    // after a digit the run survives sanitize; eval treated the second + as unary, so 5++2 = 7
    expect(sanitizeInput('5++2')).toBe('5++2')
    expect(evaluateFormula('5++2=')).toBe('7')
  })
})

describe('validateFormula', () => {
  it('accepts empty, plain numbers and valid formulas', () => {
    expect(validateFormula('')).toBe(true)
    expect(validateFormula('=')).toBe(true)
    expect(validateFormula('42')).toBe(true)
    expect(validateFormula('5+3*2')).toBe(true)
    expect(validateFormula('-5+2')).toBe(true)
    expect(validateFormula('+5')).toBe(true)
    expect(validateFormula('5*-2')).toBe(true)
    expect(validateFormula('1.5/0.5')).toBe(true)
  })

  it('rejects malformed formulas', () => {
    expect(validateFormula('5+')).toBe(false)
    expect(validateFormula('5+*2')).toBe(false)
    expect(validateFormula('..')).toBe(false)
  })
})

describe('evaluateFormula', () => {
  it('evaluates only when = is present', () => {
    expect(evaluateFormula('5+3*2=')).toBe('11')
    expect(evaluateFormula('5+3*2')).toBe('5+3*2')
  })

  it('applies precedence, unary minus and 10-place rounding', () => {
    expect(evaluateFormula('2+10/2=')).toBe('7')
    expect(evaluateFormula('-5+2=')).toBe('-3')
    expect(evaluateFormula('1/3=')).toBe('0.3333333333')
    expect(evaluateFormula('10-2*3=')).toBe('4')
  })

  it('drops the = when the formula is invalid', () => {
    expect(evaluateFormula('5+=')).toBe('5+')
  })
})
